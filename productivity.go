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

// --- Labels -----------------------------------------------------------------

func (c *AppStateController) ListLabels() ([]LabelRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	labels, err := decodeCollection[LabelRecord](c.store.Labels)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
	return labels, nil
}

func (c *AppStateController) AddLabel(name, color string) (LabelRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxLabelNameBytes {
		return LabelRecord{}, errors.New("label name must be 1-64 characters")
	}
	if err := validateLabelColor(color); err != nil {
		return LabelRecord{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	labels, err := decodeCollection[LabelRecord](c.store.Labels)
	if err != nil {
		return LabelRecord{}, err
	}
	for _, existing := range labels {
		if strings.EqualFold(existing.Name, name) {
			return LabelRecord{}, errors.New("a label with this name already exists")
		}
	}
	id, err := newRecordID()
	if err != nil {
		return LabelRecord{}, err
	}
	now := nowStamp()
	label := LabelRecord{ID: id, Name: name, Color: color, CreatedAt: now, UpdatedAt: now}
	labels = append(labels, label)
	if _, err := saveCollection(c, c.store.Labels, labels, AppEventLabelsChanged); err != nil {
		return LabelRecord{}, err
	}
	return label, nil
}

func (c *AppStateController) RenameLabel(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxLabelNameBytes {
		return errors.New("label name must be 1-64 characters")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	labels, err := decodeCollection[LabelRecord](c.store.Labels)
	if err != nil {
		return err
	}
	for i := range labels {
		if labels[i].ID != id {
			continue
		}
		for j, other := range labels {
			if j != i && strings.EqualFold(other.Name, name) {
				return errors.New("a label with this name already exists")
			}
		}
		labels[i].Name = name
		labels[i].UpdatedAt = nowStamp()
		_, err := saveCollection(c, c.store.Labels, labels, AppEventLabelsChanged)
		return err
	}
	return errors.New("label not found")
}

// RemoveLabel detaches the label from every bookmark instead of leaving a
// broken reference behind.
func (c *AppStateController) RemoveLabel(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	labels, err := decodeCollection[LabelRecord](c.store.Labels)
	if err != nil {
		return err
	}
	found := false
	keptLabels := labels[:0:0]
	for _, label := range labels {
		if label.ID == id {
			found = true
			continue
		}
		keptLabels = append(keptLabels, label)
	}
	if !found {
		return errors.New("label not found")
	}
	bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
	if err != nil {
		return err
	}
	detached := false
	for i := range bookmarks {
		filtered := bookmarks[i].LabelIDs[:0]
		for _, labelID := range bookmarks[i].LabelIDs {
			if labelID != id {
				filtered = append(filtered, labelID)
			}
		}
		if len(filtered) != len(bookmarks[i].LabelIDs) {
			bookmarks[i].LabelIDs = filtered
			bookmarks[i].UpdatedAt = nowStamp()
			detached = true
		}
	}
	if _, err := saveCollection(c, c.store.Labels, keptLabels, AppEventLabelsChanged); err != nil {
		return err
	}
	if detached {
		if _, err := saveCollection(c, c.store.Bookmarks, bookmarks, AppEventBookmarksChanged); err != nil {
			return err
		}
	}
	return nil
}

// SetBookmarkLabels replaces a bookmark's label list; every label ID must
// already exist (broken references are rejected, never stored).
func (c *AppStateController) SetBookmarkLabels(bookmarkID string, labelIDs []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	labels, err := decodeCollection[LabelRecord](c.store.Labels)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(labels))
	for _, label := range labels {
		known[label.ID] = true
	}
	seen := make(map[string]bool, len(labelIDs))
	for _, labelID := range labelIDs {
		if !known[labelID] {
			return errors.New("unknown label reference")
		}
		if seen[labelID] {
			return errors.New("duplicate label reference")
		}
		seen[labelID] = true
	}
	bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
	if err != nil {
		return err
	}
	for i := range bookmarks {
		if bookmarks[i].ID != bookmarkID {
			continue
		}
		bookmarks[i].LabelIDs = labelIDs
		bookmarks[i].UpdatedAt = nowStamp()
		_, err := saveCollection(c, c.store.Bookmarks, bookmarks, AppEventBookmarksChanged)
		return err
	}
	return errors.New("bookmark not found")
}

// --- Notes ------------------------------------------------------------------

func (c *AppStateController) ListNotes() ([]NoteRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	notes, err := decodeCollection[NoteRecord](c.store.Notes)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].UpdatedAt > notes[j].UpdatedAt })
	return notes, nil
}

