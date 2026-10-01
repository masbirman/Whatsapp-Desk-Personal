package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	lockKDFName       = "argon2id"
	lockVerifierBytes = 32
	lockSaltBytes     = 16
	lockMemoryMinKiB  = 19 * 1024
	lockMemoryMaxKiB  = 256 * 1024
	lockIterationsMin = 2
	lockIterationsMax = 10
	lockThreadsMin    = 1
	lockThreadsMax    = 4
	lockCredentialMax = 128
)

type LockConfig struct {
	Enabled           bool   `json:"enabled"`
	KDF               string `json:"kdf"`
	KDFVersion        int    `json:"kdf_version"`
	Salt              string `json:"salt,omitempty"`
	Verifier          string `json:"verifier,omitempty"`
	RecoverySalt      string `json:"recovery_salt,omitempty"`
	RecoveryVerifier  string `json:"recovery_verifier,omitempty"`
	MemoryKiB         uint32 `json:"memory_kib"`
	Iterations        uint32 `json:"iterations"`
	Parallelism       uint8  `json:"parallelism"`
	IdleTimeoutSecond int    `json:"idle_timeout_seconds"`
	LockOnMinimize    bool   `json:"lock_on_minimize"`
	LockOnTray        bool   `json:"lock_on_tray"`
	LockOnStartup     bool   `json:"lock_on_startup"`
}

type PublicLockPolicy struct {
	Enabled           bool `json:"enabled"`
	IdleTimeoutSecond int  `json:"idle_timeout_seconds"`
	LockOnMinimize    bool `json:"lock_on_minimize"`
	LockOnTray        bool `json:"lock_on_tray"`
	LockOnStartup     bool `json:"lock_on_startup"`
}

type PendingLockSetup struct {
	config          LockConfig
	recoveryCode    []byte
	replaceExisting bool
}

func defaultLockConfig() LockConfig {
	return LockConfig{
		KDF: lockKDFName, KDFVersion: argon2.Version,
		MemoryKiB: lockMemoryMinKiB, Iterations: lockIterationsMin,
		Parallelism: lockThreadsMin,
	}
}

func validateLockConfig(config LockConfig) error {
	if !config.Enabled {
		return nil
	}
	if config.KDF != lockKDFName || config.KDFVersion != argon2.Version {
		return errors.New("unsupported lock verifier format")
	}
	if config.MemoryKiB < lockMemoryMinKiB || config.MemoryKiB > lockMemoryMaxKiB ||
		config.Iterations < lockIterationsMin || config.Iterations > lockIterationsMax ||
		config.Parallelism < lockThreadsMin || config.Parallelism > lockThreadsMax {
		return errors.New("lock KDF parameters outside supported bounds")
	}
	if config.IdleTimeoutSecond < 0 || config.IdleTimeoutSecond > 24*60*60 {
		return errors.New("lock idle timeout outside supported bounds")
	}
	salt, err := base64.StdEncoding.DecodeString(config.Salt)
	if err != nil || len(salt) != lockSaltBytes {
		return errors.New("invalid credential salt")
	}
	verifier, err := base64.StdEncoding.DecodeString(config.Verifier)
	if err != nil || len(verifier) != lockVerifierBytes {
		return errors.New("invalid credential verifier")
	}
	recoverySalt, err := base64.StdEncoding.DecodeString(config.RecoverySalt)
	if err != nil || len(recoverySalt) != lockSaltBytes {
		return errors.New("invalid recovery salt")
	}
	recoveryVerifier, err := base64.StdEncoding.DecodeString(config.RecoveryVerifier)
	if err != nil || len(recoveryVerifier) != lockVerifierBytes {
		return errors.New("invalid recovery verifier")
	}
	return nil
}

func decodeLockConfig(raw json.RawMessage) (LockConfig, error) {
	config := defaultLockConfig()
	if len(raw) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, err
	}
	if err := validateLockConfig(config); err != nil {
		return config, err
	}
	return config, nil
}

