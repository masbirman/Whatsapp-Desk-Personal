package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockCredentialPolicyAndKDFBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   []byte
		wantErr bool
	}{
		{name: "six digit pin", value: []byte("123456")},
		{name: "eight character passphrase", value: []byte("correct horse")},
		{name: "short pin", value: []byte("12345"), wantErr: true},
		{name: "short phrase", value: []byte("short"), wantErr: true},
		{name: "control byte", value: []byte("abcdefgh\n"), wantErr: true},
		{name: "too long", value: bytes.Repeat([]byte("x"), lockCredentialMax+1), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLockCredential(tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateLockCredential() error=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
	config := defaultLockConfig()
	config.Enabled = true
	config.Salt = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, lockSaltBytes))
	config.Verifier = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, lockVerifierBytes))
	config.RecoverySalt = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, lockSaltBytes))
	config.RecoveryVerifier = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, lockVerifierBytes))
	if err := validateLockConfig(config); err != nil {
		t.Fatalf("minimum supported Argon2id config rejected: %v", err)
	}
	config.MemoryKiB = lockMemoryMaxKiB + 1
	if err := validateLockConfig(config); err == nil {
		t.Fatal("out-of-range KDF memory was accepted")
	}
}

func TestAppLockStoresOnlySeparateSaltedVerifiersAndRecoveryIsOneTime(t *testing.T) {
	useTempAppConfig(t)
	processLockAttempts.succeeded()
	credential := []byte("123456")
	code, err := applicationState.EnableLock(credential)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(credential, make([]byte, len(credential))) {
		t.Fatal("credential input buffer was not cleared")
	}
	config, err := applicationState.LockConfig()
	if err != nil || !config.Enabled {
		t.Fatalf("lock config = %#v err=%v", config, err)
	}
	credentialSalt, _ := base64.StdEncoding.DecodeString(config.Salt)
	recoverySalt, _ := base64.StdEncoding.DecodeString(config.RecoverySalt)
	if bytes.Equal(credentialSalt, recoverySalt) {
		t.Fatal("credential and recovery verifier salts must be independent")
	}
	stored, err := os.ReadFile(getApplicationStorePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("123456")) || bytes.Contains(stored, code) {
		t.Fatal("plaintext credential or recovery code was persisted")
	}
	valid, _, err := applicationState.VerifyLockCredential([]byte("123456"))
	if err != nil || !valid {
		t.Fatalf("correct credential rejected: valid=%v err=%v", valid, err)
	}
	valid, delay, err := applicationState.VerifyLockCredential([]byte("654321"))
	if err != nil || valid || delay <= 0 {
		t.Fatalf("incorrect credential result: valid=%v delay=%v err=%v", valid, delay, err)
	}
	processLockAttempts.succeeded() // isolate the recovery test from the enforced wait
	newCode, wait, err := applicationState.RecoverLockCredential(append([]byte(nil), code...), []byte("a longer passphrase"))
	if err != nil || wait != 0 {
		t.Fatalf("recovery failed: wait=%v err=%v", wait, err)
	}
	processLockAttempts.succeeded()
	valid, _, err = applicationState.VerifyLockCredential([]byte("123456"))
	if err != nil || valid {
		t.Fatalf("old credential remained valid after recovery: valid=%v err=%v", valid, err)
	}
	processLockAttempts.succeeded()
	valid, _, err = applicationState.VerifyLockCredential([]byte("a longer passphrase"))
	if err != nil || !valid {
		t.Fatalf("new credential rejected after recovery: valid=%v err=%v", valid, err)
	}
	processLockAttempts.succeeded()
	if _, wait, err := applicationState.RecoverLockCredential(append([]byte(nil), code...), []byte("another passphrase")); err != nil || wait == 0 {
		t.Fatalf("reused recovery code should fail with backoff: wait=%v err=%v", wait, err)
	}
	zeroBytes(newCode)
}

func TestLockAttemptTrackerAppliesCappedInProcessBackoff(t *testing.T) {
	var tracker lockAttemptTracker
	now := time.Unix(100, 0)
	if got := tracker.failed(now); got != 250*time.Millisecond {
		t.Fatalf("first backoff = %v", got)
	}
	if got := tracker.remaining(now.Add(100 * time.Millisecond)); got != 150*time.Millisecond {
		t.Fatalf("remaining backoff = %v", got)
	}
	for i := 0; i < 20; i++ {
		tracker.failed(now)
	}
	if got := tracker.remaining(now); got != 30*time.Second {
		t.Fatalf("backoff cap = %v, want 30s", got)
	}
	tracker.succeeded()
	if got := tracker.remaining(now); got != 0 {
		t.Fatalf("successful verification did not clear in-process backoff: %v", got)
	}
}

