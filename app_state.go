package main

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

type AppEventKind string

const (
	AppEventSettingsChanged      AppEventKind = "settings-changed"
	AppEventProfileChanged       AppEventKind = "profile-changed"
	AppEventLockChanged          AppEventKind = "lock-changed"
	AppEventNotificationsChanged AppEventKind = "notifications-changed"
	AppEventPinsChanged          AppEventKind = "pins-changed"
	AppEventBookmarksChanged     AppEventKind = "bookmarks-changed"
	AppEventLabelsChanged        AppEventKind = "labels-changed"
	AppEventNotesChanged         AppEventKind = "notes-changed"
)

type AppStateSnapshot struct {
	Revision        uint64           `json:"revision"`
	CurrentProfile  string           `json:"current_profile_id"`
	CurrentPrivacy  PrivacyPolicy    `json:"current_privacy"`
	Profiles        []PrivacyProfile `json:"profiles"`
	NotificationsOn bool             `json:"notifications_enabled"`
	LockEnabled     bool             `json:"lock_enabled"`
	Locked          bool             `json:"locked"`
	Settings        AppSettings      `json:"settings"`
}

type AppStateEvent struct {
	Kind     AppEventKind     `json:"kind"`
	Revision uint64           `json:"revision"`
	Snapshot AppStateSnapshot `json:"snapshot"`
}

// AppStateController owns the current native store snapshot and emits typed
// change events. Feature-specific policy transitions are added in their
// milestones; page JavaScript is not the owner of persistent application state.
type AppStateController struct {
	mu          sync.Mutex
	loaded      bool
	store       ApplicationStore
	loadReport  StoreLoadReport
	loadErr     error
	locked      bool
	forceLocked bool
	revision    uint64
	nextSubID   uint64
	subscribers map[uint64]chan AppStateEvent
}

func NewAppStateController() *AppStateController {
	return &AppStateController{subscribers: make(map[uint64]chan AppStateEvent)}
}

var applicationState = NewAppStateController()

func (c *AppStateController) ensureLoadedLocked() {
	if c.loaded {
		return
	}
	c.store, c.loadReport, c.loadErr = loadApplicationStore()
	if c.store.SchemaVersion != appStoreSchemaVersion {
		c.store = newApplicationStore(defaultAppSettings())
	}
	if c.loadErr != nil || (c.loadReport.Source == StoreRecoveredDefault && c.loadReport.Detail != "") {
		// A corrupt store may have contained an enabled lock verifier. Do not
		// interpret recovery-to-defaults as permission to expose the WebView.
		c.forceLocked = true
		c.locked = true
	}
	c.loaded = true
	if c.loadReport.Source != StoreLoadedCurrent || c.loadReport.Detail != "" {
		cacheDebugLog("application store source=%s detail=%s", c.loadReport.Source, c.loadReport.Detail)
	}
}

func (c *AppStateController) Snapshot() AppStateSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	return c.snapshotLocked()
}

func (c *AppStateController) snapshotLocked() AppStateSnapshot {
	profiles, profileErr := decodeProfiles(c.store.Profiles)
	if profileErr != nil {
		profiles = nil
	}
	var currentPrivacy PrivacyPolicy
	for _, profile := range profiles {
		if profile.ID == c.store.CurrentProfile {
			currentPrivacy = profile.Privacy
			break
		}
	}
	lockConfig, lockErr := decodeLockConfig(c.store.Lock)
	// A corrupt stored policy fails safe: notifications report off in the
	// snapshot, while resolution itself degrades to content-free presentation.
	notificationPolicy, policyErr := decodeNotificationPolicy(c.store.Notifications, c.store.Settings.NotificationsEnabled)
	notificationsOn := policyErr == nil && notificationPolicy.Enabled
	return AppStateSnapshot{
		Revision:        c.revision,
		CurrentProfile:  c.store.CurrentProfile,
		CurrentPrivacy:  currentPrivacy,
		Profiles:        profiles,
		NotificationsOn: notificationsOn,
		LockEnabled:     lockErr != nil || lockConfig.Enabled || c.forceLocked,
		Locked:          c.locked || c.forceLocked,
		Settings:        c.store.Settings,
	}
}

func rawObjectBool(raw []byte, key string) bool {
	var values map[string]interface{}
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return false
	}
	value, _ := values[key].(bool)
	return value
}

func (c *AppStateController) Settings() AppSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	return c.store.Settings
}

func (c *AppStateController) SaveSettings(settings AppSettings) error {
	_, err := c.UpdateSettings(func(current *AppSettings) { *current = settings })
	return err
}

