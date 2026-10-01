package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	appStoreSchemaVersion = 1
	maxAppStoreBytes      = 8 << 20
	maxStoreRecords       = 50000
	maxStoreRecordBytes   = 256 << 10
)

var (
	appStoreDiskMu         sync.Mutex
	atomicStoreFileReplace = atomicReplaceFile
	errUnsupportedSchema   = errors.New("unsupported application store schema")
)

// ApplicationStore is the versioned native-owned envelope. Feature records
// that are not implemented yet remain opaque JSON objects so M2 can establish
// storage/migration without prematurely implementing later milestones.
type ApplicationStore struct {
	SchemaVersion  int               `json:"schema_version"`
	Settings       AppSettings       `json:"settings"`
	CurrentProfile string            `json:"current_profile_id"`
	Profiles       []json.RawMessage `json:"profiles"`
	Lock           json.RawMessage   `json:"lock"`
	Notifications  json.RawMessage   `json:"notifications"`
	Pins           []json.RawMessage `json:"pins"`
	Bookmarks      []json.RawMessage `json:"bookmarks"`
	Labels         []json.RawMessage `json:"labels"`
	Notes          []json.RawMessage `json:"notes"`
	Appearance     json.RawMessage   `json:"appearance"`
	UpdatedAt      string            `json:"updated_at"`
}

type StoreLoadSource string

const (
	StoreLoadedCurrent    StoreLoadSource = "current"
	StoreCreatedDefault   StoreLoadSource = "default"
	StoreMigratedLegacy   StoreLoadSource = "migrated-legacy-settings"
	StoreRecoveredBackup  StoreLoadSource = "recovered-backup"
	StoreRecoveredDefault StoreLoadSource = "recovered-defaults"
)

// StoreLoadReport contains safe recovery metadata and never includes user data.
type StoreLoadReport struct {
	Source StoreLoadSource
	Detail string
}

func getApplicationStorePath() string {
	return filepath.Join(filepath.Dir(getSettingsFilePath()), "app_store.json")
}

func defaultAppSettings() AppSettings {
	return AppSettings{
		DownloadDir:          getDefaultDownloadDir(),
		NotifyOnDownload:     true,
		NotificationsEnabled: true,
		Theme:                "dark",
		SpellCheckEnabled:    true,
		SpellCheckLang:       "auto",
	}
}

func normalizeAppSettings(settings AppSettings) AppSettings {
	if strings.TrimSpace(settings.DownloadDir) == "" || validateDownloadDir(settings.DownloadDir) != nil {
		settings.DownloadDir = getDefaultDownloadDir()
	}
	if settings.Theme != "dark" && settings.Theme != "light" && settings.Theme != "system" {
		settings.Theme = "dark"
	}
	if strings.TrimSpace(settings.SpellCheckLang) == "" || len(settings.SpellCheckLang) > 64 {
		settings.SpellCheckLang = "auto"
	}
	return settings
}

func newApplicationStore(settings AppSettings) ApplicationStore {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	profiles := defaultProfiles(settings.BlurAvatars)
	lock, _ := json.Marshal(defaultLockConfig())
	notifications, _ := json.Marshal(defaultNotificationPolicy(settings.NotificationsEnabled))
	return ApplicationStore{
		SchemaVersion:  appStoreSchemaVersion,
		Settings:       normalizeAppSettings(settings),
		CurrentProfile: "normal",
		Profiles:       profiles,
		Lock:           lock,
		Notifications:  notifications,
		Pins:           []json.RawMessage{},
		Bookmarks:      []json.RawMessage{},
		Labels:         []json.RawMessage{},
		Notes:          []json.RawMessage{},
		Appearance:     json.RawMessage(`{}`),
		UpdatedAt:      now,
	}
}

func defaultStoreReport() StoreLoadReport {
	return StoreLoadReport{Source: StoreCreatedDefault}
}

func loadApplicationStore() (ApplicationStore, StoreLoadReport, error) {
	appStoreDiskMu.Lock()
	defer appStoreDiskMu.Unlock()
	return loadApplicationStoreLocked()
}

