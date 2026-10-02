# Local Data Contract

This document defines the native local store contract. Milestone 2 introduced its version 1 envelope in `app_store.json`; Milestone 3 expands the version 1 profile and lock records. The store migrates the current flat `settings.json` values on first load. The legacy file is retained unchanged after migration.

## 1. Store Location and File Handling

Use the current user's WhatsAppDesk config/support directory already used by `settings.go`. The current implementation writes `app_store.json` beside `settings.json`; it leaves `settings.json` in place and does not access or move WebView profile directories. Window geometry remains in the existing `window_state.json` under the platform WebView data directory. One versioned JSON file is the default because current data volumes are small and there is one app instance; split files only if measured size/concurrency justifies it.

Write by validating the in-memory model, marshaling to a sibling temporary file with restrictive permissions, flushing and closing it, then atomically replacing the primary file using platform-appropriate semantics. Keep one previous valid backup. On read, validate version and records before applying. If current is corrupt, try backup; if both fail, use safe defaults, report recovery, and preserve corrupt files for optional local diagnosis. Never reset the WebView profile.

## 2. Version 1 Shape

Illustrative JSON contract (field names may be represented as Go structs but semantics are normative):

```json
{
  "schema_version": 1,
  "settings": {},
  "current_profile_id": "normal",
  "profiles": [],
  "lock": {},
  "notifications": {},
  "pins": [],
  "bookmarks": [],
  "labels": [],
  "notes": [],
  "appearance": {},
  "updated_at": "RFC3339 UTC"
}
```

All timestamps are UTC RFC3339. IDs are random local IDs, not WhatsApp server IDs. Unknown fields are ignored for forward compatibility; unknown schema versions are not overwritten. The app must not mutate a newer schema it cannot understand.

## 3. Records

### Settings

Preserve existing fields: download directory, download month organization, notification enabled, theme, spell-check enabled/language, blur avatars, crash-report-notified timestamp. Add only general app preferences such as launch at login, minimize/close behavior, shortcut overrides if supported. Existing `settings.json` is migrated once; invalid download paths fall back to the existing safe default.

### Profiles

```text
Profile {
  id: stable local string
  kind: NORMAL | OFFICE | PRESENTATION | MAXIMUM_PRIVACY | CUSTOM
  name: local display name
  built_in: bool
  privacy: PrivacyPolicy
  notification_policy: NotificationPolicy
  lock_overrides: optional LockPolicyOverrides  // may tighten user-wide policy only
  appearance_overrides: optional AppearanceOverrides
  updated_at: timestamp
}
```

Built-in profile IDs are stable: `normal`, `office`, `presentation`, and `maximum-privacy`. `custom` is user-editable and is not reset when built-ins are restored. User customization of built-ins is represented by copying the selected profile into CUSTOM; do not mutate an unknown profile silently.

`LockPolicyOverrides` may set a shorter non-zero idle timeout and turn on additional lock triggers such as minimize/tray. Effective policy is the stricter combination of the user-wide lock config and active profile; a profile never disables a globally enabled lock trigger or lengthens a user-selected timeout.

```text
LockPolicyOverrides {
  idle_timeout_seconds: optional positive integer // 0/omitted means inherit
  lock_on_minimize: optional true                 // false/omitted means inherit
  lock_on_tray: optional true
  lock_on_startup: optional true
}
```

### PrivacyPolicy

Boolean settings per surface: chat names, group names, avatars, preview, timestamps, unread count, message text, image, video, sticker, quoted/replied content, voice-note identifying details, header name/avatar/subtitle, media viewer. Include `reveal_mode` (`hover`, `click`, `modifier`) and modifier key. Distinguish unread count from unread preview.

### LockConfig

```text
LockConfig {                                      // verifier fields required when enabled
  enabled: bool
  kdf: "argon2id"
  kdf_version: integer
  salt: base64 random bytes
  verifier: base64 derived bytes
  recovery_salt: base64 random bytes              // required when enabled
  recovery_verifier: base64 derived bytes         // required when enabled
  memory_kib: bounded integer
  iterations: bounded integer
  parallelism: bounded integer
  idle_timeout_seconds: bounded integer
  lock_on_minimize: bool
  lock_on_tray: bool
  lock_on_startup: bool
}
```

Never add a raw PIN/password or recovery-code field. The recovery code is generated randomly and displayed once; only its separately salted verifier is stored. Do not persist failure backoff as the security authority because same-user local edits can reset it; runtime rate limiting is enforced in process. KDF parameters are validated against hard bounds before use to prevent malicious resource exhaustion.

### NotificationPolicy

Stored under the store's `notifications` key since M4-01 (implemented):

```text
NotificationPolicy {
  enabled: bool
  sender_visibility: show | hide | generic
  body_visibility: show | hide
  focused_behavior: allow | generic | suppress
  background_behavior: allow | generic | suppress
  locked_behavior: generic | suppress   // never allow full content while locked
  quiet_hours: { enabled, start_local "HH:MM", end_local "HH:MM" }
  sound: { enabled }
}
```

Resolution order: master switch → locked floor (generic-or-suppress only) →
quiet hours (suppress) → focused/background behavior → per-field sender/body
redaction. The page proposes a bounded `NotificationEvent { title, body, tag,
chat_type: unknown|private|group, focused }`; the native driver receives only
the resolved `NotificationPresentation { title, body, sound, suppressed }`.
A corrupt stored policy degrades to a content-free policy (generic title, no
body) instead of leaking content. Private/group rule overrides are reserved
until the adapter reports a trusted chat type (M5).