func TestPrivacyProfilesPersistSwitchCopyAndResetIndependently(t *testing.T) {
	useTempAppConfig(t)
	profiles, current, err := applicationState.Profiles()
	if err != nil || current != "normal" || len(profiles) != 5 {
		t.Fatalf("default profiles: current=%q count=%d err=%v", current, len(profiles), err)
	}
	office, err := applicationState.SelectProfile("office")
	if err != nil || !office.Privacy.ChatNames || !office.Privacy.MediaViewer {
		t.Fatalf("office profile defaults not applied: %#v err=%v", office, err)
	}
	custom, err := applicationState.CopyCurrentProfileToCustom()
	if err != nil || custom.ID != "custom" || custom.Privacy != office.Privacy {
		t.Fatalf("copy to CUSTOM failed: %#v err=%v", custom, err)
	}
	custom.Privacy.Images = false
	custom.Privacy.RevealMode = RevealClick
	if _, err := applicationState.UpdateCustomPrivacy(custom.Privacy); err != nil {
		t.Fatal(err)
	}
	profiles, current, err = applicationState.Profiles()
	if err != nil || current != "custom" {
		t.Fatalf("custom selection was not persisted: current=%q err=%v", current, err)
	}
	var persisted PrivacyProfile
	for _, profile := range profiles {
		if profile.ID == "custom" {
			persisted = profile
		}
	}
	if !persisted.Privacy.ChatNames || persisted.Privacy.Images || persisted.Privacy.RevealMode != RevealClick {
		t.Fatalf("granular custom policy did not persist: %#v", persisted.Privacy)
	}
	if err := applicationState.ResetBuiltInProfiles(); err != nil {
		t.Fatal(err)
	}
	profiles, _, err = applicationState.Profiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		if profile.ID == "custom" && (!profile.Privacy.ChatNames || profile.Privacy.Images || profile.Privacy.RevealMode != RevealClick) {
			t.Fatalf("resetting built-ins changed CUSTOM: %#v", profile.Privacy)
		}
		if profile.ID == "office" && !profile.Privacy.Stickers {
			t.Fatalf("built-in OFFICE profile was not restored: %#v", profile.Privacy)
		}
	}
}

func TestPrivacySurfaceBridgeRejectsUnknownFieldsAndBuiltInEdits(t *testing.T) {
	useTempAppConfig(t)
	policyJSON, _ := json.Marshal(PrivacyPolicy{ChatNames: true, RevealMode: RevealHover, RevealModifier: "alt"})
	if result := updateCustomPrivacyPolicyJSON(string(policyJSON)); result != "{}" {
		t.Fatalf("built-in profile edit should be rejected, result=%s", result)
	}
	if _, err := applicationState.SelectProfile("custom"); err != nil {
		t.Fatal(err)
	}
	if result := updateCustomPrivacyPolicyJSON(`{"chat_names":true,"unknown_field":true}`); result != "{}" {
		t.Fatalf("unknown privacy policy field was accepted: %s", result)
	}
	if result := updateCustomPrivacyPolicyJSON(string(policyJSON)); result == "{}" {
		t.Fatal("valid CUSTOM privacy policy was rejected")
	}
}

func TestProfileLockOverridesCanOnlyTightenGlobalLock(t *testing.T) {
	global := defaultLockConfig()
	global.Enabled = true
	global.IdleTimeoutSecond = 300
	global.LockOnMinimize = true
	profile := PrivacyProfile{LockOverrides: &LockPolicyOverrides{IdleTimeoutSeconds: 600, LockOnMinimize: false, LockOnStartup: true}}
	effective := effectiveLockPolicy(global, &profile)
	if effective.IdleTimeoutSecond != 300 || !effective.LockOnMinimize || !effective.LockOnStartup {
		t.Fatalf("profile weakened global lock policy: %#v", effective)
	}
	profile.LockOverrides.IdleTimeoutSeconds = 60
	effective = effectiveLockPolicy(global, &profile)
	if effective.IdleTimeoutSecond != 60 {
		t.Fatalf("shorter profile timeout was not applied: %d", effective.IdleTimeoutSecond)
	}
}

func TestCorruptStoreFailsClosedUntilNativeResetAndDoesNotTouchWebViewPath(t *testing.T) {
	root := useTempAppConfig(t)
	storePath := getApplicationStorePath()
	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storePath, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	webviewSentinel := filepath.Join(root, "webview-profile", "session.marker")
	if err := os.MkdirAll(filepath.Dir(webviewSentinel), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webviewSentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := applicationState.Snapshot()
	if !snapshot.LockEnabled || !snapshot.Locked || !applicationState.NeedsExplicitStoreReset() {
		t.Fatalf("damaged store did not fail closed: %#v", snapshot)
	}
	if err := applicationState.ResetCorruptStoreAfterNativeConfirmation(); err != nil {
		t.Fatal(err)
	}
	if applicationState.Snapshot().Locked {
		t.Fatal("explicit native reset did not clear fail-closed lock state")
	}
	content, err := os.ReadFile(webviewSentinel)
	if err != nil || string(content) != "keep" {
		t.Fatalf("explicit settings reset changed the WebView profile: %q err=%v", content, err)
	}
}
