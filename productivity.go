package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// M5 local productivity records. Everything here is local-only metadata; no
// message bodies or conversation histories are stored. Identity keys come
// from the DOM adapter and are opaque hashes, never raw WhatsApp IDs.

const (
	adapterVersionCurrent = 2

	identityKindDataID       = "data-id"
	identityKindTitleFallback = "title-fallback"
	identityConfidenceHigh   = "high"
	identityConfidenceMedium = "medium"

	maxIdentityKeyBytes   = 160
	maxDisplayLabelBytes  = 128
	maxNoteTextBytes      = 8 * 1024
	maxLabelNameBytes     = 64
	maxMessageKindBytes   = 32
	recordIDBytes         = 16
)

type PinRecord struct {
	ID             string `json:"id"`
	ChatKey        string `json:"chat_key"`
	IdentityKind   string `json:"identity_kind"`
	AdapterVersion int    `json:"adapter_version"`
	Confidence     string `json:"confidence"`
	DisplayLabel   string `json:"display_label"`
	Order          int64  `json:"order"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type BookmarkRecord struct {
	ID             string   `json:"id"`
	ChatKey        string   `json:"chat_key"`
	MessageKey     string   `json:"message_key"`
	IdentityKind   string   `json:"identity_kind"`
	AdapterVersion int      `json:"adapter_version"`
	Confidence     string   `json:"confidence"`
	ChatLabel      string   `json:"chat_label"`
	MessageKind    string   `json:"message_kind"`
	MessageTime    string   `json:"message_time,omitempty"`
	Excerpt        string   `json:"excerpt,omitempty"` // off by default; explicit opt-in only
	LabelIDs       []string `json:"label_ids"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

type LabelRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type NoteRecord struct {
	ID        string `json:"id"`
	ChatKey   string `json:"chat_key,omitempty"`
	BookmarkID string `json:"bookmark_id,omitempty"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func validateIdentityFields(chatKey, identityKind string, adapterVersion int, confidence string) error {
	if len(chatKey) == 0 || len(chatKey) > maxIdentityKeyBytes {
		return errors.New("identity key is required and must be bounded")
	}
	// The adapter issues hashed keys in a strict charset; anything else
	// (e.g. a raw JID with '@') was not produced by this adapter version.
	for i := 0; i < len(chatKey); i++ {
		c := chatKey[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == ':', c == '_', c == '-':
		default:
			return errors.New("identity key must be an adapter-issued opaque key")
		}
	}
	switch identityKind {
	case identityKindDataID, identityKindTitleFallback:
	default:
		return errors.New("unsupported identity kind")
	}
	if adapterVersion != adapterVersionCurrent {
		return fmt.Errorf("adapter version %d is not supported (expected %d)", adapterVersion, adapterVersionCurrent)
	}
	switch confidence {
	case identityConfidenceHigh, identityConfidenceMedium:
	default:
		return errors.New("unsupported identity confidence")
	}
	return nil
}

func newRecordID() (string, error) {
	buf := make([]byte, recordIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeMessageKind(kind string) string {
	switch kind {
	case "text", "image", "video", "audio", "document", "sticker":
		return kind
	default:
		return "unknown"
	}
}

func validateLabelColor(color string) error {
	if color == "" {
		return nil
	}
	if len(color) != 7 || color[0] != '#' {
		return errors.New("label color must be an #rrggbb hex value")
	}
	for i := 1; i < len(color); i++ {
		c := color[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return errors.New("label color must be an #rrggbb hex value")
		}
	}
	return nil
}

// identified lets the shared delete helper work over any record type.
type identified interface{ getID() string }

func (p PinRecord) getID() string      { return p.ID }
func (b BookmarkRecord) getID() string { return b.ID }
func (l LabelRecord) getID() string    { return l.ID }
func (n NoteRecord) getID() string     { return n.ID }

// decodeCollection decodes a stored raw collection. Unknown records that fail
// to decode are skipped: corrupt entries never poison the whole collection.
func decodeCollection[T any](raw []json.RawMessage) ([]T, error) {
	items := make([]T, 0, len(raw))
	for _, entry := range raw {
		var item T
		if err := json.Unmarshal(entry, &item); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func encodeCollection[T any](items []T) ([]json.RawMessage, error) {
	raw := make([]json.RawMessage, 0, len(items))
	for i := range items {
		encoded, err := json.Marshal(&items[i])
		if err != nil {
			return nil, err
		}
		raw = append(raw, encoded)
	}
	return raw, nil
}

// saveCollection persists one collection and publishes its change event.
func saveCollection[T any](c *AppStateController, current []json.RawMessage, items []T, event AppEventKind) ([]json.RawMessage, error) {
	encoded, err := encodeCollection(items)
	if err != nil {
		return current, err
	}
	updated := c.store
	switch event {
	case AppEventPinsChanged:
		updated.Pins = encoded
	case AppEventBookmarksChanged:
		updated.Bookmarks = encoded
	case AppEventLabelsChanged:
		updated.Labels = encoded
	case AppEventNotesChanged:
		updated.Notes = encoded
	default:
		return current, errors.New("unsupported collection event")
	}
	if err := saveApplicationStore(updated); err != nil {
		return current, err
	}
	c.store = updated
	c.revision++
	c.publishLocked(event)
	return encoded, nil
}

// --- Controller API ---------------------------------------------------------

func (c *AppStateController) ListPins() ([]PinRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	pins, err := decodeCollection[PinRecord](c.store.Pins)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(pins, func(i, j int) bool { return pins[i].Order < pins[j].Order })
	return pins, nil
}

func (c *AppStateController) AddPin(pin PinRecord) (PinRecord, error) {
	if err := validateIdentityFields(pin.ChatKey, pin.IdentityKind, pin.AdapterVersion, pin.Confidence); err != nil {
		return PinRecord{}, err
	}
	if len(pin.DisplayLabel) > maxDisplayLabelBytes {
		return PinRecord{}, errors.New("pin label exceeds the supported length")
	}
	pin.DisplayLabel = strings.TrimSpace(pin.DisplayLabel)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	pins, err := decodeCollection[PinRecord](c.store.Pins)
	if err != nil {
		return PinRecord{}, err
	}
	for _, existing := range pins {
		if existing.ChatKey == pin.ChatKey {
			return PinRecord{}, errors.New("this chat is already pinned")
		}
	}
	id, err := newRecordID()
	if err != nil {
		return PinRecord{}, err
	}
	var maxOrder int64 = -1
	for _, existing := range pins {
		if existing.Order > maxOrder {
			maxOrder = existing.Order
		}
	}
	now := nowStamp()
	pin.ID = id
	pin.Order = maxOrder + 1
	pin.CreatedAt = now
	pin.UpdatedAt = now
	pins = append(pins, pin)
	if _, err := saveCollection(c, c.store.Pins, pins, AppEventPinsChanged); err != nil {
		return PinRecord{}, err
	}
	return pin, nil
}

func (c *AppStateController) UpdatePin(id string, mutate func(*PinRecord)) (PinRecord, error) {
	if mutate == nil {
		return PinRecord{}, errors.New("pin update is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	pins, err := decodeCollection[PinRecord](c.store.Pins)
	if err != nil {
		return PinRecord{}, err
	}
	for i := range pins {
		if pins[i].ID != id {
			continue
		}
		before := pins[i]
		mutate(&pins[i])
		if len(pins[i].DisplayLabel) > maxDisplayLabelBytes {
			return PinRecord{}, errors.New("pin label exceeds the supported length")
		}
		pins[i].DisplayLabel = strings.TrimSpace(pins[i].DisplayLabel)
		pins[i].ID = before.ID
		pins[i].ChatKey = before.ChatKey
		pins[i].IdentityKind = before.IdentityKind
		pins[i].AdapterVersion = before.AdapterVersion
		pins[i].Confidence = before.Confidence
		pins[i].Order = before.Order
		pins[i].CreatedAt = before.CreatedAt
		pins[i].UpdatedAt = nowStamp()
		if _, err := saveCollection(c, c.store.Pins, pins, AppEventPinsChanged); err != nil {
			return PinRecord{}, err
		}
		return pins[i], nil
	}
	return PinRecord{}, errors.New("pin not found")
}

func (c *AppStateController) ReorderPins(orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return errors.New("pin order is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	pins, err := decodeCollection[PinRecord](c.store.Pins)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(orderedIDs))
	position := make(map[string]int64, len(orderedIDs))
	for i, listID := range orderedIDs {
		if seen[listID] {
			return errors.New("duplicate pin in requested order")
		}
		seen[listID] = true
		position[listID] = int64(i)
	}
	if len(orderedIDs) != len(pins) {
		return errors.New("pin order must cover every pinned chat")
	}
	changed := false
	for i := range pins {
		want, ok := position[pins[i].ID]
		if !ok {
			return errors.New("pin order references an unknown pin")
		}
		if pins[i].Order != want {
			pins[i].Order = want
			pins[i].UpdatedAt = nowStamp()
			changed = true
		}
	}
	if !changed {
		return nil
	}
	_, err = saveCollection(c, c.store.Pins, pins, AppEventPinsChanged)
	return err
}

func (c *AppStateController) RemovePin(id string) error {
	return removeFromCollection(c, id, AppEventPinsChanged, func() []json.RawMessage { return c.store.Pins },
		func(raw []json.RawMessage) ([]PinRecord, error) { return decodeCollection[PinRecord](raw) })
}

func (c *AppStateController) ClearPins() error {
	return c.clearCollection(AppEventPinsChanged, func() []json.RawMessage { return c.store.Pins })
}

// removeFromCollection deletes one record by ID from a typed collection and
// persists the remainder. Callers supply collection accessors; the work runs
// under the controller lock like every other mutation.
func removeFromCollection[T identified](c *AppStateController, id string, event AppEventKind, load func() []json.RawMessage, decode func([]json.RawMessage) ([]T, error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	items, err := decode(load())
	if err != nil {
		return err
	}
	kept := items[:0:0]
	found := false
	for _, item := range items {
		if item.getID() == id {
			found = true
			continue
		}
		kept = append(kept, item)
	}
	if !found {
		return errors.New("record not found")
	}
	_, err = saveCollection(c, load(), kept, event)
	return err
}

func (c *AppStateController) ListBookmarks() ([]BookmarkRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(bookmarks, func(i, j int) bool { return bookmarks[i].CreatedAt > bookmarks[j].CreatedAt })
	return bookmarks, nil
}

func (c *AppStateController) AddBookmark(bookmark BookmarkRecord) (BookmarkRecord, error) {
	if err := validateIdentityFields(bookmark.ChatKey, bookmark.IdentityKind, bookmark.AdapterVersion, bookmark.Confidence); err != nil {
		return BookmarkRecord{}, err
	}
	if len(bookmark.MessageKey) == 0 || len(bookmark.MessageKey) > maxIdentityKeyBytes {
		return BookmarkRecord{}, errors.New("a bookmark requires a bounded message key")
	}
	// Message bodies are never copied; excerpts are a future explicit
	// opt-in and are rejected outright in this milestone.
	if bookmark.Excerpt != "" {
		return BookmarkRecord{}, errors.New("message excerpts are not supported yet")
	}
	bookmark.MessageKind = normalizeMessageKind(bookmark.MessageKind)
	if len(bookmark.ChatLabel) > maxDisplayLabelBytes {
		return BookmarkRecord{}, errors.New("bookmark chat label exceeds the supported length")
	}
	if len(bookmark.MessageTime) > 64 {
		return BookmarkRecord{}, errors.New("bookmark timestamp exceeds the supported length")
	}
	bookmark.ChatLabel = strings.TrimSpace(bookmark.ChatLabel)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
	if err != nil {
		return BookmarkRecord{}, err
	}
	for _, existing := range bookmarks {
		if existing.ChatKey == bookmark.ChatKey && existing.MessageKey == bookmark.MessageKey {
			return BookmarkRecord{}, errors.New("this message is already bookmarked")
		}
	}
	id, err := newRecordID()
	if err != nil {
		return BookmarkRecord{}, err
	}
	bookmark.ID = id
	bookmark.LabelIDs = []string{}
	bookmark.CreatedAt = nowStamp()
	bookmark.UpdatedAt = bookmark.CreatedAt
	bookmarks = append(bookmarks, bookmark)
	if _, err := saveCollection(c, c.store.Bookmarks, bookmarks, AppEventBookmarksChanged); err != nil {
		return BookmarkRecord{}, err
	}
	return bookmark, nil
}

func (c *AppStateController) RemoveBookmark(id string) error {
	return removeFromCollection(c, id, AppEventBookmarksChanged,
		func() []json.RawMessage { return c.store.Bookmarks },
		func(raw []json.RawMessage) ([]BookmarkRecord, error) { return decodeCollection[BookmarkRecord](raw) })
}

func (c *AppStateController) ClearBookmarks() error {
	return c.clearCollection(AppEventBookmarksChanged, func() []json.RawMessage { return c.store.Bookmarks })
}

func (c *AppStateController) clearCollection(event AppEventKind, load func() []json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if len(load()) == 0 {
		return nil
	}
	_, err := saveCollection(c, load(), []json.RawMessage{}, event)
	return err
}

// --- Bridge JSON handlers ----------------------------------------------------
// The page proposes pin operations through bounded JSON; every handler re-
// validates in Go and returns a safe envelope (never raw identity payloads
// or error internals beyond a short reason).

type pinEnvelope struct {
	OK    bool        `json:"ok"`
	Pin   *PinRecord  `json:"pin,omitempty"`
	Pins  []PinRecord `json:"pins,omitempty"`
	Error string      `json:"error,omitempty"`
}

func pinFailure(err error) string {
	data, _ := json.Marshal(pinEnvelope{OK: false, Error: err.Error()})
	return string(data)
}

func pinAddJSON(raw string) string {
	var request PinRecord
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return pinFailure(errors.New("invalid pin request"))
	}
	pin, err := applicationState.AddPin(request)
	if err != nil {
		return pinFailure(err)
	}
	data, marshalErr := json.Marshal(pinEnvelope{OK: true, Pin: &pin})
	if marshalErr != nil {
		return pinFailure(errors.New("pin could not be encoded"))
	}
	return string(data)
}

func pinListJSON() string {
	pins, err := applicationState.ListPins()
	if err != nil {
		return pinFailure(err)
	}
	data, marshalErr := json.Marshal(pinEnvelope{OK: true, Pins: pins})
	if marshalErr != nil {
		return pinFailure(errors.New("pins could not be encoded"))
	}
	return string(data)
}

func pinRenameJSON(id, alias string) string {
	if len(id) != recordIDBytes*2 {
		return pinFailure(errors.New("invalid pin id"))
	}
	pin, err := applicationState.UpdatePin(id, func(p *PinRecord) { p.DisplayLabel = alias })
	if err != nil {
		return pinFailure(err)
	}
	data, marshalErr := json.Marshal(pinEnvelope{OK: true, Pin: &pin})
	if marshalErr != nil {
		return pinFailure(errors.New("pin could not be encoded"))
	}
	return string(data)
}

func pinReorderJSON(raw string) string {
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return pinFailure(errors.New("invalid pin order"))
	}
	if err := applicationState.ReorderPins(ids); err != nil {
		return pinFailure(err)
	}
	return pinListJSON()
}

func pinRemoveJSON(id string) string {
	if len(id) != recordIDBytes*2 {
		return pinFailure(errors.New("invalid pin id"))
	}
	if err := applicationState.RemovePin(id); err != nil {
		return pinFailure(err)
	}
	return pinListJSON()
}

func pinClearJSON() string {
	if err := applicationState.ClearPins(); err != nil {
		return pinFailure(err)
	}
	return pinListJSON()
}

func bookmarkEnvelopeStruct(ok bool, bookmarks []BookmarkRecord, errValue error) string {
	if errValue != nil {
		data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": errValue.Error()})
		return string(data)
	}
	data, marshalErr := json.Marshal(map[string]interface{}{"ok": true, "bookmarks": bookmarks})
	if marshalErr != nil {
		data, _ = json.Marshal(map[string]interface{}{"ok": false, "error": "bookmarks could not be encoded"})
	}
	return string(data)
}

func bookmarkFailure(err error) string {
	data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": err.Error()})
	return string(data)
}

func bookmarkAddJSON(raw string) string {
	var request BookmarkRecord
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return bookmarkFailure(errors.New("invalid bookmark request"))
	}
	bookmark, err := applicationState.AddBookmark(request)
	if err != nil {
		return bookmarkFailure(err)
	}
	return bookmarkEnvelopeStruct(true, []BookmarkRecord{bookmark}, nil)
}

func bookmarkListJSON() string {
	bookmarks, err := applicationState.ListBookmarks()
	if err != nil {
		return bookmarkFailure(err)
	}
	return bookmarkEnvelopeStruct(true, bookmarks, nil)
}

func bookmarkRemoveJSON(id string) string {
	if len(id) != recordIDBytes*2 {
		return bookmarkFailure(errors.New("invalid bookmark id"))
	}
	if err := applicationState.RemoveBookmark(id); err != nil {
		return bookmarkFailure(err)
	}
	return bookmarkListJSON()
}

func bookmarkClearJSON() string {
	if err := applicationState.ClearBookmarks(); err != nil {
		return bookmarkFailure(err)
	}
	return bookmarkListJSON()
}
