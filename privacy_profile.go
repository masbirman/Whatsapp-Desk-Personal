package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type ProfileKind string

const (
	ProfileNormal         ProfileKind = "NORMAL"
	ProfileOffice         ProfileKind = "OFFICE"
	ProfilePresentation   ProfileKind = "PRESENTATION"
	ProfileMaximumPrivacy ProfileKind = "MAXIMUM_PRIVACY"
	ProfileCustom         ProfileKind = "CUSTOM"
)

type RevealMode string

const (
	RevealHover    RevealMode = "hover"
	RevealClick    RevealMode = "click"
	RevealModifier RevealMode = "modifier"
)

// PrivacyPolicy is intentionally granular: every field maps to an independent
// visual surface in the DOM adapter. A false value means visible by default.
type PrivacyPolicy struct {
	ChatNames        bool       `json:"chat_names"`
	GroupNames       bool       `json:"group_names"`
	Avatars          bool       `json:"avatars"`
	Preview          bool       `json:"preview"`
	Timestamps       bool       `json:"timestamps"`
	UnreadCount      bool       `json:"unread_count"`
	MessageText      bool       `json:"message_text"`
	Images           bool       `json:"images"`
	Videos           bool       `json:"videos"`
	Stickers         bool       `json:"stickers"`
	QuotedContent    bool       `json:"quoted_content"`
	VoiceNoteDetails bool       `json:"voice_note_details"`
	HeaderName       bool       `json:"header_name"`
	HeaderAvatar     bool       `json:"header_avatar"`
	HeaderSubtitle   bool       `json:"header_subtitle"`
	MediaViewer      bool       `json:"media_viewer"`
	RevealMode       RevealMode `json:"reveal_mode"`
	RevealModifier   string     `json:"reveal_modifier"`
}

type LockPolicyOverrides struct {
	IdleTimeoutSeconds int  `json:"idle_timeout_seconds,omitempty"`
	LockOnMinimize     bool `json:"lock_on_minimize,omitempty"`
	LockOnTray         bool `json:"lock_on_tray,omitempty"`
	LockOnStartup      bool `json:"lock_on_startup,omitempty"`
}

type PrivacyProfile struct {
	ID                  string               `json:"id"`
	Kind                ProfileKind          `json:"kind"`
	Name                string               `json:"name"`
	BuiltIn             bool                 `json:"built_in"`
	Privacy             PrivacyPolicy        `json:"privacy"`
	NotificationPolicy  ProfileNotificationOverrides `json:"notification_policy,omitempty"`
	LockOverrides       *LockPolicyOverrides `json:"lock_overrides,omitempty"`
	AppearanceOverrides map[string]any       `json:"appearance_overrides,omitempty"`
	UpdatedAt           string               `json:"updated_at"`
}

func (p PrivacyPolicy) Validate() error {
	if p.RevealMode == "" {
		p.RevealMode = RevealHover
	}
	switch p.RevealMode {
	case RevealHover, RevealClick, RevealModifier:
	default:
		return errors.New("unsupported privacy reveal mode")
	}
	modifier := strings.ToLower(strings.TrimSpace(p.RevealModifier))
	if modifier != "" && modifier != "alt" && modifier != "shift" && modifier != "ctrl" && modifier != "control" {
		return errors.New("unsupported privacy reveal modifier")
	}
	return nil
}

func (p PrivacyPolicy) normalized() PrivacyPolicy {
	if p.RevealMode != RevealClick && p.RevealMode != RevealModifier {
		p.RevealMode = RevealHover
	}
	if p.RevealModifier == "" {
		p.RevealModifier = "alt"
	} else {
		p.RevealModifier = strings.ToLower(strings.TrimSpace(p.RevealModifier))
	}
	return p
}

