package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testNotificationTime(hour, minute int) time.Time {
	return time.Date(2026, 10, 2, hour, minute, 0, 0, time.Local)
}

func mustEvent(t *testing.T, title, body string, focused bool) NotificationEvent {
	t.Helper()
	event, err := newNotificationEvent(title, body, "", "unknown", focused)
	if err != nil {
		t.Fatalf("newNotificationEvent: %v", err)
	}
	return event
}

func TestDefaultNotificationPolicyFailsSafe(t *testing.T) {
	policy := defaultNotificationPolicy(true)
	if err := validateNotificationPolicy(policy); err != nil {
		t.Fatalf("default policy must validate: %v", err)
	}
	if !policy.Enabled || policy.SenderVisibility != SenderShow || policy.BodyVisibility != BodyShow {
		t.Fatalf("unexpected defaults: %+v", policy)
	}
	// The locked floor must never have an "allow" variant.
	if policy.LockedBehavior != LockedGeneric {
		t.Fatalf("locked default must be generic, got %q", policy.LockedBehavior)
	}
}

func TestDecodeNotificationPolicyHandlesLegacyStore(t *testing.T) {
	// Empty and `{}` payloads come from earlier store versions.
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`{}`)} {
		policy, err := decodeNotificationPolicy(raw, false)
		if err != nil {
			t.Fatalf("decode(%s): %v", raw, err)
		}
		if policy.Enabled {
			t.Fatalf("legacy disabled master switch must stay disabled, got %+v", policy)
		}
		if policy.SenderVisibility != SenderShow {
			t.Fatalf("unconfigured policy must seed sender defaults, got %+v", policy)
		}
	}
	// A configured policy keeps its values.
	configured := json.RawMessage(`{"enabled":true,"sender_visibility":"generic","body_visibility":"hide","focused_behavior":"allow","background_behavior":"generic","locked_behavior":"suppress","quiet_hours":{"enabled":true,"start_local":"22:00","end_local":"07:00"},"sound":{"enabled":false}}`)
	policy, err := decodeNotificationPolicy(configured, true)
	if err != nil {
		t.Fatalf("decode configured: %v", err)
	}
	if policy.SenderVisibility != SenderGeneric || policy.BodyVisibility != BodyHide ||
		policy.LockedBehavior != LockedSuppress || policy.Sound.Enabled {
		t.Fatalf("configured policy not preserved: %+v", policy)
	}
	// Corrupt JSON is an error; the controller degrades instead of trusting it.
	if _, err := decodeNotificationPolicy(json.RawMessage(`{"sender_visibility":"bogus"}`), true); err == nil {
		t.Fatal("invalid enum must be rejected")
	}
}

func TestNotificationEventValidation(t *testing.T) {
	if _, err := newNotificationEvent("hi", "body", "tag", "private", true); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	longTitle := strings.Repeat("a", maxBridgeNotificationTitle+1)
	if _, err := newNotificationEvent(longTitle, "", "", "unknown", false); err == nil {
		t.Fatal("over-long title must be rejected")
	}
	if _, err := newNotificationEvent("t", "b", strings.Repeat("x", maxNotificationTagBytes+1), "unknown", false); err == nil {
		t.Fatal("over-long tag must be rejected")
	}
	if _, err := newNotificationEvent("t", "b", "", "secret", false); err == nil {
		t.Fatal("unknown chat type enum must be rejected")
	}
	if _, err := newNotificationEvent("ti\n tle", "b", "", "unknown", false); err == nil {
		t.Fatal("control characters must be rejected")
	}
}

func TestResolveNotificationRedactionMatrix(t *testing.T) {
	event := mustEvent(t, "Alice", "secret message text", false)
	cases := []struct {
		name        string
		policy      NotificationPolicy
		locked      bool
		checkTitle  func(t *testing.T, p NotificationPresentation)
		checkBody   func(t *testing.T, p NotificationPresentation)
		suppressed  bool
	}{
		{
			name:       "full show allows content in background",
			policy:     defaultNotificationPolicy(true),
			checkTitle: expectTitle("Alice"),
			checkBody:  expectBody("secret message text"),
		},
		{
			name: "hidden sender drops title but keeps body",
			policy: func() NotificationPolicy {
				p := defaultNotificationPolicy(true)
				p.SenderVisibility = SenderHide
				return p
			}(),
			checkTitle: expectTitle(""),
			checkBody:  expectBody("secret message text"),
		},
		{
			name: "generic sender replaces title but keeps body",
			policy: func() NotificationPolicy {
				p := defaultNotificationPolicy(true)
				p.SenderVisibility = SenderGeneric
				return p
			}(),
			checkTitle: expectTitle(notificationGenericTitle),
			checkBody:  expectBody("secret message text"),
		},
		{
			name: "hidden body drops message but keeps sender",
			policy: func() NotificationPolicy {
				p := defaultNotificationPolicy(true)
				p.BodyVisibility = BodyHide
				return p
			}(),
			checkTitle: expectTitle("Alice"),
			checkBody:  expectBody(""),
		},
		{
			name: "background generic replaces everything",
			policy: func() NotificationPolicy {
				p := defaultNotificationPolicy(true)
				p.BackgroundBehavior = DeliveryGeneric
				return p
			}(),
			checkTitle: expectTitle(notificationGenericTitle),
			checkBody:  expectBody(notificationGenericBody),
		},
		{
			name: "disabled master switch suppresses",
			policy: func() NotificationPolicy {
				p := defaultNotificationPolicy(true)
				p.Enabled = false
				return p
			}(),
			suppressed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := resolveNotificationPresentation(tc.policy, event, tc.locked, testNotificationTime(12, 0))
			if p.Suppressed != tc.suppressed {
				t.Fatalf("suppressed=%v, want %v", p.Suppressed, tc.suppressed)
			}
			if tc.checkTitle != nil {
				tc.checkTitle(t, p)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, p)
			}
		})
	}
}

