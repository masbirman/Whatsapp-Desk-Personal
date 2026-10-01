//go:build linux || windows

package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type nativeDispatcher interface {
	Dispatch(func())
}

var nativeLockPromptActive atomic.Bool

func lockPolicyJSON() string {
	policy, err := applicationState.PublicLockPolicy()
	if err != nil {
		policy.Enabled = true // corrupt lock data fails closed in the UI
	}
	data, marshalErr := json.Marshal(policy)
	if marshalErr != nil {
		return `{"enabled":true}`
	}
	return string(data)
}

func collectMatchingCredential(owner uintptr, title, message string) ([]byte, bool) {
	first, ok := nativeCredentialPrompt(owner, title, message, true)
	if !ok {
		return nil, false
	}
	defer zeroBytes(first)
	if err := validateLockCredential(first); err != nil {
		nativeInform(owner, title, err.Error())
		return nil, false
	}
	second, ok := nativeCredentialPrompt(owner, title, "Enter the same credential again to confirm.", true)
	if !ok {
		return nil, false
	}
	defer zeroBytes(second)
	if subtle.ConstantTimeCompare(first, second) != 1 {
		nativeInform(owner, title, "The entries did not match. Nothing was saved.")
		return nil, false
	}
	return append([]byte(nil), first...), true
}

func commitPendingLockSetup(owner uintptr, pending PendingLockSetup) bool {
	defer zeroBytes(pending.recoveryCode)
	code := formatRecoveryCode(pending.recoveryCode)
	if !nativeShowRecoveryCode(owner, code) {
		nativeInform(owner, "App lock", "App lock was not enabled because the recovery code was not confirmed as saved.")
		return false
	}
	if err := applicationState.CommitLockSetup(pending); err != nil {
		nativeInform(owner, "App lock", "Could not save the app lock settings.")
		return false
	}
	return true
}

func setupNativeAppLock(owner uintptr) bool {
	credential, ok := collectMatchingCredential(owner, "Set up app lock", "Enter a PIN with at least 6 digits or a passphrase with at least 8 characters.")
	if !ok {
		return false
	}
	pending, err := applicationState.PrepareLockSetup(credential)
	if err != nil {
		nativeInform(owner, "App lock", "Could not prepare app lock settings.")
		return false
	}
	return commitPendingLockSetup(owner, pending)
}

func recoverNativeAppLock(owner uintptr) bool {
	recoveryCode, ok := nativeCredentialPrompt(owner, "Recover app lock", "Enter your one-time recovery code.", true)
	if !ok {
		return false
	}
	defer zeroBytes(recoveryCode)
	credential, ok := collectMatchingCredential(owner, "Recover app lock", "Choose a new PIN or passphrase.")
	if !ok {
		return false
	}
	pending, wait, err := applicationState.PrepareRecovery(recoveryCode, credential)
	if err != nil {
		nativeInform(owner, "App lock", "Recovery could not be completed. Check the code and try again.")
		return false
	}
	if wait > 0 {
		nativeInform(owner, "App lock", fmt.Sprintf("Please wait %s before trying again.", wait.Round(time.Second)))
		zeroBytes(pending.recoveryCode)
		return false
	}
	return commitPendingLockSetup(owner, pending)
}