Per-profile overrides (`privacy_profile.notification_policy`, implemented as
`ProfileNotificationOverrides`): `enabled`, `sender_visibility`,
`body_visibility`, `focused_behavior`, `background_behavior`,
`locked_behavior`, `quiet_hours_enabled`, `sound_enabled` — all optional;
omitted fields inherit the base policy. Overrides cannot weaken the locked
floor; invalid merged values degrade to the content-free policy.

Legacy `settings.notifications_enabled` remains in sync as the initial
enabled state for stores whose `notifications` key was never configured.

### Local Pin

```text
Pin {
  id: random local ID
  chat_key: adapter-issued local opaque key
  identity_kind: selector/attribute provenance enum
  adapter_version: integer
  confidence: high | medium
  display_label: user alias or minimal last-seen visible title
  order: integer
  created_at, updated_at: timestamps
}
```

Never invent an identity for an ambiguous chat. `chat_key` must not be presented as a permanent WhatsApp ID; adapter migration can mark a pin unresolved. There is no application-defined limit on pin count; safe file-size/decoding guards may exist but must not silently truncate records.

Implemented in M5: the DOM adapter (v2) issues `chat:`/`msg:` keys as 16-hex
double-FNV hashes of the provenance-prefixed raw attribute (`data-id` →
confidence high; visible title → `title-fallback`, medium, rename-sensitive).
Raw WhatsApp attributes never leave the adapter. Ambiguous or missing inputs
produce no key. The store keeps `pins`, `bookmarks`, `labels`, and `notes`
with strict validation: opaque-key charset, adapter-version pinning, alias
length caps, bookmark duplicates (chat+message) rejected, excerpts rejected
outright in M5, label references resolved or rejected, and note text bounded
at 8 KiB rendered as text only.

### Bookmark

```text
Bookmark {
  id: random local ID
  chat_key: adapter-issued local opaque key
  message_key: adapter-issued visible-message key
  identity_kind, adapter_version, confidence
  chat_label: minimal cached display label
  message_kind: text | image | video | audio | document | sticker | unknown
  message_time: optional visible timestamp
  excerpt: optional, off by default, explicit user opt-in
  label_ids: []
  created_at, updated_at: timestamps
}
```

Do not copy complete message or conversation bodies. Unsupported/changed identities remain unresolved and cannot open a different message silently.

### Labels and Notes

`Label` is a local id/name/color with timestamps. `Note` is a local id, optional `chat_key` or `bookmark_id`, note text, and timestamps. Notes are personal content; disclose that they are stored unencrypted in the user profile unless a future approved encryption design changes that. Apply length limits and render as text, never HTML.

### Appearance

Include compact mode, density enum, scale percent, hide-section preferences, custom CSS text, enabled flag, and hash/version of last-known-good CSS. Keep last-known-good CSS recoverable if new CSS fails validation. Exclude app lock/control center styling from custom CSS scope.

Implemented in M6: `appearance` stores compact_mode, density
(comfortable|cozy|compact), scale_percent (80-130), hide_unread_badges,
hide_archived_row, custom_css, css_enabled, and last_known_good_css/hash.
Validation rejects @import, every url() form, script/style breakout
constructs, unbalanced braces, and CSS above 64 KiB; the last-known-good
copy only ever tracks CSS that passed validation. Recovery paths: the
Ctrl+Shift+X page shortcut, the Control Center disable/reset buttons,
and reset-to-defaults — the native lock dialog is outside any CSS
surface by construction.

## 4. Migration

Migration from current `settings.json`:

1. Read with current safe defaults and existing download directory validation.
2. Map current fields without changing their meaning.
3. Add the five built-in profile records and carry `blur_avatars` into `NORMAL.privacy.avatars`; add missing policy fields from profile defaults without replacing fields already stored.
4. Do not migrate the old browser localStorage `wa_desk_privacy_autolock` value as an app lock. It represented visual auto-blur, not credential protection, and the insecure page-only auto-unlock path is removed.
5. Keep the original file until the new store is flushed and re-read successfully.
6. Treat a valid versioned primary store as migration completion so retries are idempotent. Never remove WebView data or existing window geometry.

Milestone 2 writes the versioned primary atomically, verifies it by reading it back, retains a valid previous store as `.bak` on later updates, and leaves the original `settings.json` untouched. Recovery preserves invalid primary bytes under an `.corrupt-*` name and keeps an invalid backup in place for diagnosis.

## 5. Validation and Limits

- Reject invalid enum values, out-of-range timeouts/KDF costs, duplicate IDs, broken label references, negative ordering values if unsupported, and invalid local paths.
- Bound names, note text, CSS size, total JSON size, and arrays against memory abuse. Array validation must not impose a product pin quota or silently drop records; return an actionable error if storage exceeds an overall safety ceiling.
- Use unique local IDs and deterministic validation order to make migrations/tests repeatable.
- Unknown optional fields can be preserved only if round-trip safety is proven; otherwise keep a backup and report unsupported schema rather than destroying them.

## 6. Required Storage Tests

Atomic write/replace, interrupted write, corrupt primary/valid backup, corrupt both, unknown schema, v0-to-v1 migration, repeat migration, path/symlink validation, KDF parameter bounds, invalid IDs/references, large record handling, no silent pin truncation, and proof that profile/window state migration never touches WebView cookies/session files.