func expectTitle(want string) func(t *testing.T, p NotificationPresentation) {
	return func(t *testing.T, p NotificationPresentation) {
		t.Helper()
		if p.Title != want {
			t.Fatalf("title=%q, want %q", p.Title, want)
		}
	}
}

func expectBody(want string) func(t *testing.T, p NotificationPresentation) {
	return func(t *testing.T, p NotificationPresentation) {
		t.Helper()
		if p.Body != want {
			t.Fatalf("body=%q, want %q", p.Body, want)
		}
	}
}

func TestResolveNotificationLockFloorNeverLeaks(t *testing.T) {
	event := mustEvent(t, "Alice", "secret message text", false)
	for _, behavior := range []NotificationLockedBehavior{LockedGeneric, LockedSuppress} {
		for _, visibility := range []NotificationSenderVisibility{SenderShow, SenderHide, SenderGeneric} {
			policy := defaultNotificationPolicy(true)
			policy.LockedBehavior = behavior
			policy.SenderVisibility = visibility
			presentation := resolveNotificationPresentation(policy, event, true, testNotificationTime(12, 0))
			if behavior == LockedSuppress && !presentation.Suppressed {
				t.Fatal("locked suppress must suppress")
			}
			if behavior == LockedGeneric && !presentation.isContentFree() {
				t.Fatalf("locked generic must be content-free, got title=%q body=%q", presentation.Title, presentation.Body)
			}
		}
	}
}

func TestResolveNotificationFocusAndQuietHours(t *testing.T) {
	policy := defaultNotificationPolicy(true) // focused=suppress, background=allow
	focusedEvent := mustEvent(t, "Alice", "secret message text", true)
	if p := resolveNotificationPresentation(policy, focusedEvent, false, testNotificationTime(12, 0)); !p.Suppressed {
		t.Fatal("focused event must be suppressed by default")
	}
	if p := resolveNotificationPresentation(policy, focusedEvent, false, testNotificationTime(12, 0)); !p.Suppressed {
		t.Fatal("focused default changed unexpectedly")
	}

	// Quiet hours suppress everything, including background content.
	quiet := defaultNotificationPolicy(true)
	quiet.QuietHours = NotificationQuietHours{Enabled: true, Start: "22:00", End: "07:00"}
	backgroundEvent := mustEvent(t, "Alice", "secret message text", false)
	if p := resolveNotificationPresentation(quiet, backgroundEvent, false, testNotificationTime(23, 30)); !p.Suppressed {
		t.Fatal("quiet hours overnight-start must suppress")
	}
	if p := resolveNotificationPresentation(quiet, backgroundEvent, false, testNotificationTime(3, 0)); !p.Suppressed {
		t.Fatal("quiet hours overnight-end must suppress")
	}
	if p := resolveNotificationPresentation(quiet, backgroundEvent, false, testNotificationTime(12, 0)).Suppressed; p {
		t.Fatal("outside quiet hours must not suppress")
	}

	// Same-day window.
	dayQuiet := defaultNotificationPolicy(true)
	dayQuiet.QuietHours = NotificationQuietHours{Enabled: true, Start: "09:00", End: "17:00"}
	if p := resolveNotificationPresentation(dayQuiet, backgroundEvent, false, testNotificationTime(12, 0)); !p.Suppressed {
		t.Fatal("same-day quiet window must suppress")
	}
	if p := resolveNotificationPresentation(dayQuiet, backgroundEvent, false, testNotificationTime(18, 0)).Suppressed; p {
		t.Fatal("outside same-day window must not suppress")
	}

	// Invalid quiet window data never suppresses silently.
	broken := defaultNotificationPolicy(true)
	broken.QuietHours = NotificationQuietHours{Enabled: true, Start: "bad", End: "17:00"}
	if p := resolveNotificationPresentation(broken, backgroundEvent, false, testNotificationTime(12, 0)).Suppressed; p {
		t.Fatal("invalid quiet window must not suppress")
	}
}

