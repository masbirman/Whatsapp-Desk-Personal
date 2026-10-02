package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// M6-03/M6-04 appearance and custom CSS. All of it is page-scoped styling:
// the native lock dialog and every native window control are outside any CSS
// surface by construction. Custom CSS is untrusted input — it is validated
// (size, remote references, script constructs) before it can be enabled, and
// the last valid version is retained for recovery.

const (
	appearanceDensityComfortable = "comfortable"
	appearanceDensityCozy        = "cozy"
	appearanceDensityCompact     = "compact"

	appearanceScaleMin  = 80
	appearanceScaleMax  = 130
	maxCustomCSSBytes   = 64 * 1024
)

type AppearanceSettings struct {
	CompactMode      bool   `json:"compact_mode"`
	Density          string `json:"density"`
	ScalePercent     int    `json:"scale_percent"`
	HideUnreadBadges bool   `json:"hide_unread_badges"`
	HideArchivedRow  bool   `json:"hide_archived_row"`
	CustomCSS        string `json:"custom_css"`
	CSSEnabled       bool   `json:"css_enabled"`
	// LastKnownGood retains the newest CSS that passed validation, so a
	// malformed save can never leave the user without a recovery point.
	LastKnownGoodCSS  string `json:"last_known_good_css,omitempty"`
	LastKnownGoodHash string `json:"last_known_good_hash,omitempty"`
}

func defaultAppearance() AppearanceSettings {
	return AppearanceSettings{
		Density:      appearanceDensityComfortable,
		ScalePercent: 100,
	}
}

func validateAppearanceCSS(css string) error {
	if len(css) > maxCustomCSSBytes {
		return fmt.Errorf("custom CSS exceeds %d KiB", maxCustomCSSBytes/1024)
	}
	lower := strings.ToLower(css)
	for _, forbidden := range []string{"@import", "expression(", "javascript:", "</style", "<script"} {
		if strings.Contains(lower, forbidden) {
			return fmt.Errorf("custom CSS must not contain %s", forbidden)
		}
	}
	// Any url() reference is rejected: remote CSS references and exotic
	// schemes have no legitimate styling purpose here.
	if strings.Contains(lower, "url(") {
		return errors.New("custom CSS must not contain url() references")
	}
	if strings.Count(css, "{") != strings.Count(css, "}") {
		return errors.New("custom CSS braces are unbalanced")
	}
	return nil
}

func cssHash(css string) string {
	sum := sha256.Sum256([]byte(css))
	return hex.EncodeToString(sum[:])
}

func validateAppearance(appearance AppearanceSettings) error {
	switch appearance.Density {
	case appearanceDensityComfortable, appearanceDensityCozy, appearanceDensityCompact:
	default:
		return errors.New("unsupported display density")
	}
	if appearance.ScalePercent < appearanceScaleMin || appearance.ScalePercent > appearanceScaleMax {
		return errors.New("display scale outside the supported range")
	}
	if err := validateAppearanceCSS(appearance.CustomCSS); err != nil {
		return err
	}
	return nil
}

func decodeAppearance(raw json.RawMessage) AppearanceSettings {
	appearance := defaultAppearance()
	if len(raw) == 0 {
		return appearance
	}
	if err := json.Unmarshal(raw, &appearance); err != nil {
		return defaultAppearance()
	}
	if err := validateAppearance(appearance); err != nil {
		// Corrupt or invalid stored appearance degrades to defaults but
		// keeps the last-known-good CSS recovery point if it is present.
		fallback := defaultAppearance()
		fallback.LastKnownGoodCSS = appearance.LastKnownGoodCSS
		fallback.LastKnownGoodHash = appearance.LastKnownGoodHash
		return fallback
	}
	return appearance
}

func (c *AppStateController) Appearance() AppearanceSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	return decodeAppearance(c.store.Appearance)
}

// UpdateAppearance persists a validated appearance change. Enabling custom
// CSS requires the CSS to pass validation; the last-known-good copy is then
// refreshed so recovery always points at something that validated.
func (c *AppStateController) UpdateAppearance(mutate func(*AppearanceSettings)) (AppearanceSettings, error) {
	if mutate == nil {
		return AppearanceSettings{}, errors.New("appearance update is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()
	appearance := decodeAppearance(c.store.Appearance)
	mutate(&appearance)
	if err := validateAppearance(appearance); err != nil {
		return appearance, err
	}
	if appearance.CSSEnabled || appearance.CustomCSS != "" {
		appearance.LastKnownGoodCSS = appearance.CustomCSS
		appearance.LastKnownGoodHash = cssHash(appearance.CustomCSS)
	}
	encoded, err := json.Marshal(appearance)
	if err != nil {
		return appearance, err
	}
	updated := c.store
	updated.Appearance = encoded
	if err := saveApplicationStore(updated); err != nil {
		return appearance, err
	}
	c.store = updated
	c.revision++
	c.publishLocked(AppEventSettingsChanged)
	return appearance, nil
}

// DisableCustomCSS is the recovery path (keyboard shortcut and Control
// Center). It never requires the page to be readable: the shortcut listener
// runs even when custom CSS has visually broken the page.
func (c *AppStateController) DisableCustomCSS() (AppearanceSettings, error) {
	return c.UpdateAppearance(func(appearance *AppearanceSettings) {
		appearance.CSSEnabled = false
	})
}

// --- Bridge handlers ---------------------------------------------------------

func appearanceEnvelope(appearance AppearanceSettings, errValue error) string {
	if errValue != nil {
		data, _ := json.Marshal(map[string]interface{}{"ok": false, "error": errValue.Error()})
		return string(data)
	}
	data, marshalErr := json.Marshal(map[string]interface{}{"ok": true, "appearance": appearance})
	if marshalErr != nil {
		data, _ = json.Marshal(map[string]interface{}{"ok": false, "error": "appearance could not be encoded"})
	}
	return string(data)
}

func getAppearanceJSON() string {
	return appearanceEnvelope(applicationState.Appearance(), nil)
}

func setAppearanceJSON(raw string) string {
	var update AppearanceSettings
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		return appearanceEnvelope(AppearanceSettings{}, errors.New("invalid appearance request"))
	}
	appearance, err := applicationState.UpdateAppearance(func(current *AppearanceSettings) {
		*current = update
	})
	return appearanceEnvelope(appearance, err)
}

func disableCustomCSSJSON() string {
	appearance, err := applicationState.DisableCustomCSS()
	return appearanceEnvelope(appearance, err)
}