func validateLockCredential(credential []byte) error {
	if len(credential) > lockCredentialMax || !utf8.Valid(credential) {
		return errors.New("credential must be valid UTF-8 and at most 128 bytes")
	}
	if len(credential) == 0 {
		return errors.New("credential is required")
	}
	allDigits := true
	for i := 0; i < len(credential); {
		r, size := utf8.DecodeRune(credential[i:])
		i += size
		if r < '0' || r > '9' {
			allDigits = false
		}
		if unicode.IsControl(r) {
			return errors.New("credential contains an unsupported control character")
		}
	}
	if allDigits {
		if len(credential) < 6 {
			return errors.New("numeric PIN must contain at least 6 digits")
		}
		return nil
	}
	if utf8.RuneCount(credential) < 8 {
		return errors.New("passphrase must contain at least 8 characters")
	}
	return nil
}

func deriveLockVerifier(credential, salt []byte, memoryKiB, iterations uint32, parallelism uint8) []byte {
	return argon2.IDKey(credential, salt, iterations, memoryKiB, parallelism, lockVerifierBytes)
}

func newRandomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		zeroBytes(value)
		return nil, err
	}
	return value, nil
}

func newRecoveryCode() ([]byte, error) {
	raw, err := newRandomBytes(20)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(raw)
	code := []byte(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
	return code, nil
}

func normalizeRecoveryCode(code []byte) []byte {
	clean := make([]byte, 0, len(code))
	for _, ch := range code {
		if ch == ' ' || ch == '-' || ch == '\t' || ch == '\n' || ch == '\r' {
			continue
		}
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		clean = append(clean, ch)
	}
	return clean
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

type lockAttemptTracker struct {
	mu       sync.Mutex
	failures uint32
	until    time.Time
}

func (a *lockAttemptTracker) remaining(now time.Time) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.until.After(now) {
		return 0
	}
	return a.until.Sub(now)
}

func (a *lockAttemptTracker) failed(now time.Time) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures++
	shift := a.failures - 1
	if shift > 7 {
		shift = 7
	}
	delay := 250 * time.Millisecond * time.Duration(1<<shift)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	a.until = now.Add(delay)
	return delay
}

func (a *lockAttemptTracker) succeeded() {
	a.mu.Lock()
	a.failures = 0
	a.until = time.Time{}
	a.mu.Unlock()
}

var processLockAttempts lockAttemptTracker

func (c *AppStateController) PublicLockPolicy() (PublicLockPolicy, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return PublicLockPolicy{Enabled: true}, err
	}
	return PublicLockPolicy{
		Enabled: config.Enabled, IdleTimeoutSecond: config.IdleTimeoutSecond,
		LockOnMinimize: config.LockOnMinimize, LockOnTray: config.LockOnTray,
		LockOnStartup: config.LockOnStartup,
	}, nil
}

func (c *AppStateController) LockConfig() (LockConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	return decodeLockConfig(c.store.Lock)
}

func (c *AppStateController) EffectiveLockConfig() (LockConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return LockConfig{}, err
	}
	profiles, err := decodeProfiles(c.store.Profiles)
	if err != nil {
		return LockConfig{}, err
	}
	for i := range profiles {
		if profiles[i].ID == c.store.CurrentProfile {
			return effectiveLockPolicy(config, &profiles[i]), nil
		}
	}
	return config, nil
}

func (c *AppStateController) saveLockConfigLocked(config LockConfig) error {
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	updated := c.store
	updated.Lock = encoded
	if err := saveApplicationStore(updated); err != nil {
		return err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventLockChanged)
	return nil
}