func defaultProfile(kind ProfileKind, now string, legacyBlurAvatars bool) PrivacyProfile {
	policy := PrivacyPolicy{RevealMode: RevealHover, RevealModifier: "alt"}
	name := "Normal"
	profileID := "normal"
	builtIn := true
	switch kind {
	case ProfileOffice:
		name, profileID = "Office", "office"
		policy = fullyPrivatePolicy()
	case ProfilePresentation:
		name, profileID = "Presentation", "presentation"
		policy = fullyPrivatePolicy()
	case ProfileMaximumPrivacy:
		name, profileID = "Maximum Privacy", "maximum-privacy"
		policy = fullyPrivatePolicy()
	case ProfileCustom:
		name, profileID, builtIn = "Custom", "custom", false
	default:
		kind = ProfileNormal
		policy.Avatars = legacyBlurAvatars
	}
	return PrivacyProfile{
		ID: profileID, Kind: kind, Name: name, BuiltIn: builtIn,
		Privacy: policy, NotificationPolicy: ProfileNotificationOverrides{}, UpdatedAt: now,
	}
}

func fullyPrivatePolicy() PrivacyPolicy {
	return PrivacyPolicy{
		ChatNames: true, GroupNames: true, Avatars: true, Preview: true,
		Timestamps: true, UnreadCount: true, MessageText: true, Images: true,
		Videos: true, Stickers: true, QuotedContent: true,
		VoiceNoteDetails: true, HeaderName: true, HeaderAvatar: true,
		HeaderSubtitle: true, MediaViewer: true, RevealMode: RevealHover,
		RevealModifier: "alt",
	}
}

func defaultProfiles(legacyBlurAvatars bool) []json.RawMessage {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	kinds := []ProfileKind{ProfileNormal, ProfileOffice, ProfilePresentation, ProfileMaximumPrivacy, ProfileCustom}
	profiles := make([]json.RawMessage, 0, len(kinds))
	for _, kind := range kinds {
		encoded, _ := json.Marshal(defaultProfile(kind, now, legacyBlurAvatars))
		profiles = append(profiles, encoded)
	}
	return profiles
}

func normalizeProfileRecords(records []json.RawMessage, legacyBlurAvatars bool) ([]json.RawMessage, error) {
	byID := make(map[string]json.RawMessage, len(records)+5)
	for _, raw := range records {
		var profile PrivacyProfile
		if err := json.Unmarshal(raw, &profile); err != nil {
			return nil, fmt.Errorf("decode privacy profile: %w", err)
		}
		if strings.TrimSpace(profile.ID) == "" {
			continue
		}
		if _, exists := byID[profile.ID]; exists {
			return nil, errors.New("duplicate privacy profile identifier")
		}
		byID[profile.ID] = raw
	}
	defaults := defaultProfiles(legacyBlurAvatars)
	for _, raw := range defaults {
		var def PrivacyProfile
		_ = json.Unmarshal(raw, &def)
		prior, exists := byID[def.ID]
		if !exists {
			byID[def.ID] = raw
			continue
		}
		var old map[string]json.RawMessage
		if err := json.Unmarshal(prior, &old); err != nil {
			return nil, fmt.Errorf("decode profile %q: %w", def.ID, err)
		}
		// Fill missing fields from the stable built-in defaults while keeping
		// explicitly stored per-surface values from earlier versions.
		var merged map[string]json.RawMessage
		_ = json.Unmarshal(raw, &merged)
		for key, value := range old {
			merged[key] = value
		}
		var privacy map[string]json.RawMessage
		if rawPrivacy := old["privacy"]; len(rawPrivacy) > 0 {
			_ = json.Unmarshal(rawPrivacy, &privacy)
		}
		if privacy == nil {
			privacy = map[string]json.RawMessage{}
		}
		// M2 stored the legacy avatar option under blur_avatars.
		if value, ok := privacy["blur_avatars"]; ok {
			if _, already := privacy["avatars"]; !already {
				privacy["avatars"] = value
			}
			delete(privacy, "blur_avatars")
		}
		defaultPrivacy, _ := json.Marshal(def.Privacy)
		var completePrivacy map[string]json.RawMessage
		_ = json.Unmarshal(defaultPrivacy, &completePrivacy)
		for key, value := range privacy {
			completePrivacy[key] = value
		}
		merged["privacy"], _ = json.Marshal(completePrivacy)
		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		byID[def.ID] = encoded
	}
	result := make([]json.RawMessage, 0, len(byID))
	for _, id := range []string{"normal", "office", "presentation", "maximum-privacy", "custom"} {
		result = append(result, byID[id])
		delete(byID, id)
	}
	for _, raw := range byID {
		result = append(result, raw)
	}
	return result, nil
}