func loadApplicationStoreLocked() (ApplicationStore, StoreLoadReport, error) {
	path := getApplicationStorePath()
	currentBytes, currentErr := readStoreFile(path)
	if currentErr == nil {
		store, err := decodeApplicationStore(currentBytes)
		if err == nil {
			return store, StoreLoadReport{Source: StoreLoadedCurrent}, nil
		}
		if errors.Is(err, errUnsupportedSchema) {
			return ApplicationStore{}, StoreLoadReport{Source: StoreLoadedCurrent, Detail: "unsupported schema; file left unchanged"}, err
		}
		currentErr = err
	} else if !errors.Is(currentErr, os.ErrNotExist) {
		return ApplicationStore{}, StoreLoadReport{}, currentErr
	}

	backupPath := path + ".bak"
	backupBytes, backupErr := readStoreFile(backupPath)
	if backupErr == nil {
		backupStore, err := decodeApplicationStore(backupBytes)
		if err == nil {
			if currentErr == nil {
				currentErr = errors.New("primary store is unavailable")
			}
			if _, statErr := os.Lstat(path); statErr == nil {
				if preserveErr := preserveCorruptStore(path); preserveErr != nil {
					return backupStore, StoreLoadReport{Source: StoreRecoveredBackup, Detail: "backup valid; primary preservation failed"}, preserveErr
				}
			}
			if err := writeAtomicFile(path, backupBytes, 0o600); err != nil {
				return backupStore, StoreLoadReport{Source: StoreRecoveredBackup, Detail: "backup valid; primary restore failed"}, err
			}
			return backupStore, StoreLoadReport{Source: StoreRecoveredBackup, Detail: "primary store restored from backup"}, nil
		}
		if errors.Is(err, errUnsupportedSchema) {
			return ApplicationStore{}, StoreLoadReport{Source: StoreRecoveredBackup, Detail: "unsupported backup schema; files left unchanged"}, err
		}
		backupErr = err
	} else if !errors.Is(backupErr, os.ErrNotExist) {
		backupErr = fmt.Errorf("read backup: %w", backupErr)
	}

	if errors.Is(currentErr, os.ErrNotExist) {
		legacy, legacyErr := readLegacySettings()
		if legacyErr != nil {
			return ApplicationStore{}, StoreLoadReport{}, legacyErr
		}
		store := newApplicationStore(legacy.Settings)
		source := StoreCreatedDefault
		if legacy.Exists {
			source = StoreMigratedLegacy
		}
		if legacy.Detail != "" {
			source = StoreRecoveredDefault
		}
		if err := saveApplicationStoreLocked(store, true); err != nil {
			return store, StoreLoadReport{Source: source, Detail: err.Error()}, err
		}
		verified, err := readAndValidateStore(path)
		if err != nil {
			return store, StoreLoadReport{Source: source, Detail: "write verification failed"}, err
		}
		return verified, StoreLoadReport{Source: source, Detail: legacy.Detail}, nil
	}

	if _, statErr := os.Lstat(path); statErr == nil {
		if err := preserveCorruptStore(path); err != nil {
			return ApplicationStore{}, StoreLoadReport{Source: StoreRecoveredDefault, Detail: "primary preservation failed"}, err
		}
	}
	store := newApplicationStore(defaultAppSettings())
	if err := saveApplicationStoreLocked(store, true); err != nil {
		return store, StoreLoadReport{Source: StoreRecoveredDefault, Detail: "using safe defaults; store write failed"}, err
	}
	detail := "invalid primary and backup preserved; safe defaults written"
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		detail += "; backup was also invalid"
	}
	return store, StoreLoadReport{Source: StoreRecoveredDefault, Detail: detail}, nil
}

type legacySettingsRead struct {
	Settings AppSettings
	Exists   bool
	Detail   string
}

func readLegacySettings() (legacySettingsRead, error) {
	path := getSettingsFilePath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return legacySettingsRead{Settings: defaultAppSettings()}, nil
	}
	if err != nil {
		return legacySettingsRead{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return legacySettingsRead{Settings: defaultAppSettings(), Exists: true, Detail: "legacy settings was not a regular file; defaults used"}, nil
	}
	if info.Size() > 1<<20 {
		return legacySettingsRead{Settings: defaultAppSettings(), Exists: true, Detail: "legacy settings exceeded size limit; defaults used"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return legacySettingsRead{}, err
	}
	settings := defaultAppSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return legacySettingsRead{Settings: defaultAppSettings(), Exists: true, Detail: "legacy settings was malformed; defaults used"}, nil
	}
	return legacySettingsRead{Settings: normalizeAppSettings(settings), Exists: true}, nil
}

func readStoreFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("store path is not a regular file: %s", filepath.Base(path))
	}
	if info.Size() > maxAppStoreBytes {
		return nil, fmt.Errorf("store exceeds %d byte limit", maxAppStoreBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > maxAppStoreBytes {
		return nil, fmt.Errorf("store exceeds %d byte limit", maxAppStoreBytes)
	}
	return data, nil
}

func readAndValidateStore(path string) (ApplicationStore, error) {
	data, err := readStoreFile(path)
	if err != nil {
		return ApplicationStore{}, err
	}
	return decodeApplicationStore(data)
}

func decodeApplicationStore(data []byte) (ApplicationStore, error) {
	if len(data) == 0 || len(data) > maxAppStoreBytes {
		return ApplicationStore{}, errors.New("invalid application store size")
	}
	var store ApplicationStore
	if err := json.Unmarshal(data, &store); err != nil {
		return ApplicationStore{}, fmt.Errorf("decode application store: %w", err)
	}
	if store.SchemaVersion != appStoreSchemaVersion {
		return ApplicationStore{}, fmt.Errorf("schema %d: %w", store.SchemaVersion, errUnsupportedSchema)
	}
	profiles, err := normalizeProfileRecords(store.Profiles, store.Settings.BlurAvatars)
	if err != nil {
		return ApplicationStore{}, err
	}
	store.Profiles = profiles
	if len(store.Lock) == 0 {
		lock, _ := json.Marshal(defaultLockConfig())
		store.Lock = lock
	}
	if err := validateApplicationStore(store); err != nil {
		return ApplicationStore{}, err
	}
	store.Settings = normalizeAppSettings(store.Settings)
	return store, nil
}

func validateApplicationStore(store ApplicationStore) error {
	if store.SchemaVersion != appStoreSchemaVersion {
		return errUnsupportedSchema
	}
	if strings.TrimSpace(store.CurrentProfile) == "" || len(store.CurrentProfile) > 128 {
		return errors.New("invalid current profile identifier")
	}
	if len(store.Settings.DownloadDir) > 4096 || len(store.Settings.SpellCheckLang) > 64 {
		return errors.New("application settings exceed field limits")
	}
	if store.Settings.Theme != "" && store.Settings.Theme != "dark" && store.Settings.Theme != "light" && store.Settings.Theme != "system" {
		return errors.New("invalid theme value")
	}
	profiles, err := decodeProfiles(store.Profiles)
	if err != nil {
		return fmt.Errorf("profiles: %w", err)
	}
	currentFound := false
	for _, profile := range profiles {
		if profile.ID == store.CurrentProfile {
			currentFound = true
			break
		}
	}
	if !currentFound {
		return errors.New("current profile does not exist")
	}
	if _, err := decodeLockConfig(store.Lock); err != nil {
		return fmt.Errorf("lock configuration: %w", err)
	}
	if store.UpdatedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, store.UpdatedAt); err != nil {
			return errors.New("invalid updated_at timestamp")
		}
	}
	recordGroups := []struct {
		name    string
		records []json.RawMessage
	}{
		{name: "profiles", records: store.Profiles},
		{name: "pins", records: store.Pins},
		{name: "bookmarks", records: store.Bookmarks},
		{name: "labels", records: store.Labels},
		{name: "notes", records: store.Notes},
	}
	for _, group := range recordGroups {
		name, records := group.name, group.records
		if len(records) > maxStoreRecords {
			return fmt.Errorf("%s exceeds record limit", name)
		}
		for i, record := range records {
			if len(record) == 0 || len(record) > maxStoreRecordBytes || !isJSONObject(record) {
				return fmt.Errorf("%s record %d is invalid", name, i)
			}
		}
	}
	objectFields := []struct {
		name  string
		value json.RawMessage
	}{
		{name: "lock", value: store.Lock},
		{name: "notifications", value: store.Notifications},
		{name: "appearance", value: store.Appearance},
	}
	for _, field := range objectFields {
		name, value := field.name, field.value
		if len(value) > maxStoreRecordBytes || (len(value) > 0 && !isJSONObject(value)) {
			return fmt.Errorf("%s settings must be a JSON object", name)
		}
	}
	return nil
}

func isJSONObject(data []byte) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(data, &object) == nil && object != nil
}

func saveApplicationStore(store ApplicationStore) error {
	appStoreDiskMu.Lock()
	defer appStoreDiskMu.Unlock()
	return saveApplicationStoreLocked(store, false)
}

func saveApplicationStoreLocked(store ApplicationStore, skipBackup bool) error {
	if store.SchemaVersion == 0 {
		store.SchemaVersion = appStoreSchemaVersion
	}
	store.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.Settings = normalizeAppSettings(store.Settings)
	if err := validateApplicationStore(store); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxAppStoreBytes {
		return fmt.Errorf("application store exceeds %d byte limit", maxAppStoreBytes)
	}

	path := getApplicationStorePath()
	if !skipBackup {
		if current, readErr := readStoreFile(path); readErr == nil {
			if _, decodeErr := decodeApplicationStore(current); decodeErr == nil {
				if err := writeAtomicFile(path+".bak", current, 0o600); err != nil {
					return fmt.Errorf("write previous-store backup: %w", err)
				}
			} else if errors.Is(decodeErr, errUnsupportedSchema) {
				return decodeErr
			} else if err := preserveCorruptStore(path); err != nil {
				return fmt.Errorf("preserve invalid previous store: %w", err)
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
	}
	return writeAtomicFile(path, data, 0o600)
}

func preserveCorruptStore(path string) error {
	data, err := readStoreFile(path)
	if err != nil {
		return err
	}
	corruptPath := path + ".corrupt-" + time.Now().UTC().Format("20060102T150405.000000000")
	f, err := os.OpenFile(corruptPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".whatsappdesk-store-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := atomicStoreFileReplace(tmpPath, path); err != nil {
		return err
	}
	return syncStoreDirectory(dir)
}