func buildLockConfig(credential []byte, prior LockConfig) (LockConfig, []byte, error) {
	if err := validateLockCredential(credential); err != nil {
		return LockConfig{}, nil, err
	}
	config := prior
	if config.KDF == "" {
		config = defaultLockConfig()
	}
	config.Enabled = true
	salt, err := newRandomBytes(lockSaltBytes)
	if err != nil {
		return LockConfig{}, nil, err
	}
	defer zeroBytes(salt)
	recoverySalt, err := newRandomBytes(lockSaltBytes)
	if err != nil {
		return LockConfig{}, nil, err
	}
	defer zeroBytes(recoverySalt)
	code, err := newRecoveryCode()
	if err != nil {
		return LockConfig{}, nil, err
	}
	defer zeroBytes(code)
	verifier := deriveLockVerifier(credential, salt, config.MemoryKiB, config.Iterations, config.Parallelism)
	recoveryVerifier := deriveLockVerifier(code, recoverySalt, config.MemoryKiB, config.Iterations, config.Parallelism)
	defer zeroBytes(verifier)
	defer zeroBytes(recoveryVerifier)
	config.Salt = base64.StdEncoding.EncodeToString(salt)
	config.Verifier = base64.StdEncoding.EncodeToString(verifier)
	config.RecoverySalt = base64.StdEncoding.EncodeToString(recoverySalt)
	config.RecoveryVerifier = base64.StdEncoding.EncodeToString(recoveryVerifier)
	if err := validateLockConfig(config); err != nil {
		return LockConfig{}, nil, err
	}
	return config, append([]byte(nil), code...), nil
}

func (c *AppStateController) PrepareLockSetup(credential []byte) (PendingLockSetup, error) {
	defer zeroBytes(credential)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	current, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return PendingLockSetup{}, err
	}
	if current.Enabled {
		return PendingLockSetup{}, errors.New("app lock is already enabled")
	}
	config, code, err := buildLockConfig(credential, current)
	if err != nil {
		return PendingLockSetup{}, err
	}
	return PendingLockSetup{config: config, recoveryCode: code}, nil
}

func (c *AppStateController) PrepareCredentialChange(credential []byte) (PendingLockSetup, error) {
	defer zeroBytes(credential)
	if err := validateLockCredential(credential); err != nil {
		return PendingLockSetup{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	current, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return PendingLockSetup{}, err
	}
	if !current.Enabled {
		return PendingLockSetup{}, errors.New("app lock is disabled")
	}
	config, code, err := buildLockConfig(credential, current)
	if err != nil {
		return PendingLockSetup{}, err
	}
	return PendingLockSetup{config: config, recoveryCode: code, replaceExisting: true}, nil
}

func (c *AppStateController) PrepareRecovery(recoveryCode, newCredential []byte) (PendingLockSetup, time.Duration, error) {
	defer zeroBytes(recoveryCode)
	defer zeroBytes(newCredential)
	if err := validateLockCredential(newCredential); err != nil {
		return PendingLockSetup{}, 0, err
	}
	c.mu.Lock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	c.mu.Unlock()
	if err != nil {
		return PendingLockSetup{}, 0, err
	}
	if !config.Enabled {
		return PendingLockSetup{}, 0, errors.New("app lock is disabled")
	}
	if remaining := processLockAttempts.remaining(time.Now()); remaining > 0 {
		return PendingLockSetup{}, remaining, nil
	}
	if !verifyLockSecret(config, recoveryCode, true) {
		return PendingLockSetup{}, processLockAttempts.failed(time.Now()), nil
	}
	processLockAttempts.succeeded()
	config.Salt, config.Verifier = "", ""
	config.RecoverySalt, config.RecoveryVerifier = "", ""
	config, code, err := buildLockConfig(newCredential, config)
	if err != nil {
		return PendingLockSetup{}, 0, err
	}
	return PendingLockSetup{config: config, recoveryCode: code, replaceExisting: true}, 0, nil
}

func (c *AppStateController) CommitLockSetup(pending PendingLockSetup) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if c.loadErr != nil {
		return c.loadErr
	}
	current, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return err
	}
	if current.Enabled && !pending.replaceExisting {
		return errors.New("app lock was changed before setup could be saved")
	}
	if !current.Enabled && pending.replaceExisting {
		return errors.New("app lock was disabled before replacement could be saved")
	}
	if err := validateLockConfig(pending.config); err != nil || !pending.config.Enabled {
		if err != nil {
			return err
		}
		return errors.New("pending app lock setup is invalid")
	}
	if err := c.saveLockConfigLocked(pending.config); err != nil {
		return err
	}
	processLockAttempts.succeeded()
	return nil
}

