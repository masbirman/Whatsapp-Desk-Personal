package main

import (
	"strings"
	"testing"
)

func TestExternalLinkBridgeRequestValidation(t *testing.T) {
	for _, raw := range []string{
		"https://web.whatsapp.com/",
		"http://example.org/path?q=one",
	} {
		if _, err := newExternalLinkRequest(raw); err != nil {
			t.Errorf("valid URL %q rejected: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"javascript:alert(1)", "file:///etc/passwd", "//example.org/path",
		"https://user:password@example.org/", "https://example.org/\r\nheader: value",
		"https://" + strings.Repeat("a", maxBridgeURLBytes),
	} {
		if _, err := newExternalLinkRequest(raw); err == nil {
			t.Errorf("invalid URL %q was accepted", raw)
		}
	}
}

func TestTypedBridgeRequestValidation(t *testing.T) {
	for _, theme := range []string{"dark", "light", "system"} {
		if _, err := newThemeChangeRequest(theme); err != nil {
			t.Errorf("theme %q rejected: %v", theme, err)
		}
	}
	if _, err := newThemeChangeRequest("solarized"); err == nil {
		t.Fatal("unsupported theme was accepted")
	}
	for _, language := range []string{"auto", "en", "en-US", "id-ID"} {
		if _, err := newSpellCheckLanguageRequest(language); err != nil {
			t.Errorf("language %q rejected: %v", language, err)
		}
	}
	for _, language := range []string{"", "en_US", "en--US", "../../tmp", strings.Repeat("a", 65)} {
		if _, err := newSpellCheckLanguageRequest(language); err == nil {
			t.Errorf("invalid language %q was accepted", language)
		}
	}
	if _, err := newSaveFileRequest("", "data:abc"); err == nil {
		t.Fatal("empty filename was accepted")
	}
	if _, err := newOpenLocalFileRequest(strings.Repeat("a", maxBridgePathBytes+1)); err == nil {
		t.Fatal("oversized path was accepted")
	}
	if _, err := newWindowSizeRequest(449, 320); err == nil {
		t.Fatal("undersized window request was accepted")
	}
	if _, err := newWindowSizeRequest(450, maxBridgeDimension+1); err == nil {
		t.Fatal("oversized window request was accepted")
	}
	if _, err := newUnreadBadgeRequest(strings.Repeat("9", 33)); err == nil {
		t.Fatal("oversized unread badge was accepted")
	}
	if _, err := newNativeNotificationRequest(strings.Repeat("x", maxBridgeNotificationTitle+1), "body"); err == nil {
		t.Fatal("oversized notification title was accepted")
	}

	previousLimit := maxAttachmentBytes
	maxAttachmentBytes = 1
	t.Cleanup(func() { maxAttachmentBytes = previousLimit })
	if _, err := newSaveFileRequest("safe.txt", strings.Repeat("A", 5000)); err == nil {
		t.Fatal("oversized base64 payload was accepted")
	}
}
