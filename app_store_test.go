package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func useTempAppConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", root)
	case "darwin":
		t.Setenv("HOME", root)
	default:
		t.Setenv("XDG_CONFIG_HOME", root)
	}
	previousController := applicationState
	applicationState = NewAppStateController()
	t.Cleanup(func() { applicationState = previousController })
	return root
}

func TestLegacySettingsMigrateIdempotentlyToVersionedStore(t *testing.T) {
	root := useTempAppConfig(t)
	downloadDir := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := AppSettings{
		DownloadDir: downloadDir, NotifyOnDownload: false, NotificationsEnabled: false,
		Theme: "light", OrganizeByMonth: true, SpellCheckEnabled: false,
		SpellCheckLang: "id-ID", BlurAvatars: true, LastCrashNotified: 123,
	}
	legacyBytes, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := getSettingsFilePath()
	if err := os.WriteFile(legacyPath, legacyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	windowStatePath := filepath.Join(filepath.Dir(legacyPath), "window_state.json")
	windowState := []byte(`{"width":1200,"height":800}`)
	if err := os.WriteFile(windowStatePath, windowState, 0o600); err != nil {
		t.Fatal(err)
	}

	store, report, err := loadApplicationStore()
	if err != nil {
		t.Fatal(err)
	}
	if report.Source != StoreMigratedLegacy {
		t.Fatalf("migration source = %q, want %q", report.Source, StoreMigratedLegacy)
	}
	if store.SchemaVersion != appStoreSchemaVersion || store.Settings != legacy {
		t.Fatalf("migrated store did not preserve settings: %#v", store)
	}
	if store.CurrentProfile != "normal" || len(store.Profiles) != 5 || len(store.Pins) != 0 {
		t.Fatalf("unexpected initial feature state: profile=%q profiles=%d pins=%d", store.CurrentProfile, len(store.Profiles), len(store.Pins))
	}
	var normalProfile struct {
		Privacy PrivacyPolicy `json:"privacy"`
	}
	if err := json.Unmarshal(store.Profiles[0], &normalProfile); err != nil || !normalProfile.Privacy.Avatars {
		t.Fatalf("legacy avatar privacy setting was not carried into NORMAL profile: %#v err=%v", normalProfile, err)
	}
	afterLegacy, err := os.ReadFile(legacyPath)
	if err != nil || string(afterLegacy) != string(legacyBytes) {
		t.Fatalf("legacy file changed during migration: %v", err)
	}

	second, secondReport, err := loadApplicationStore()
	if err != nil {
		t.Fatal(err)
	}
	if secondReport.Source != StoreLoadedCurrent || second.Settings != legacy {
		t.Fatalf("second load was not idempotent: report=%#v settings=%#v", secondReport, second.Settings)
	}
	afterWindowState, err := os.ReadFile(windowStatePath)
	if err != nil || string(afterWindowState) != string(windowState) {
		t.Fatalf("migration changed existing window state: %q err=%v", afterWindowState, err)
	}
}

func TestApplicationStoreAtomicSaveKeepsValidBackup(t *testing.T) {
	useTempAppConfig(t)
	first := newApplicationStore(defaultAppSettings())
	first.Settings.Theme = "dark"
	if err := saveApplicationStore(first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Settings.Theme = "light"
	if err := saveApplicationStore(second); err != nil {
		t.Fatal(err)
	}

	path := getApplicationStorePath()
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("store permissions are too broad: %o", info.Mode().Perm())
		}
	}

	primary, err := readAndValidateStore(path)
	if err != nil || primary.Settings.Theme != "light" {
		t.Fatalf("primary store = %#v, err=%v", primary.Settings, err)
	}
	backup, err := readAndValidateStore(path + ".bak")
	if err != nil || backup.Settings.Theme != "dark" {
		t.Fatalf("backup store = %#v, err=%v", backup.Settings, err)
	}
}

func TestAtomicStoreWriteFailurePreservesPreviousFile(t *testing.T) {
	useTempAppConfig(t)
	path := getApplicationStorePath()
	previous := []byte("previous valid bytes")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}

	originalReplace := atomicStoreFileReplace
	atomicStoreFileReplace = func(string, string) error { return errors.New("injected replace failure") }
	t.Cleanup(func() { atomicStoreFileReplace = originalReplace })
	if err := writeAtomicFile(path, []byte("incomplete replacement"), 0o600); err == nil {
		t.Fatal("injected atomic replacement failure was ignored")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(previous) {
		t.Fatalf("failed replacement changed previous file: %q err=%v", current, err)
	}
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".whatsappdesk-store-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary write files were not cleaned up: %v err=%v", temporary, err)
	}
}

func TestApplicationStoreRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	root := useTempAppConfig(t)
	path := getApplicationStorePath()
	target := filepath.Join(root, "store-target.json")
	original := []byte(`{"schema_version":99,"future":"keep"}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, _, err := loadApplicationStore(); err == nil {
		t.Fatal("symlinked store path was accepted")
	}
	after, err := os.ReadFile(target)
	if err != nil || string(after) != string(original) {
		t.Fatalf("symlink target changed: %q err=%v", after, err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: info=%v err=%v", info, err)
	}
}

func TestApplicationStoreRecoversBackupAndPreservesCorruptPrimary(t *testing.T) {
	useTempAppConfig(t)
	path := getApplicationStorePath()
	good := newApplicationStore(defaultAppSettings())
	good.Settings.Theme = "light"
	data, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, report, err := loadApplicationStore()
	if err != nil {
		t.Fatal(err)
	}
	if report.Source != StoreRecoveredBackup || store.Settings.Theme != "light" {
		t.Fatalf("did not recover expected backup: report=%#v theme=%q", report, store.Settings.Theme)
	}
	if _, err := readAndValidateStore(path); err != nil {
		t.Fatalf("primary was not restored: %v", err)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt primary was not preserved: matches=%v err=%v", matches, err)
	}
	corrupt, err := os.ReadFile(matches[0])
	if err != nil || string(corrupt) != "{broken" {
		t.Fatalf("preserved corrupt data changed: %q err=%v", corrupt, err)
	}
}

func TestApplicationStoreCorruptFilesRecoverToDefaults(t *testing.T) {
	useTempAppConfig(t)
	path := getApplicationStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{primary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("{backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, report, err := loadApplicationStore()
	if err != nil {
		t.Fatal(err)
	}
	if report.Source != StoreRecoveredDefault || store.Settings.Theme != "dark" {
		t.Fatalf("expected safe defaults: report=%#v theme=%q", report, store.Settings.Theme)
	}
	if _, err := readAndValidateStore(path); err != nil {
		t.Fatalf("safe default store was not written: %v", err)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt primary was not retained: matches=%v err=%v", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil || string(content) != "{primary" {
		t.Fatalf("preserved primary changed: %q err=%v", content, err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != "{backup" {
		t.Fatalf("corrupt backup was overwritten: %q err=%v", backup, err)
	}
}

func TestUnknownApplicationStoreSchemaIsNotOverwritten(t *testing.T) {
	useTempAppConfig(t)
	path := getApplicationStorePath()
	original := []byte(`{"schema_version":99,"settings":{"future":true}}`)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadApplicationStore(); !errors.Is(err, errUnsupportedSchema) {
		t.Fatalf("load error = %v, want unsupported schema", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatalf("unsupported-schema file was changed: %q err=%v", after, err)
	}
}

func TestApplicationStoreRejectsMalformedRecordsAndOversizedFile(t *testing.T) {
	store := newApplicationStore(defaultAppSettings())
	store.Pins = []json.RawMessage{json.RawMessage(`[]`)}
	if err := validateApplicationStore(store); err == nil || !strings.Contains(err.Error(), "pins record") {
		t.Fatalf("invalid record error = %v", err)
	}
	if _, err := decodeApplicationStore(make([]byte, maxAppStoreBytes+1)); err == nil {
		t.Fatal("oversized store was accepted")
	}
}

func TestAppStateControllerEmitsTypedSettingsEvent(t *testing.T) {
	useTempAppConfig(t)
	controller := NewAppStateController()
	events, cancel := controller.Subscribe(1)
	defer cancel()

	settings, err := controller.UpdateSettings(func(current *AppSettings) { current.Theme = "light" })
	if err != nil {
		t.Fatal(err)
	}
	if settings.Theme != "light" {
		t.Fatalf("theme = %q, want light", settings.Theme)
	}
	select {
	case event := <-events:
		if event.Kind != AppEventSettingsChanged || event.Revision != 1 || event.Snapshot.Settings.Theme != "light" {
			t.Fatalf("unexpected event: %#v", event)
		}
	default:
		t.Fatal("settings update did not emit a typed event")
	}
}
