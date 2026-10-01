package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// M4-01 notification policy. The policy and its resolution live entirely in
// native Go: the page may only propose an event, and the OS-facing driver
// receives exactly the resolved presentation — never a hidden sender or body.

const (
	// Generic fallbacks carry no message content or sender identity.
	notificationGenericTitle = "WhatsApp Desk"
	notificationGenericBody  = "You have a new message"

	maxNotificationTagBytes = 128
)

type NotificationSenderVisibility string

const (
	SenderShow    NotificationSenderVisibility = "show"
	SenderHide    NotificationSenderVisibility = "hide"
	SenderGeneric NotificationSenderVisibility = "generic"
)

type NotificationBodyVisibility string

const (
	BodyShow NotificationBodyVisibility = "show"
	BodyHide NotificationBodyVisibility = "hide"
)

type NotificationDeliveryBehavior string

const (
	DeliveryAllow    NotificationDeliveryBehavior = "allow"
	DeliveryGeneric  NotificationDeliveryBehavior = "generic"
	DeliverySuppress NotificationDeliveryBehavior = "suppress"
)

type NotificationLockedBehavior string

const (
	LockedGeneric  NotificationLockedBehavior = "generic"
	LockedSuppress NotificationLockedBehavior = "suppress"
)

type NotificationChatType string

const (
	ChatTypeUnknown NotificationChatType = "unknown"
	ChatTypePrivate NotificationChatType = "private"
	ChatTypeGroup   NotificationChatType = "group"
)

type NotificationQuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start_local"` // HH:MM local time, inclusive start
	End     string `json:"end_local"`   // HH:MM local time, exclusive end
}

type NotificationSoundPolicy struct {
	Enabled bool `json:"enabled"`
}

type NotificationPolicy struct {
	Enabled            bool                         `json:"enabled"`
	SenderVisibility   NotificationSenderVisibility `json:"sender_visibility"`
	BodyVisibility     NotificationBodyVisibility   `json:"body_visibility"`
	FocusedBehavior    NotificationDeliveryBehavior `json:"focused_behavior"`
	BackgroundBehavior NotificationDeliveryBehavior `json:"background_behavior"`
	LockedBehavior     NotificationLockedBehavior   `json:"locked_behavior"`
	QuietHours         NotificationQuietHours       `json:"quiet_hours"`
	Sound              NotificationSoundPolicy      `json:"sound"`
}

// ProfileNotificationOverrides are per-profile adjustments merged over the
// base policy. Only the current profile's overrides apply.
type ProfileNotificationOverrides struct {
	Enabled            *bool                        `json:"enabled,omitempty"`
	SenderVisibility   NotificationSenderVisibility `json:"sender_visibility,omitempty"`
	BodyVisibility     NotificationBodyVisibility   `json:"body_visibility,omitempty"`
	FocusedBehavior    NotificationDeliveryBehavior `json:"focused_behavior,omitempty"`
	BackgroundBehavior NotificationDeliveryBehavior `json:"background_behavior,omitempty"`
	LockedBehavior     NotificationLockedBehavior   `json:"locked_behavior,omitempty"`
	QuietHoursEnabled  *bool                        `json:"quiet_hours_enabled,omitempty"`
	SoundEnabled       *bool                        `json:"sound_enabled,omitempty"`
}

func defaultNotificationPolicy(legacyEnabled bool) NotificationPolicy {
	return NotificationPolicy{
		Enabled:            legacyEnabled,
		SenderVisibility:   SenderShow,
		BodyVisibility:     BodyShow,
		FocusedBehavior:    DeliverySuppress,
		BackgroundBehavior: DeliveryAllow,
		// The locked floor is generic-or-suppress by construction; the schema
		// has no "allow" value while the app is locked.
		LockedBehavior: LockedGeneric,
		QuietHours:     NotificationQuietHours{},
		Sound:          NotificationSoundPolicy{Enabled: true},
	}
}

func validateNotificationPolicy(policy NotificationPolicy) error {
	switch policy.SenderVisibility {
	case SenderShow, SenderHide, SenderGeneric:
	default:
		return errors.New("unsupported notification sender visibility")
	}
	switch policy.BodyVisibility {
	case BodyShow, BodyHide:
	default:
		return errors.New("unsupported notification body visibility")
	}
	switch policy.FocusedBehavior {
	case DeliveryAllow, DeliveryGeneric, DeliverySuppress:
	default:
		return errors.New("unsupported notification focused behavior")
	}
	switch policy.BackgroundBehavior {
	case DeliveryAllow, DeliveryGeneric, DeliverySuppress:
	default:
		return errors.New("unsupported notification background behavior")
	}
	switch policy.LockedBehavior {
	case LockedGeneric, LockedSuppress:
	default:
		return errors.New("unsupported notification locked behavior")
	}
	if policy.QuietHours.Enabled {
		start, err := parseQuietClock(policy.QuietHours.Start)
		if err != nil {
			return err
		}
		end, err := parseQuietClock(policy.QuietHours.End)
		if err != nil {
			return err
		}
		if start == end {
			return errors.New("quiet hours start and end must differ")
		}
	}
	return nil
}

