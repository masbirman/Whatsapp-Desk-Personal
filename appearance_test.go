package main

import (
	"strings"
	"testing"
)

func TestAppearanceValidation(t *testing.T) {
	valid := defaultAppearance()
	if err := validateAppearance(valid); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if err := validateAppearance(func() AppearanceSettings { a := defaultAppearance(); a.Density = "tiny"; return a }()); err == nil {
		t.Fatal("unknown density must be rejected")
	}
	if err := validateAppearance(func() AppearanceSettings { a := defaultAppearance(); a.ScalePercent = 200; return a }()); err == nil {
		t.Fatal("out-of-range scale must be rejected")
	}
}

func TestCustomCSSValidationRejectsUntrustedConstructs(t *testing.T) {
	cases := map[string]string{
		"remote import":  "@import url('https://evil.example/x.css');",
		"url reference":  "body { background: url(https://evil.example/i.png); }",
		"data url":       "body { background: url(data:text/html,x); }",
		"expression":     "body { width: expression(alert(1)); }",
		"javascript":     "a { color: javascript:alert(1) }",
		"style breakout": "</style><script>alert(1)</script>",
		"unbalanced":     "body { color: red;",
		"oversized":      strings.Repeat("a{}", 40*1024),
	}
	for name, css := range cases {
		if err := validateAppearanceCSS(css); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
	plain := "#side { width: 30%; }\n.main-view { padding: 2px; }"
	if err := validateAppearanceCSS(plain); err != nil {
		t.Fatalf("plain CSS must pass: %v", err)
	}
	if len(plain) >= maxCustomCSSBytes {
		t.Fatal("test css unexpectedly oversized")
	}
}

func TestAppearanceUpdateAndLastKnownGood(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	css := "#side { width: 28%; }"
	updated, err := controller.UpdateAppearance(func(a *AppearanceSettings) {
		a.CustomCSS = css
		a.CSSEnabled = true
	})
	if err != nil {
		t.Fatalf("enable css: %v", err)
	}
	if updated.LastKnownGoodHash != cssHash(css) {
		t.Fatal("last-known-good must track the newest valid CSS")
	}
	// Malformed CSS cannot replace the last-known-good point.
	if _, err := controller.UpdateAppearance(func(a *AppearanceSettings) {
		a.CustomCSS = "body { color: red;"
	}); err == nil {
		t.Fatal("malformed css must be rejected")
	}
	stored := controller.Appearance()
	if stored.LastKnownGoodCSS != css {
		t.Fatalf("last-known-good was clobbered: %+v", stored)
	}
	// Disabled CSS keeps the recovery copy.
	recovered, err := controller.DisableCustomCSS()
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if recovered.CSSEnabled {
		t.Fatal("css must be disabled")
	}
	if recovered.LastKnownGoodCSS != css {
		t.Fatal("recovery copy must survive disable")
	}
	// Corrupt stored appearance degrades to defaults but keeps recovery.
	if got := decodeAppearance([]byte(`{"density":"bogus","scale_percent":999}`)); got.Density != appearanceDensityComfortable || got.ScalePercent != 100 {
		t.Fatalf("corrupt appearance must degrade: %+v", got)
	}
}