func (c *AppStateController) EnableLock(credential []byte) ([]byte, error) {
	pending, err := c.PrepareLockSetup(credential)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(pending.recoveryCode)
	if err := c.CommitLockSetup(pending); err != nil {
		return nil, err
	}
	return append([]byte(nil), pending.recoveryCode...), nil
}

func verifyLockSecret(config LockConfig, secret []byte, recovery bool) bool {
	saltText, verifierText := config.Salt, config.Verifier
	if recovery {
		saltText, verifierText = config.RecoverySalt, config.RecoveryVerifier
		normalized := normalizeRecoveryCode(secret)
		zeroBytes(secret)
		secret = normalized
		defer zeroBytes(normalized)
	}
	salt, err := base64.StdEncoding.DecodeString(saltText)
	if err != nil {
		return false
	}
	defer zeroBytes(salt)
	expected, err := base64.StdEncoding.DecodeString(verifierText)
	if err != nil {
		return false
	}
	defer zeroBytes(expected)
	actual := deriveLockVerifier(secret, salt, config.MemoryKiB, config.Iterations, config.Parallelism)
	defer zeroBytes(actual)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (c *AppStateController) VerifyLockCredential(credential []byte) (bool, time.Duration, error) {
	defer zeroBytes(credential)
	c.mu.Lock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	c.mu.Unlock()
	if err != nil {
		return false, 0, err
	}
	if !config.Enabled {
		return false, 0, errors.New("app lock is disabled")
	}
	if remaining := processLockAttempts.remaining(time.Now()); remaining > 0 {
		return false, remaining, nil
	}
	if verifyLockSecret(config, credential, false) {
		processLockAttempts.succeeded()
		return true, 0, nil
	}
	return false, processLockAttempts.failed(time.Now()), nil
}

func (c *AppStateController) RecoverLockCredential(recoveryCode, newCredential []byte) ([]byte, time.Duration, error) {
	pending, wait, err := c.PrepareRecovery(recoveryCode, newCredential)
	if err != nil || wait > 0 {
		return nil, wait, err
	}
	defer zeroBytes(pending.recoveryCode)
	if err := c.CommitLockSetup(pending); err != nil {
		return nil, 0, err
	}
	return append([]byte(nil), pending.recoveryCode...), 0, nil
}

func (c *AppStateController) UpdateLockPolicy(update func(*LockConfig)) error {
	if update == nil {
		return errors.New("lock policy update is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return errors.New("app lock is disabled")
	}
	update(&config)
	if err := validateLockConfig(config); err != nil {
		return err
	}
	return c.saveLockConfigLocked(config)
}

func (c *AppStateController) DisableLockAfterVerification() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	config, err := decodeLockConfig(c.store.Lock)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}
	config.Enabled = false
	config.Salt, config.Verifier = "", ""
	config.RecoverySalt, config.RecoveryVerifier = "", ""
	config.LockOnStartup, config.LockOnMinimize, config.LockOnTray = false, false, false
	config.IdleTimeoutSecond = 0
	processLockAttempts.succeeded()
	return c.saveLockConfigLocked(config)
}

func formatRecoveryCode(code []byte) string {
	compact := string(code)
	parts := make([]string, 0, (len(compact)+3)/4)
	for len(compact) > 4 {
		parts = append(parts, compact[:4])
		compact = compact[4:]
	}
	if compact != "" {
		parts = append(parts, compact)
	}
	return strings.Join(parts, "-")
}

func lockPolicySummary(policy PublicLockPolicy) string {
	parts := []string{}
	if policy.IdleTimeoutSecond > 0 {
		parts = append(parts, fmt.Sprintf("idle %d seconds", policy.IdleTimeoutSecond))
	}
	if policy.LockOnMinimize {
		parts = append(parts, "minimize")
	}
	if policy.LockOnTray {
		parts = append(parts, "tray")
	}
	if policy.LockOnStartup {
		parts = append(parts, "startup")
	}
	if len(parts) == 0 {
		return "manual lock only"
	}
	return strings.Join(parts, ", ")
}