func decodeNotificationPolicy(raw json.RawMessage, legacyEnabled bool) (NotificationPolicy, error) {
	if len(raw) == 0 {
		return defaultNotificationPolicy(legacyEnabled), nil
	}
	var policy NotificationPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return NotificationPolicy{}, err
	}
	// A policy object with no configured behavior (e.g. the `{}` written by
	// earlier store versions) means "never configured": seed the defaults,
	// keeping the legacy master switch as the initial enabled state.
	if policy.SenderVisibility == "" && policy.BodyVisibility == "" &&
		policy.FocusedBehavior == "" && policy.BackgroundBehavior == "" &&
		policy.LockedBehavior == "" && !policy.QuietHours.Enabled {
		return defaultNotificationPolicy(legacyEnabled), nil
	}
	if err := validateNotificationPolicy(policy); err != nil {
		return NotificationPolicy{}, err
	}
	return policy, nil
}

// degradedNotificationPolicy is the fail-safe result of unreadable stored
// policy data: notifications stay on but can only ever render generic text,
// so a corrupt store cannot leak content.
func degradedNotificationPolicy() NotificationPolicy {
	policy := defaultNotificationPolicy(true)
	policy.SenderVisibility = SenderGeneric
	policy.BodyVisibility = BodyHide
	return policy
}

func parseQuietClock(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, errors.New("quiet hours time must be HH:MM")
	}
	hour, errHour := strconv.Atoi(parts[0])
	minute, errMinute := strconv.Atoi(parts[1])
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, errors.New("quiet hours time must be HH:MM between 00:00 and 23:59")
	}
	return hour*60 + minute, nil
}

func quietHoursActive(quiet NotificationQuietHours, now time.Time) bool {
	if !quiet.Enabled {
		return false
	}
	start, errStart := parseQuietClock(quiet.Start)
	end, errEnd := parseQuietClock(quiet.End)
	if errStart != nil || errEnd != nil || start == end {
		return false
	}
	minutes := now.Hour()*60 + now.Minute()
	if start < end {
		return minutes >= start && minutes < end
	}
	// Overnight window (e.g. 22:00-07:00).
	return minutes >= start || minutes < end
}

// mergeNotificationProfileOverrides applies the current profile's overrides
// on top of the base policy. Overrides cannot weaken the locked floor: the
// merged locked behavior must still be generic-or-suppress, and it is forced
// back to generic if an out-of-range value somehow arrives.
func mergeNotificationProfileOverrides(base NotificationPolicy, overrides ProfileNotificationOverrides) NotificationPolicy {
	merged := base
	if overrides.Enabled != nil {
		merged.Enabled = *overrides.Enabled
	}
	if overrides.SenderVisibility != "" {
		merged.SenderVisibility = overrides.SenderVisibility
	}
	if overrides.BodyVisibility != "" {
		merged.BodyVisibility = overrides.BodyVisibility
	}
	if overrides.FocusedBehavior != "" {
		merged.FocusedBehavior = overrides.FocusedBehavior
	}
	if overrides.BackgroundBehavior != "" {
		merged.BackgroundBehavior = overrides.BackgroundBehavior
	}
	if overrides.LockedBehavior != "" {
		merged.LockedBehavior = overrides.LockedBehavior
	}
	if overrides.QuietHoursEnabled != nil {
		merged.QuietHours.Enabled = *overrides.QuietHoursEnabled
	}
	if overrides.SoundEnabled != nil {
		merged.Sound.Enabled = *overrides.SoundEnabled
	}
	switch merged.LockedBehavior {
	case LockedGeneric, LockedSuppress:
	default:
		merged.LockedBehavior = LockedGeneric
	}
	if err := validateNotificationPolicy(merged); err != nil {
		return degradedNotificationPolicy()
	}
	return merged
}

// NotificationPresentation is the only structure handed to the OS-facing
// driver. Suppressed means the driver is not invoked at all.
type NotificationPresentation struct {
	Title      string
	Body       string
	Sound      bool
	Suppressed bool
}

func (p NotificationPresentation) isContentFree() bool {
	if p.Suppressed {
		return true
	}
	titleFree := p.Title == "" || p.Title == notificationGenericTitle
	bodyFree := p.Body == "" || p.Body == notificationGenericBody
	return titleFree && bodyFree
}

