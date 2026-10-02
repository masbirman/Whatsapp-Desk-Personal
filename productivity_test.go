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