func verifyNativeAppLockCredential(owner uintptr, lockedPrompt bool) bool {
	for {
		promptOwner := owner
		if lockedPrompt {
			promptOwner = 0
		}
		credential, ok := nativeCredentialPrompt(promptOwner, "Unlock WhatsApp Desk", "Enter your app lock PIN or passphrase.", true)
		if !ok {
			if lockedPrompt {
				// Closing the native prompt never releases the hidden WebView.
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return false
		}
		valid, wait, err := applicationState.VerifyLockCredential(credential)
		zeroBytes(credential)
		if err != nil {
			nativeInform(promptOwner, "App lock", "The stored verifier cannot be used. Recover the local settings to continue.")
			if lockedPrompt {
				return false
			}
			return false
		}
		if valid {
			return true
		}
		if wait > 0 {
			nativeInform(promptOwner, "App lock", fmt.Sprintf("Incorrect credential. Wait %s before the next attempt.", wait.Round(time.Second)))
		} else {
			nativeInform(promptOwner, "App lock", "Incorrect credential.")
		}
		if !lockedPrompt {
			return false
		}
		if nativeAskChoice(promptOwner, "App lock", "Do you want to recover with your one-time recovery code?", "Recover", "Try again") == 1 {
			if recoverNativeAppLock(promptOwner) {
				return true
			}
		}
	}
}

func manageNativeAppLock(owner uintptr) bool {
	if !nativeLockPromptActive.CompareAndSwap(false, true) {
		return false
	}
	defer nativeLockPromptActive.Store(false)
	return manageNativeAppLockCore(owner)
}

func manageNativeAppLockCore(owner uintptr) bool {
	if applicationState.NeedsExplicitStoreReset() {
		nativeSetMainWindowVisible(owner, false)
		for {
			if nativeAskChoice(0, "App settings recovery", "The local settings store is damaged. Reset app settings and lock data? WhatsApp Web cookies and session data will be preserved.", "Reset settings", "Keep locked") == 1 {
				if err := applicationState.ResetCorruptStoreAfterNativeConfirmation(); err != nil {
					nativeInform(0, "App settings recovery", "The damaged settings could not be reset.")
					continue
				}
				nativeInform(0, "App settings recovery", "App settings were reset. The WhatsApp Web session was left untouched.")
				nativeSetMainWindowVisible(owner, true)
				return true
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	config, err := applicationState.LockConfig()
	if err != nil {
		if nativeAskChoice(owner, "App lock recovery", "The stored lock verifier is invalid. Reset the local app settings? WhatsApp Web session data will be preserved.", "Reset", "Cancel") == 1 {
			if resetErr := applicationState.ResetCorruptStoreAfterNativeConfirmation(); resetErr == nil {
				return true
			}
		}
		return false
	}
	if !config.Enabled {
		return setupNativeAppLock(owner)
	}
	choice := nativeAskChoice(owner, "Manage app lock", "Choose recovery-code access, or verify your current app lock credential.", "Use recovery code", "Verify credential")
	if choice == 0 {
		return false
	}
	if choice == 1 {
		return recoverNativeAppLock(owner)
	}
	if !verifyNativeAppLockCredential(owner, false) {
		return false
	}
	if nativeAskChoice(owner, "Manage app lock", "Change the app lock credential and create a new recovery code?", "Change credential", "Keep current") == 1 {
		credential, ok := collectMatchingCredential(owner, "Change app lock credential", "Enter a new PIN or passphrase.")
		if !ok {
			return false
		}
		pending, prepareErr := applicationState.PrepareCredentialChange(credential)
		if prepareErr != nil || !commitPendingLockSetup(owner, pending) {
			return false
		}
	}
	if nativeAskChoice(owner, "Manage app lock", "Disable app lock?", "Disable", "Keep enabled") == 1 {
		if err := applicationState.DisableLockAfterVerification(); err != nil {
			nativeInform(owner, "App lock", "Could not disable app lock.")
			return false
		}
		return true
	}
	configureNativeLockTriggers(owner, config)
	return true
}

func configureNativeLockTriggers(owner uintptr, config LockConfig) {
	message := fmt.Sprintf("Idle lock timeout in seconds (0 disables it). Current value: %d. Use a value from 0 to 86400.", config.IdleTimeoutSecond)
	value, ok := nativeTextPrompt(owner, "Lock triggers", message)
	if !ok {
		return
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(string(value)))
	zeroBytes(value)
	if err != nil || seconds < 0 || seconds > 86400 {
		nativeInform(owner, "Lock triggers", "Enter a whole number from 0 to 86400.")
		return
	}
	lockOnMinimize := nativeAskChoice(owner, "Lock triggers", "Lock when the app window is minimized?", "Yes", "No") == 1
	lockOnStartup := nativeAskChoice(owner, "Lock triggers", "Lock when WhatsApp Desk starts?", "Yes", "No") == 1
	if err := applicationState.UpdateLockPolicy(func(next *LockConfig) {
		next.IdleTimeoutSecond = seconds
		next.LockOnMinimize = lockOnMinimize
		next.LockOnStartup = lockOnStartup
	}); err != nil {
		nativeInform(owner, "Lock triggers", "Could not save lock trigger settings.")
	}
}

// requestNativeAppLock is safe to expose through the bridge: it accepts no
// secret and can only make the app more restrictive.
func requestNativeAppLock(owner uintptr) bool {
	if !nativeLockPromptActive.CompareAndSwap(false, true) {
		return false
	}
	defer nativeLockPromptActive.Store(false)
	if applicationState.NeedsExplicitStoreReset() {
		return manageNativeAppLockCore(owner)
	}
	config, err := applicationState.EffectiveLockConfig()
	if err != nil || !config.Enabled {
		return false
	}
	applicationState.MarkLocked()
	nativeSetMainWindowVisible(owner, false)
	if verifyNativeAppLockCredential(owner, true) {
		applicationState.MarkUnlockedAfterNativeVerification()
		nativeSetMainWindowVisible(owner, true)
		return true
	}
	return false
}

func showStartupNativeAppLock(owner uintptr) bool {
	if !nativeLockPromptActive.CompareAndSwap(false, true) {
		return false
	}
	defer nativeLockPromptActive.Store(false)
	if applicationState.NeedsExplicitStoreReset() {
		return manageNativeAppLockCore(owner)
	}
	config, err := applicationState.EffectiveLockConfig()
	if err != nil {
		return manageNativeAppLockCore(owner)
	}
	if !config.Enabled || !config.LockOnStartup {
		return true
	}
	applicationState.MarkLocked()
	nativeSetMainWindowVisible(owner, false)
	if verifyNativeAppLockCredential(owner, true) {
		applicationState.MarkUnlockedAfterNativeVerification()
		nativeSetMainWindowVisible(owner, true)
		return true
	}
	return false
}

func startNativeAppLockWatcher(w nativeDispatcher, owner uintptr) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		wasMinimized := false
		lastInputLock := time.Time{}
		for range ticker.C {
			config, err := applicationState.EffectiveLockConfig()
			if err != nil || !config.Enabled || applicationState.Snapshot().Locked {
				wasMinimized = nativeWindowMinimized(owner)
				continue
			}
			minimized := nativeWindowMinimized(owner)
			trigger := config.LockOnMinimize && minimized && !wasMinimized
			wasMinimized = minimized
			if config.IdleTimeoutSecond > 0 && nativeIdleFor(owner) >= time.Duration(config.IdleTimeoutSecond)*time.Second {
				if lastInputLock.IsZero() || time.Since(lastInputLock) >= time.Duration(config.IdleTimeoutSecond)*time.Second {
					trigger = true
				}
			}
			if trigger && nativeLockPromptActive.CompareAndSwap(false, true) {
				lastInputLock = time.Now()
				w.Dispatch(func() {
					defer nativeLockPromptActive.Store(false)
					if applicationState.Snapshot().Locked {
						return
					}
					applicationState.MarkLocked()
					nativeSetMainWindowVisible(owner, false)
					if verifyNativeAppLockCredential(owner, true) {
						applicationState.MarkUnlockedAfterNativeVerification()
						nativeSetMainWindowVisible(owner, true)
					}
				})
			}
		}
	}()
}