func decodeProfiles(records []json.RawMessage) ([]PrivacyProfile, error) {
	profiles := make([]PrivacyProfile, 0, len(records))
	seen := map[string]bool{}
	for _, raw := range records {
		var profile PrivacyProfile
		if err := json.Unmarshal(raw, &profile); err != nil {
			return nil, err
		}
		if profile.ID == "" || seen[profile.ID] || len(profile.Name) > 128 || profile.Kind == "" {
			return nil, errors.New("invalid or duplicate profile")
		}
		switch profile.Kind {
		case ProfileNormal, ProfileOffice, ProfilePresentation, ProfileMaximumPrivacy, ProfileCustom:
		default:
			return nil, errors.New("unsupported privacy profile kind")
		}
		if err := profile.Privacy.Validate(); err != nil {
			return nil, err
		}
		if profile.LockOverrides != nil && (profile.LockOverrides.IdleTimeoutSeconds < 0 || profile.LockOverrides.IdleTimeoutSeconds > 24*60*60) {
			return nil, errors.New("profile lock override outside supported bounds")
		}
		profile.Privacy = profile.Privacy.normalized()
		seen[profile.ID] = true
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

type privacyProfileBridgeState struct {
	CurrentProfile string           `json:"current_profile_id"`
	Profiles       []PrivacyProfile `json:"profiles"`
}

func privacyProfileStateJSON() string {
	profiles, current, err := applicationState.Profiles()
	if err != nil {
		return "{}"
	}
	encoded, err := json.Marshal(privacyProfileBridgeState{CurrentProfile: current, Profiles: profiles})
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func selectPrivacyProfileJSON(id string) string {
	if _, err := applicationState.SelectProfile(id); err != nil {
		return "{}"
	}
	return privacyProfileStateJSON()
}

func copyPrivacyProfileToCustomJSON() string {
	if _, err := applicationState.CopyCurrentProfileToCustom(); err != nil {
		return "{}"
	}
	return privacyProfileStateJSON()
}

func resetPrivacyProfilesJSON() string {
	if err := applicationState.ResetBuiltInProfiles(); err != nil {
		return "{}"
	}
	return privacyProfileStateJSON()
}

func updateCustomPrivacyPolicyJSON(raw string) string {
	if len(raw) == 0 || len(raw) > 4096 {
		return "{}"
	}
	var policy PrivacyPolicy
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return "{}"
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "{}"
	}
	if _, err := applicationState.UpdateCustomPrivacy(policy); err != nil {
		return "{}"
	}
	return privacyProfileStateJSON()
}

func effectiveLockPolicy(config LockConfig, profile *PrivacyProfile) LockConfig {
	if profile == nil || profile.LockOverrides == nil {
		return config
	}
	override := profile.LockOverrides
	if override.IdleTimeoutSeconds > 0 && (config.IdleTimeoutSecond == 0 || override.IdleTimeoutSeconds < config.IdleTimeoutSecond) {
		config.IdleTimeoutSecond = override.IdleTimeoutSeconds
	}
	config.LockOnMinimize = config.LockOnMinimize || override.LockOnMinimize
	config.LockOnTray = config.LockOnTray || override.LockOnTray
	config.LockOnStartup = config.LockOnStartup || override.LockOnStartup
	return config
}

func marshalProfiles(profiles []PrivacyProfile) ([]json.RawMessage, error) {
	encoded := make([]json.RawMessage, 0, len(profiles))
	seen := map[string]bool{}
	for i := range profiles {
		profile := profiles[i]
		profile.ID = strings.TrimSpace(profile.ID)
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.ID == "" || len(profile.ID) > 128 || profile.Name == "" || len(profile.Name) > 128 || seen[profile.ID] {
			return nil, errors.New("invalid profile identity")
		}
		if err := profile.Privacy.Validate(); err != nil {
			return nil, err
		}
		profile.Privacy = profile.Privacy.normalized()
		if profile.UpdatedAt == "" {
			profile.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		data, err := json.Marshal(profile)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, data)
		seen[profile.ID] = true
	}
	return encoded, nil
}