func (c *AppStateController) UpdateSettings(update func(*AppSettings)) (AppSettings, error) {
	if update == nil {
		return AppSettings{}, errors.New("settings update is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if c.loadErr != nil {
		return c.store.Settings, c.loadErr
	}

	next := c.store.Settings
	update(&next)
	next = normalizeAppSettings(next)
	updatedStore := c.store
	updatedStore.Settings = next
	if err := saveApplicationStore(updatedStore); err != nil {
		return c.store.Settings, err
	}
	c.store = updatedStore
	c.revision++
	c.publishLocked(AppEventSettingsChanged)
	return next, nil
}

func (c *AppStateController) Profiles() ([]PrivacyProfile, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	profiles, err := decodeProfiles(c.store.Profiles)
	return profiles, c.store.CurrentProfile, err
}

func (c *AppStateController) SelectProfile(id string) (PrivacyProfile, error) {
	id = strings.TrimSpace(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	profiles, err := decodeProfiles(c.store.Profiles)
	if err != nil {
		return PrivacyProfile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			updated := c.store
			updated.CurrentProfile = id
			if err := saveApplicationStore(updated); err != nil {
				return PrivacyProfile{}, err
			}
			c.store = updated
			c.revision++
			c.publishLocked(AppEventProfileChanged)
			return profile, nil
		}
	}
	return PrivacyProfile{}, errors.New("unknown privacy profile")
}

func (c *AppStateController) UpdateCustomPrivacy(policy PrivacyPolicy) (PrivacyProfile, error) {
	if err := policy.Validate(); err != nil {
		return PrivacyProfile{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if c.store.CurrentProfile != "custom" {
		return PrivacyProfile{}, errors.New("switch to CUSTOM before editing privacy settings")
	}
	profiles, err := decodeProfiles(c.store.Profiles)
	if err != nil {
		return PrivacyProfile{}, err
	}
	var updatedProfile PrivacyProfile
	found := false
	for i := range profiles {
		if profiles[i].ID == "custom" {
			profiles[i].Privacy = policy.normalized()
			profiles[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updatedProfile, found = profiles[i], true
			break
		}
	}
	if !found {
		return PrivacyProfile{}, errors.New("CUSTOM profile is missing")
	}
	encoded, err := marshalProfiles(profiles)
	if err != nil {
		return PrivacyProfile{}, err
	}
	updated := c.store
	updated.Profiles = encoded
	if err := saveApplicationStore(updated); err != nil {
		return PrivacyProfile{}, err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventProfileChanged)
	return updatedProfile, nil
}

func (c *AppStateController) CopyCurrentProfileToCustom() (PrivacyProfile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	profiles, err := decodeProfiles(c.store.Profiles)
	if err != nil {
		return PrivacyProfile{}, err
	}
	var current PrivacyProfile
	var customIndex = -1
	for i, profile := range profiles {
		if profile.ID == c.store.CurrentProfile {
			current = profile
		}
		if profile.ID == "custom" {
			customIndex = i
		}
	}
	if current.ID == "" || customIndex < 0 {
		return PrivacyProfile{}, errors.New("current or CUSTOM profile is missing")
	}
	custom := profiles[customIndex]
	custom.Privacy = current.Privacy
	custom.NotificationPolicy = current.NotificationPolicy
	custom.LockOverrides = current.LockOverrides
	custom.AppearanceOverrides = current.AppearanceOverrides
	custom.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	profiles[customIndex] = custom
	encoded, err := marshalProfiles(profiles)
	if err != nil {
		return PrivacyProfile{}, err
	}
	updated := c.store
	updated.Profiles, updated.CurrentProfile = encoded, custom.ID
	if err := saveApplicationStore(updated); err != nil {
		return PrivacyProfile{}, err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventProfileChanged)
	return custom, nil
}

func (c *AppStateController) ResetBuiltInProfiles() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	current, err := decodeProfiles(c.store.Profiles)
	if err != nil {
		return err
	}
	defaults, err := decodeProfiles(defaultProfiles(c.store.Settings.BlurAvatars))
	if err != nil {
		return err
	}
	byID := make(map[string]PrivacyProfile, len(current))
	for _, profile := range current {
		byID[profile.ID] = profile
	}
	for _, profile := range defaults {
		if profile.ID == "custom" {
			continue
		}
		byID[profile.ID] = profile
	}
	profiles := make([]PrivacyProfile, 0, len(byID))
	for _, id := range []string{"normal", "office", "presentation", "maximum-privacy", "custom"} {
		if profile, ok := byID[id]; ok {
			profiles = append(profiles, profile)
			delete(byID, id)
		}
	}
	for _, profile := range byID {
		profiles = append(profiles, profile)
	}
	encoded, err := marshalProfiles(profiles)
	if err != nil {
		return err
	}
	updated := c.store
	updated.Profiles = encoded
	if err := saveApplicationStore(updated); err != nil {
		return err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventProfileChanged)
	return nil
}

// MarkLocked and MarkUnlockedAfterNativeVerification are native-only state
// transitions. No JavaScript bridge exposes the unlock transition.
func (c *AppStateController) MarkLocked() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if !c.locked {
		c.locked = true
		c.revision++
		c.publishLocked(AppEventLockChanged)
	}
}

func (c *AppStateController) MarkUnlockedAfterNativeVerification() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if !c.forceLocked && c.locked {
		c.locked = false
		c.revision++
		c.publishLocked(AppEventLockChanged)
	}
}

func (c *AppStateController) NeedsExplicitStoreReset() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	return c.forceLocked || c.loadErr != nil
}

func (c *AppStateController) ResetCorruptStoreAfterNativeConfirmation() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if !c.forceLocked && c.loadErr == nil {
		return errors.New("application store does not require recovery")
	}
	reset := newApplicationStore(defaultAppSettings())
	if err := saveApplicationStore(reset); err != nil {
		return err
	}
	c.store = reset
	c.loadErr = nil
	c.forceLocked = false
	c.locked = false
	c.revision++
	c.publishLocked(AppEventLockChanged)
	return nil
}

func (c *AppStateController) Subscribe(buffer int) (<-chan AppStateEvent, func()) {
	if buffer < 1 {
		buffer = 1
	}
	c.mu.Lock()
	c.ensureLoadedLocked()
	c.nextSubID++
	id := c.nextSubID
	ch := make(chan AppStateEvent, buffer)
	c.subscribers[id] = ch
	c.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			c.mu.Lock()
			if current, ok := c.subscribers[id]; ok {
				delete(c.subscribers, id)
				close(current)
			}
			c.mu.Unlock()
		})
	}
	return ch, cancel
}

func (c *AppStateController) publishLocked(kind AppEventKind) {
	event := AppStateEvent{Kind: kind, Revision: c.revision, Snapshot: c.snapshotLocked()}
	for _, ch := range c.subscribers {
		select {
		case ch <- event:
		default:
			// Keep the latest state if a listener is slower than the producer.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}
