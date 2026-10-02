package main

import (
	"strings"
	"testing"
)

func validPin() PinRecord {
	return PinRecord{
		ChatKey:        "chat:1a2b3c4d5e6f7788",
		IdentityKind:   identityKindDataID,
		AdapterVersion: adapterVersionCurrent,
		Confidence:     identityConfidenceHigh,
		DisplayLabel:   "Budi",
	}
}

func TestPinLifecyclePersistsAcrossReload(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	first, err := controller.AddPin(validPin())
	if err != nil {
		t.Fatalf("AddPin: %v", err)
	}
	second, err := controller.AddPin(func() PinRecord {
		p := validPin()
		p.ChatKey = "chat:8877665544332211"
		p.IdentityKind = identityKindTitleFallback
		p.Confidence = identityConfidenceMedium
		return p
	}())
	if err != nil {
		t.Fatalf("AddPin second: %v", err)
	}
	if first.Order >= second.Order {
		t.Fatalf("orders must increase: %d then %d", first.Order, second.Order)
	}
	if first.ID == second.ID {
		t.Fatal("record IDs must be unique")
	}

	// A fresh controller must see the same records from disk.
	reloaded := NewAppStateController()
	pins, err := reloaded.ListPins()
	if err != nil {
		t.Fatalf("ListPins: %v", err)
	}
	if len(pins) != 2 || pins[0].ID != first.ID || pins[1].ID != second.ID {
		t.Fatalf("unexpected reload: %+v", pins)
	}
}

