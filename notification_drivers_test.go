package main

import (
	"os"
	"strings"
	"testing"
)

func TestBuildNotifySendArgsRejectsOptionInjection(t *testing.T) {
	args := buildNotifySendArgs("--help=evil", "body", "/tmp/icon.png")
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(strings.Join(args[0:1], " "), "--app-name") {
		t.Fatalf("app name must come first: %q", joined)
	}
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 {
		t.Fatalf("arguments must end with a -- separator before content: %q", joined)
	}
	rest := args[sep+1:]
	if len(rest) != 2 || rest[0] != "--help=evil" || rest[1] != "body" {
		t.Fatalf("content must be passed verbatim after --: %q", joined)
	}
	args = buildNotifySendArgs("title", "body", "")
	for _, arg := range args {
		if arg == "/tmp/icon.png" {
			t.Fatalf("empty icon path must not add an --icon flag: %q", strings.Join(args, " "))
		}
	}
}

func TestWindowsToastActivationCannotRevealLockedWindow(t *testing.T) {
	source, err := os.ReadFile("app_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	restore := strings.Index(content, "SW_RESTORE")
	visibility := strings.Index(content, "IsWindowVisible")
	if restore < 0 || visibility < 0 {
		t.Fatalf("single-instance restore must consult IsWindowVisible: restore=%d visibility=%d", restore, visibility)
	}
	checkFn := content[strings.Index(content, "func checkSingleInstance"):]
	if !strings.Contains(checkFn, "IsWindowVisible") {
		t.Fatal("checkSingleInstance must verify window visibility before restoring")
	}
	// The guard must be a visibility check, not just an existence check.
	if !strings.Contains(checkFn, "procIsWindowVisible.Call(hwnd)") {
		t.Fatal("checkSingleInstance must restore only when the window is visible (lock concealment)")
	}
}

func TestWindowsToastSoundHonorsResolvedPolicy(t *testing.T) {
	source, err := os.ReadFile("app_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	if !strings.Contains(content, "notification.Audio = toast.Default") {
		t.Fatal("toast sound must be opt-in based on the resolved policy")
	}
	bind := content[strings.Index(content, `w.Bind("sendNativeNotification"`):]
	for _, want := range []string{"ResolveNotificationPresentation", "presentation.Suppressed"} {
		if !strings.Contains(bind, want) {
			t.Fatalf("windows notification bridge must route through policy resolution (%q)", want)
		}
	}
}

func TestLinuxNotificationBridgeRoutesThroughPolicy(t *testing.T) {
	source, err := os.ReadFile("app_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	bind := content[strings.Index(content, `w.Bind("sendNativeNotification"`):]
	for _, want := range []string{"ResolveNotificationPresentation", "presentation.Suppressed", "presentation.Title"} {
		if !strings.Contains(bind, want) {
			t.Fatalf("linux notification bridge must route through policy resolution (%q)", want)
		}
	}
	if !strings.Contains(content, "nativeNotificationTimeout") {
		t.Fatal("linux notification driver must be bounded by a timeout")
	}
}
