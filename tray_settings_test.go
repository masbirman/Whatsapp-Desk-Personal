package main

import (
	"os"
	"strings"
	"testing"
)

func TestTraySettingsBridgeAndUIWired(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	script := getInitScript("test-agent")
	for _, want := range []string{
		"getTraySettingsNative",
		"wa-tray-status",
		"wa-tray-toggles",
		"Tray-hide controls are available on Windows.",
		"lock_on_tray ? ' · tray'",
	} {
		if !strings.Contains(script, want) && !strings.Contains(string(source), want) {
			t.Errorf("tray settings UI is missing %q", want)
		}
	}
	for _, file := range []string{"app_linux.go", "app_windows.go"} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"getTraySettingsNative"`) ||
			!strings.Contains(string(content), `"setTraySettingsNative"`) {
			t.Errorf("%s must bind tray settings state", file)
		}
	}
}

func TestLockOnTrayEngagesOnHideToTray(t *testing.T) {
	source, err := os.ReadFile("native_tray_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	if !strings.Contains(content, "lockOnTrayConfigured()") {
		t.Fatal("hide-to-tray must consult the LockOnTray policy")
	}
	if strings.Count(content, "applicationState.MarkLocked()") < 2 {
		t.Fatal("both minimize-to-tray and close-to-tray paths must engage the lock")
	}
}