func TestMergeNotificationProfileOverrides(t *testing.T) {
	base := defaultNotificationPolicy(true)
	enabled := true
	disabled := false

	merged := mergeNotificationProfileOverrides(base, ProfileNotificationOverrides{
		Enabled:            &disabled,
		SenderVisibility:   SenderGeneric,
		BodyVisibility:     BodyHide,
		BackgroundBehavior: DeliveryGeneric,
		SoundEnabled:       &disabled,
	})
	if merged.Enabled || merged.SenderVisibility != SenderGeneric || merged.BodyVisibility != BodyHide ||
		merged.BackgroundBehavior != DeliveryGeneric || merged.Sound.Enabled {
		t.Fatalf("overrides not applied: %+v", merged)
	}
	if merged.LockedBehavior != LockedGeneric {
		t.Fatalf("unrelated fields must keep defaults: %+v", merged)
	}

	// Enabling quiet hours keeps the window configured on the base policy;
	// enabling without any configured times degrades fail-safe instead.
	withWindow := base
	withWindow.QuietHours = NotificationQuietHours{Start: "22:00", End: "07:00"}
	quietOn := mergeNotificationProfileOverrides(withWindow, ProfileNotificationOverrides{
		QuietHoursEnabled: &enabled,
	})
	if !quietOn.QuietHours.Enabled || quietOn.QuietHours.Start != "22:00" {
		t.Fatalf("quiet hours override not applied: %+v", quietOn)
	}
	noWindow := mergeNotificationProfileOverrides(base, ProfileNotificationOverrides{
		QuietHoursEnabled: &enabled,
	})
	if noWindow.SenderVisibility != SenderGeneric || noWindow.BodyVisibility != BodyHide {
		t.Fatalf("quiet hours enabled without a window must degrade content-free, got %+v", noWindow)
	}

	// Overrides can never weaken the locked floor.
	weakened := mergeNotificationProfileOverrides(base, ProfileNotificationOverrides{
		LockedBehavior: "allow",
	})
	if weakened.LockedBehavior != LockedGeneric {
		t.Fatalf("locked floor must resist weakening, got %q", weakened.LockedBehavior)
	}

	// Invalid merged values degrade instead of trusting the profile record.
	broken := mergeNotificationProfileOverrides(base, ProfileNotificationOverrides{
		SenderVisibility: "not-a-mode",
	})
	if broken.SenderVisibility != SenderGeneric || broken.BodyVisibility != BodyHide {
		t.Fatalf("invalid overrides must degrade to content-free policy, got %+v", broken)
	}
}

func TestResolveNotificationPresentationControllerPath(t *testing.T) {
	useTempAppConfig(t)
	// Uses the real controller against a temporary store directory.
	controller := NewAppStateController()
	event, err := newNotificationEvent("Alice", "secret message text", "tag", "unknown", false)
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	presentation := controller.ResolveNotificationPresentation(event)
	if presentation.Suppressed {
		t.Fatal("default policy should deliver background notifications")
	}
	if presentation.Title != "Alice" || presentation.Body != "secret message text" {
		t.Fatalf("unexpected presentation: %+v", presentation)
	}

	// Locked state flips the presentation to content-free via the real path.
	controller.MarkLocked()
	locked := controller.ResolveNotificationPresentation(event)
	if !locked.isContentFree() {
		t.Fatalf("locked controller presentation must be content-free, got %+v", locked)
	}
}

func TestUpdateNotificationPolicyPersistsAndValidates(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	if err := controller.UpdateNotificationPolicy(func(p *NotificationPolicy) {
		p.SenderVisibility = SenderGeneric
		p.BodyVisibility = BodyHide
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	policy := controller.EffectiveNotificationPolicy()
	if policy.SenderVisibility != SenderGeneric || policy.BodyVisibility != BodyHide {
		t.Fatalf("update not persisted: %+v", policy)
	}
	if err := controller.UpdateNotificationPolicy(func(p *NotificationPolicy) {
		p.SenderVisibility = "bogus"
	}); err == nil {
		t.Fatal("invalid policy must be rejected")
	}
	// The stored policy is untouched after a rejected update.
	if got := controller.EffectiveNotificationPolicy().SenderVisibility; got != SenderGeneric {
		t.Fatalf("rejected update must not change stored policy, got %q", got)
	}
}

func TestNotificationPolicySummary(t *testing.T) {
	if got := NotificationPolicySummary(defaultNotificationPolicy(true)); got != "focused=suppress" {
		t.Fatalf("unexpected default summary %q", got)
	}
	off := defaultNotificationPolicy(true)
	off.Enabled = false
	if got := NotificationPolicySummary(off); got != "off" {
		t.Fatalf("unexpected off summary %q", got)
	}
}