func TestPinRejectsUntrustedIdentity(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	cases := []struct {
		name string
		pin  PinRecord
	}{
		{"unknown adapter version", func() PinRecord { p := validPin(); p.AdapterVersion = 1; return p }()},
		{"unsupported kind", func() PinRecord { p := validPin(); p.IdentityKind = "dom-path"; return p }()},
		{"unsupported confidence", func() PinRecord { p := validPin(); p.Confidence = "guessed"; return p }()},
		{"empty key", func() PinRecord { p := validPin(); p.ChatKey = ""; return p }()},
		{"non-opaque key", func() PinRecord { p := validPin(); p.ChatKey = "6281234567890@c.us"; return p }()},
		{"oversized label", func() PinRecord { p := validPin(); p.DisplayLabel = strings.Repeat("a", maxDisplayLabelBytes+1); return p }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := controller.AddPin(tc.pin); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	pins, err := controller.ListPins()
	if err != nil || len(pins) != 0 {
		t.Fatalf("rejected pins must not persist: %v %d", err, len(pins))
	}
}

func TestPinDuplicateAndMutations(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	pin, err := controller.AddPin(validPin())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AddPin(validPin()); err == nil {
		t.Fatal("duplicate chat key must be rejected")
	}
	renamed, err := controller.UpdatePin(pin.ID, func(p *PinRecord) { p.DisplayLabel = "Alias baru" })
	if err != nil {
		t.Fatalf("UpdatePin: %v", err)
	}
	if renamed.DisplayLabel != "Alias baru" {
		t.Fatalf("alias not applied: %+v", renamed)
	}
	if renamed.ChatKey != pin.ChatKey || renamed.IdentityKind != pin.IdentityKind {
		t.Fatal("identity fields must be immutable through UpdatePin")
	}
	// Aliases longer than the cap are rejected.
	if _, err := controller.UpdatePin(pin.ID, func(p *PinRecord) { p.DisplayLabel = strings.Repeat("x", maxDisplayLabelBytes+1) }); err == nil {
		t.Fatal("oversized alias must be rejected")
	}
	if err := controller.RemovePin(pin.ID); err != nil {
		t.Fatalf("RemovePin: %v", err)
	}
	if err := controller.RemovePin(pin.ID); err == nil {
		t.Fatal("removing twice must fail")
	}
	pins, _ := controller.ListPins()
	if len(pins) != 0 {
		t.Fatalf("pin not removed: %+v", pins)
	}
}

func TestPinReorderValidatesCompleteOrder(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	a, _ := controller.AddPin(validPin())
	b, _ := controller.AddPin(func() PinRecord { p := validPin(); p.ChatKey = "chat:aaaaaaaaaaaaaaaa"; return p }())
	if err := controller.ReorderPins([]string{b.ID, a.ID}); err != nil {
		t.Fatalf("ReorderPins: %v", err)
	}
	pins, _ := controller.ListPins()
	if pins[0].ID != b.ID || pins[1].ID != a.ID {
		t.Fatalf("order not applied: %+v", pins)
	}
	if err := controller.ReorderPins([]string{b.ID}); err == nil {
		t.Fatal("partial order must be rejected")
	}
	if err := controller.ReorderPins([]string{b.ID, b.ID}); err == nil {
		t.Fatal("duplicate order entries must be rejected")
	}
	if err := controller.ReorderPins([]string{b.ID, "nonexistent"}); err == nil {
		t.Fatal("unknown IDs must be rejected")
	}
}

func TestClearPinsRemovesEverything(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	for _, key := range []string{"chat:1", "chat:2", "chat:3"} {
		p := validPin()
		p.ChatKey = key
		if _, err := controller.AddPin(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := controller.ClearPins(); err != nil {
		t.Fatalf("ClearPins: %v", err)
	}
	pins, _ := controller.ListPins()
	if len(pins) != 0 {
		t.Fatalf("pins not cleared: %+v", pins)
	}
	// Clearing an empty collection is a harmless no-op.
	if err := controller.ClearPins(); err != nil {
		t.Fatalf("second clear must be a no-op: %v", err)
	}
}

func TestLabelLifecycleAndCascade(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	red, err := controller.AddLabel("Kantor", "#ff0000")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if _, err := controller.AddLabel("kantor", "#00ff00"); err == nil {
		t.Fatal("case-insensitive duplicate names must be rejected")
	}
	if _, err := controller.AddLabel("Bad color", "red"); err == nil {
		t.Fatal("non-hex colors must be rejected")
	}
	if _, err := controller.AddLabel(strings.Repeat("x", maxLabelNameBytes+1), "#ff0000"); err == nil {
		t.Fatal("oversized names must be rejected")
	}

	bookmark, err := controller.AddBookmark(BookmarkRecord{
		ChatKey:        "chat:1111111111111111",
		MessageKey:     "msg:2222222222222222",
		IdentityKind:   identityKindDataID,
		AdapterVersion: adapterVersionCurrent,
		Confidence:     identityConfidenceHigh,
	})
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if err := controller.SetBookmarkLabels(bookmark.ID, []string{red.ID}); err != nil {
		t.Fatalf("SetBookmarkLabels: %v", err)
	}
	if err := controller.SetBookmarkLabels(bookmark.ID, []string{"unknown"}); err == nil {
		t.Fatal("unknown label references must be rejected")
	}
	bookmarks, _ := controller.ListBookmarks()
	if len(bookmarks) != 1 || len(bookmarks[0].LabelIDs) != 1 {
		t.Fatalf("label not stored: %+v", bookmarks)
	}

	// Removing the label detaches it from bookmarks instead of leaving a
	// broken reference.
	if err := controller.RemoveLabel(red.ID); err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}
	bookmarks, _ = controller.ListBookmarks()
	if len(bookmarks[0].LabelIDs) != 0 {
		t.Fatalf("label reference not detached: %+v", bookmarks[0])
	}
	labels, _ := controller.ListLabels()
	if len(labels) != 0 {
		t.Fatalf("label not removed: %+v", labels)
	}
}

func TestNoteLifecycleAndBounds(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	note, err := controller.AddNote("chat:1234567890abcdef", "", "Catatan pribadi")
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if _, err := controller.AddNote("", "", ""); err == nil {
		t.Fatal("empty note text must be rejected")
	}
	if _, err := controller.AddNote("", "", strings.Repeat("x", maxNoteTextBytes+1)); err == nil {
		t.Fatal("oversized note text must be rejected")
	}
	if _, err := controller.AddNote("chat:1", "bookmark", "both anchors"); err == nil {
		t.Fatal("dual anchors must be rejected")
	}
	if _, err := controller.AddNote("", "nonexistentbookmark00", "dangling"); err == nil {
		t.Fatal("unknown bookmark references must be rejected")
	}
	if err := controller.UpdateNoteText(note.ID, "Diperbarui"); err != nil {
		t.Fatalf("UpdateNoteText: %v", err)
	}
	notes, _ := controller.ListNotes()
	if len(notes) != 1 || notes[0].Text != "Diperbarui" {
		t.Fatalf("note update not persisted: %+v", notes)
	}
	if err := controller.RemoveNote(note.ID); err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	if _, err := controller.AddNote("", "", "free note"); err != nil {
		t.Fatalf("free note without anchors must be allowed: %v", err)
	}
	if err := controller.ClearNotes(); err != nil {
		t.Fatalf("ClearNotes: %v", err)
	}
	notes, _ = controller.ListNotes()
	if len(notes) != 0 {
		t.Fatalf("notes not cleared: %+v", notes)
	}
}