// resolveNotificationPresentation is the single redaction decision point.
// Order: master switch -> lock floor -> quiet hours -> focus context ->
// content redaction. Whatever returns here is what the OS driver gets.
func resolveNotificationPresentation(policy NotificationPolicy, event NotificationEvent, locked bool, now time.Time) NotificationPresentation {
	if !policy.Enabled {
		return NotificationPresentation{Suppressed: true}
	}
	if locked {
		switch policy.LockedBehavior {
		case LockedSuppress:
			return NotificationPresentation{Suppressed: true}
		default:
			return NotificationPresentation{
				Title: notificationGenericTitle,
				Body:  notificationGenericBody,
				Sound: policy.Sound.Enabled,
			}
		}
	}
	if quietHoursActive(policy.QuietHours, now) {
		return NotificationPresentation{Suppressed: true}
	}
	behavior := policy.BackgroundBehavior
	if event.Focused {
		behavior = policy.FocusedBehavior
	}
	switch behavior {
	case DeliverySuppress:
		return NotificationPresentation{Suppressed: true}
	case DeliveryGeneric:
		return NotificationPresentation{
			Title: notificationGenericTitle,
			Body:  notificationGenericBody,
			Sound: policy.Sound.Enabled,
		}
	}
	title := event.Title
	body := event.Body
	switch policy.SenderVisibility {
	case SenderHide:
		title = ""
	case SenderGeneric:
		title = notificationGenericTitle
	}
	if policy.BodyVisibility == BodyHide {
		body = ""
	}
	if title == "" && body == "" {
		return NotificationPresentation{
			Title: notificationGenericTitle,
			Body:  notificationGenericBody,
			Sound: policy.Sound.Enabled,
		}
	}
	return NotificationPresentation{Title: title, Body: body, Sound: policy.Sound.Enabled}
}

// Controller-facing helpers -------------------------------------------------

// EffectiveNotificationPolicy returns the base policy merged with the current
// profile's overrides. Unreadable data degrades to the content-free policy.
func (c *AppStateController) EffectiveNotificationPolicy() NotificationPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	base, err := decodeNotificationPolicy(c.store.Notifications, c.store.Settings.NotificationsEnabled)
	if err != nil {
		return degradedNotificationPolicy()
	}
	profiles, profileErr := decodeProfiles(c.store.Profiles)
	if profileErr != nil {
		return base
	}
	for i := range profiles {
		if profiles[i].ID == c.store.CurrentProfile {
			return mergeNotificationProfileOverrides(base, profiles[i].NotificationPolicy)
		}
	}
	return base
}

// ResolveNotificationPresentation applies the effective policy to one bridge
// event using the current lock state. This is the only path from a page
// notification event to the native driver.
func (c *AppStateController) ResolveNotificationPresentation(event NotificationEvent) NotificationPresentation {
	policy := c.EffectiveNotificationPolicy()
	locked := c.Snapshot().Locked
	return resolveNotificationPresentation(policy, event, locked, time.Now())
}

// UpdateNotificationPolicy persists changes to the stored base policy.
func (c *AppStateController) UpdateNotificationPolicy(update func(*NotificationPolicy)) error {
	if update == nil {
		return errors.New("notification policy update is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	if c.loadErr != nil {
		return c.loadErr
	}
	policy, err := decodeNotificationPolicy(c.store.Notifications, c.store.Settings.NotificationsEnabled)
	if err != nil {
		policy = degradedNotificationPolicy()
	}
	update(&policy)
	if err := validateNotificationPolicy(policy); err != nil {
		return err
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	updated := c.store
	updated.Notifications = encoded
	if err := saveApplicationStore(updated); err != nil {
		return err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventNotificationsChanged)
	return nil
}

// NotificationPolicySummary renders a short human-readable description of the
// effective policy for logs and UI; it contains no message content.
func NotificationPolicySummary(policy NotificationPolicy) string {
	if !policy.Enabled {
		return "off"
	}
	parts := []string{}
	if policy.SenderVisibility != SenderShow || policy.BodyVisibility != BodyShow {
		parts = append(parts, fmt.Sprintf("sender=%s body=%s", policy.SenderVisibility, policy.BodyVisibility))
	}
	if policy.FocusedBehavior != DeliveryAllow {
		parts = append(parts, fmt.Sprintf("focused=%s", policy.FocusedBehavior))
	}
	if policy.LockedBehavior != LockedGeneric {
		parts = append(parts, fmt.Sprintf("locked=%s", policy.LockedBehavior))
	}
	if policy.QuietHours.Enabled {
		parts = append(parts, fmt.Sprintf("quiet=%s-%s", policy.QuietHours.Start, policy.QuietHours.End))
	}
	if len(parts) == 0 {
		return "full"
	}
	return strings.Join(parts, ", ")
}