func (c *AppStateController) AddNote(chatKey, bookmarkID, text string) (NoteRecord, error) {
	if len(text) == 0 || len(text) > maxNoteTextBytes {
		return NoteRecord{}, errors.New("note text must be 1-8192 bytes")
	}
	// A note is tied to at most one anchor; both may be empty for a free note.
	if chatKey != "" && bookmarkID != "" {
		return NoteRecord{}, errors.New("a note anchors to a chat or a bookmark, not both")
	}
	if chatKey != "" || bookmarkID != "" {
		if err := validateIdentityFields(chatKeyOrBookmark(chatKey, bookmarkID), identityKindDataID, adapterVersionCurrent, identityConfidenceHigh); err != nil && chatKey != "" {
			return NoteRecord{}, err
		}
		if chatKey != "" && len(chatKey) > maxIdentityKeyBytes {
			return NoteRecord{}, errors.New("note chat key exceeds the supported length")
		}
		if bookmarkID != "" && len(bookmarkID) != recordIDBytes*2 {
			return NoteRecord{}, errors.New("note bookmark reference is invalid")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if bookmarkID != "" {
		bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
		if err != nil {
			return NoteRecord{}, err
		}
		exists := false
		for _, bookmark := range bookmarks {
			if bookmark.ID == bookmarkID {
				exists = true
				break
			}
		}
		if !exists {
			return NoteRecord{}, errors.New("unknown bookmark reference")
		}
	}
	id, err := newRecordID()
	if err != nil {
		return NoteRecord{}, err
	}
	now := nowStamp()
	note := NoteRecord{ID: id, ChatKey: chatKey, BookmarkID: bookmarkID, Text: text, CreatedAt: now, UpdatedAt: now}
	notes, err := decodeCollection[NoteRecord](c.store.Notes)
	if err != nil {
		return NoteRecord{}, err
	}
	notes = append(notes, note)
	if _, err := saveCollection(c, c.store.Notes, notes, AppEventNotesChanged); err != nil {
		return NoteRecord{}, err
	}
	return note, nil
}

func chatKeyOrBookmark(chatKey, bookmarkID string) string {
	if chatKey != "" {
		return chatKey
	}
	return bookmarkID
}

func (c *AppStateController) UpdateNoteText(id, text string) error {
	if len(text) == 0 || len(text) > maxNoteTextBytes {
		return errors.New("note text must be 1-8192 bytes")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	notes, err := decodeCollection[NoteRecord](c.store.Notes)
	if err != nil {
		return err
	}
	for i := range notes {
		if notes[i].ID != id {
			continue
		}
		notes[i].Text = text
		notes[i].UpdatedAt = nowStamp()
		_, err := saveCollection(c, c.store.Notes, notes, AppEventNotesChanged)
		return err
	}
	return errors.New("note not found")
}

func (c *AppStateController) RemoveNote(id string) error {
	return removeFromCollection(c, id, AppEventNotesChanged,
		func() []json.RawMessage { return c.store.Notes },
		func(raw []json.RawMessage) ([]NoteRecord, error) { return decodeCollection[NoteRecord](raw) })
}

func (c *AppStateController) ClearNotes() error {
	return c.clearCollection(AppEventNotesChanged, func() []json.RawMessage { return c.store.Notes })
}

func (c *AppStateController) ClearLabels() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if len(c.store.Labels) == 0 {
		return nil
	}
	// Clearing labels also detaches every bookmark reference.
	bookmarks, err := decodeCollection[BookmarkRecord](c.store.Bookmarks)
	if err == nil {
		for i := range bookmarks {
			if len(bookmarks[i].LabelIDs) > 0 {
				bookmarks[i].LabelIDs = []string{}
				bookmarks[i].UpdatedAt = nowStamp()
			}
		}
		if _, err := saveCollection(c, c.store.Bookmarks, bookmarks, AppEventBookmarksChanged); err != nil {
			return err
		}
	}
	_, err = saveCollection(c, c.store.Labels, []LabelRecord{}, AppEventLabelsChanged)
	return err
}

// --- Label/note/filter bridge handlers ---------------------------------------

func labelEnvelope(ok bool, labels []LabelRecord, errValue error) string {
	if errValue != nil {
		data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": errValue.Error()})
		return string(data)
	}
	data, marshalErr := json.Marshal(map[string]interface{}{"ok": true, "labels": labels})
	if marshalErr != nil {
		data, _ = json.Marshal(map[string]interface{}{"ok": false, "error": "labels could not be encoded"})
	}
	return string(data)
}

func labelFailure(err error) string {
	data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": err.Error()})
	return string(data)
}

func labelAddJSON(raw string) string {
	var request struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return labelFailure(errors.New("invalid label request"))
	}
	label, err := applicationState.AddLabel(request.Name, request.Color)
	if err != nil {
		return labelFailure(err)
	}
	return labelEnvelope(true, []LabelRecord{label}, nil)
}

func labelListJSON() string {
	labels, err := applicationState.ListLabels()
	if err != nil {
		return labelFailure(err)
	}
	return labelEnvelope(true, labels, nil)
}

func labelRenameJSON(id, name string) string {
	if len(id) != recordIDBytes*2 {
		return labelFailure(errors.New("invalid label id"))
	}
	if err := applicationState.RenameLabel(id, name); err != nil {
		return labelFailure(err)
	}
	return labelListJSON()
}

func labelRemoveJSON(id string) string {
	if len(id) != recordIDBytes*2 {
		return labelFailure(errors.New("invalid label id"))
	}
	if err := applicationState.RemoveLabel(id); err != nil {
		return labelFailure(err)
	}
	return labelListJSON()
}

func labelClearJSON() string {
	if err := applicationState.ClearLabels(); err != nil {
		return labelFailure(err)
	}
	return labelListJSON()
}

func bookmarkSetLabelsJSON(bookmarkID, raw string) string {
	if len(bookmarkID) != recordIDBytes*2 {
		return bookmarkFailure(errors.New("invalid bookmark id"))
	}
	var labelIDs []string
	if err := json.Unmarshal([]byte(raw), &labelIDs); err != nil {
		return bookmarkFailure(errors.New("invalid label reference list"))
	}
	if err := applicationState.SetBookmarkLabels(bookmarkID, labelIDs); err != nil {
		return bookmarkFailure(err)
	}
	return bookmarkListJSON()
}

func noteEnvelope(ok bool, notes []NoteRecord, errValue error) string {
	if errValue != nil {
		data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": errValue.Error()})
		return string(data)
	}
	data, marshalErr := json.Marshal(map[string]interface{}{"ok": true, "notes": notes})
	if marshalErr != nil {
		data, _ = json.Marshal(map[string]interface{}{"ok": false, "error": "notes could not be encoded"})
	}
	return string(data)
}

func noteFailure(err error) string {
	data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": err.Error()})
	return string(data)
}

func noteAddJSON(raw string) string {
	var request struct {
		ChatKey    string `json:"chat_key"`
		BookmarkID string `json:"bookmark_id"`
		Text       string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return noteFailure(errors.New("invalid note request"))
	}
	note, err := applicationState.AddNote(request.ChatKey, request.BookmarkID, request.Text)
	if err != nil {
		return noteFailure(err)
	}
	return noteEnvelope(true, []NoteRecord{note}, nil)
}

func noteListJSON() string {
	notes, err := applicationState.ListNotes()
	if err != nil {
		return noteFailure(err)
	}
	return noteEnvelope(true, notes, nil)
}

func noteUpdateJSON(id, text string) string {
	if len(id) != recordIDBytes*2 {
		return noteFailure(errors.New("invalid note id"))
	}
	if err := applicationState.UpdateNoteText(id, text); err != nil {
		return noteFailure(err)
	}
	return noteListJSON()
}

func noteRemoveJSON(id string) string {
	if len(id) != recordIDBytes*2 {
		return noteFailure(errors.New("invalid note id"))
	}
	if err := applicationState.RemoveNote(id); err != nil {
		return noteFailure(err)
	}
	return noteListJSON()
}

func noteClearJSON() string {
	if err := applicationState.ClearNotes(); err != nil {
		return noteFailure(err)
	}
	return noteListJSON()
}
