package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsTrayMenuActionsAndLockGuard(t *testing.T) {
	source, err := os.ReadFile("native_tray_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, want := range []string{
		`"Open"`, `"Toggle privacy mode"`, `"Lock now"`,
		`"Desktop notifications"`, `"Control Center"`, `"Quit"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("tray menu is missing %s", want)
		}
	}
	if !strings.Contains(content, "TaskbarCreated") {
		t.Fatal("tray must re-register after an Explorer restart")
	}
	if !strings.Contains(content, "Shell_NotifyIconW") || !strings.Contains(content, "nimDelete") {
		t.Fatal("tray lifecycle must add and remove the Shell icon")
	}
}

func TestWindowsTrayOpenRespectsAppLock(t *testing.T) {
	source, err := os.ReadFile("app_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	start := strings.Index(content, "startWindowsTray(nativeTrayActions{")
	end := strings.Index(content[start:], "\t}, embeddedIconPNG)")
	if start < 0 || end < 0 {
		t.Fatal("tray actions block not found in app_windows.go")
	}
	block := content[start : start+end]
	if !strings.Contains(block, "procIsWindowVisible.Call(hwnd)") {
		t.Fatal("tray Open must check window visibility so it cannot reveal a locked app")
	}
	if !strings.Contains(block, "requestNativeAppLock(hwnd)") {
		t.Fatal("tray Lock action must engage the native app lock")
	}
}
