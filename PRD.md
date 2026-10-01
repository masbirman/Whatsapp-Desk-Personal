# Product Requirements — WhatsApp Desk Personal

**Status:** Planning baseline, not implementation authorization by itself.  
**Product:** WhatsApp Desk Personal  
**Platforms:** Windows and Ubuntu/Debian Linux.

## 1. Purpose

Provide a personal desktop shell for WhatsApp Web with local privacy controls, a real app lock, privacy-aware desktop notifications, native desktop integration, and local productivity enhancements. WhatsApp Web remains the communication core and the user signs in using WhatsApp's normal flow.

## 2. Users and Core Use Cases

- A person using one WhatsApp account in a desktop window who needs to conceal chats during a meeting or while stepping away.
- A person who wants notifications without exposing sender or message text while the screen is shared or the app is locked.
- A person who wants local pins, bookmarks, labels, or notes without changing WhatsApp server data or saving whole conversations.
- A person who wants to save a status/story they explicitly opened.
- A person who wants a compact, calmer WhatsApp Web layout and fast tray/keyboard access.

## 3. Goals

- Keep all new settings and metadata local to the current OS user.
- Keep Windows and Ubuntu/Debian as first-class targets with native WebView and notification/tray behavior.
- Reduce DOM fragility by routing WhatsApp page queries through one adapter/selector registry.
- Preserve WhatsApp Web login/session when locking, changing profiles, updating settings, or using the tray.
- Keep the app light, with event-driven UI observation and no continuous full-page scans.

## 4. Non-Goals

- SaaS, cloud account, sync service, analytics, telemetry, custom backend, proxy, or message relay.
- CRM, marketing automation, bot, bulk sender, automatic status archive, contact scraping, or mass data export.
- Private WhatsApp protocol or bypass of server-side restrictions, including server pin limits.
- macOS feature parity, new macOS features, macOS validation/build/package work in the current scope.
- Replacing the existing Go/WebView foundation with another desktop framework.

## 5. Functional Requirements

### 5.1 Privacy and Profiles

- **PRIV-01:** User can independently enable blur for chat/group names, avatars, previews, timestamps, unread count, message text, images, video, stickers, quoted/replied content, voice-note identifying details, conversation header, and media viewer.
- **PRIV-02:** Reveal behavior is configurable: hover, click, or modifier key. The default must be usable with mouse and keyboard; privacy does not depend on a single global blur switch.
- **PRIV-03:** User can select NORMAL, OFFICE, PRESENTATION, MAXIMUM PRIVACY, or CUSTOM profiles. Profiles locally define privacy, notification redaction, optional stricter lock behavior, and optional appearance behavior. A profile may tighten global lock policy but cannot silently weaken a user-enabled lock trigger.
- **PRIV-04:** Profile changes apply without reloading or changing the WhatsApp session. Recovery/reset remains available even if a profile or selector fails.
- **PRIV-05:** Hide-unread-count is distinct from hiding unread preview text.

### 5.2 App Lock

- **LOCK-01:** User can enable/disable a local PIN or password credential; the raw credential is never stored.
- **LOCK-02:** User can manually lock, lock through a keyboard shortcut, and unlock through a blocking app-owned lock screen.
- **LOCK-03:** User can configure idle timeout and optional lock on minimize, entering tray, and startup.
- **LOCK-04:** Lock hides/blocks WhatsApp page content and input until successful unlock. The lock prompt and credential input are native-owned on Windows/Linux, cover the WebView, and never accept credentials from page JavaScript. Lock state controls notification redaction.
- **LOCK-05:** Locking/unlocking never logs out, clears, or replaces the WhatsApp Web profile/session.
- **LOCK-06:** Failed attempts are rate-limited; credential storage uses random salt and a memory-hard KDF. Setup generates a one-time recovery code; its verifier is stored, never the code. The recovery code can reset the app credential without changing session data. Recovery/reset behavior must be explicit and cannot silently remove session data.

### 5.3 Notifications

- **NOTIF-01:** User can enable/disable native notifications and choose sender visibility, body visibility, or generic text.
- **NOTIF-02:** Rules can vary for private chats and groups, focused/background app state, active privacy profile, quiet hours, and locked state.
- **NOTIF-03:** Locked state can always force generic notification text, e.g. “New WhatsApp message”. Less restrictive profiles cannot override the global locked-state privacy floor.
- **NOTIF-04:** Sound can be configured where the platform permits; unsupported settings are clearly disabled or explained.
- **NOTIF-05:** Notification errors or missing desktop daemons do not crash the app. Message content is not logged.
- **NOTIF-06:** Windows notification activation brings the app window forward. Opening a particular chat is only supported if the adapter provides a robust target; otherwise activation opens the app window.

### 5.4 Windows and Linux Desktop Integration

- **DESK-01:** Windows uses native Notification Center toast, WebView2, and a system tray icon with Open, Privacy, Lock, notification enable/disable and mute controls, Settings, and Quit actions.
- **DESK-02:** Linux uses WebKitGTK and desktop notifications suitable for Ubuntu/Debian, and extends the existing StatusNotifierItem with Open, Privacy, Lock, notification enable/disable, Settings, and Quit actions.
- **DESK-03:** Minimize-to-tray and optional close-to-tray are configurable; restore returns to the same window and session. Lock-on-tray is independently configurable.
- **DESK-04:** Tray absence, DBus watcher absence, notification daemon failure, or Windows Shell restart degrades safely and is surfaced without losing session.
- **DESK-05:** Unread indication updates from supported visible DOM signals. Windows uses a numeric taskbar overlay only if reliable; otherwise use a native unread overlay/fallback without private WhatsApp APIs.
- **DESK-06:** Auto-start behavior remains opt-in and can be queried accurately on the target OS.

### 5.5 Local Productivity

- **PROD-01:** User can locally pin an unlimited number of chats; pins can be added, removed, reordered, and opened quickly when a robust target is available.
- **PROD-02:** User can bookmark and remove a specific visible message, browse bookmarks, and optionally add a tag/category.
- **PROD-03:** Labels/tags and personal notes are local. No complete conversation is copied into the local database.
- **PROD-04:** Persistent identity is based on adapter-provided visible DOM identifiers only. If no sufficiently reliable identifier/context exists, the app must explain the limitation and avoid creating a misleading bookmark/pin.
- **PROD-05:** Local filtering may filter locally stored metadata, not invoke private WhatsApp search APIs.

### 5.6 Status/Story Download

- **STORY-01:** User explicitly selects an opened image/video status/story and chooses Download or Save As.
- **STORY-02:** No automatic collection, background archive, or bulk status scrape is performed.
- **STORY-03:** Reuse the existing download subsystem, filename sanitization, path validation, deduplication, and user-selected download directory.
- **STORY-04:** If the media URL/blob is inaccessible or DOM no longer identifies the open status, fail safely and explain the limitation.

### 5.7 Appearance and Settings

- **UI-01:** Control Center groups settings into GENERAL, PRIVACY, NOTIFICATIONS, APP LOCK, PRODUCTIVITY, APPEARANCE, DOWNLOADS, SHORTCUTS, and ABOUT / UPDATE.
- **UI-02:** Appearance supports compact mode, density, sidebar options, optional hiding of Channels/Communities/Status/Updates/navigation elements, and safe UI/font scaling.
- **UI-03:** Custom CSS is local, disabled/enabled explicitly, validated/restricted, and has reset-to-default and recovery paths independent of the CSS itself.
- **UI-04:** The lock screen and app-owned controls cannot be hidden/overridden by user CSS.
- **UI-05:** Settings work at minimum supported window size, are keyboard accessible, and do not become a generic dashboard replacing WhatsApp Web.

## 6. Data and Privacy Requirements

- **DATA-01:** Settings, profiles, lock config, notification rules, local pins/bookmarks/labels/notes, and appearance are stored under the current user profile.
- **DATA-02:** Store a schema version, write atomically, recover from malformed/partial data using a previous valid backup or safe defaults, and report recovery to the user.
- **DATA-03:** Store only minimum metadata needed for local features; never persist full conversation history, message bodies by default, authentication secrets, or WebView cookies in app metadata.
- **DATA-04:** PIN/password and one-time recovery code are represented only by separate KDF identifiers/parameters, random salts, and derived verifiers. Neither secret is stored, logged, or exposed in bridge diagnostics; the recovery code is shown once during setup.
- **DATA-05:** Migration preserves existing `settings.json`, window state, and WebView profile. Migration is idempotent and has backup/rollback tests.

## 7. Compatibility and Security Requirements

- **COMPAT-01:** All new WhatsApp selector use goes through the DOM adapter/registry and has fixture coverage for preferred selector, fallback, absent node, and ambiguous node.
- **COMPAT-02:** Feature observers are targeted and are paused/throttled when the app is hidden or the user scrolls; listeners can be cleaned up.
- **SEC-01:** Native bridge validates all strings, enum values, identifiers, file paths, payload sizes, and state transitions on the Go side.
- **SEC-02:** No arbitrary command execution, arbitrary-file open, external CSS import, remote stylesheet, or user-controlled URL outside explicit allowlists.
- **SEC-03:** External link hostname validation checks exact host or a dot-delimited subdomain boundary.
- **SEC-04:** Fork updater only checks and installs assets from `masbirman/Whatsapp-Desk-Personal`; it cannot replace the app with upstream release binaries.
- **SEC-05:** Keep upstream LICENSE and required source attribution.

## 8. Platform Acceptance Baseline

**Windows:** Windows 10/11 x64, WebView2 runtime, notification enabled/denied, tray add/remove/restore, taskbar unread fallback, startup toggle, lock/unlock, monitor/window restore, update path restricted to fork.

**Linux:** Ubuntu/Debian supported x64 desktop with WebKitGTK 4.0/4.1 where packaged; test X11 and Wayland behavior where available, DBus watcher present/absent, notify daemon present/absent, tray menu, startup entry, lock/unlock, window restore and download path validation.

Platform runtime coverage is claimed only on systems where those checks actually run. macOS source remains build-compatible where shared changes require it, but is not an acceptance target for new work.

## 9. Release Criteria

- Required Go and JavaScript/DOM tests pass.
- Windows x64 build/static checks pass; Windows runtime validation is recorded separately.
- Ubuntu/Debian Linux build and target runtime checklist are completed for release candidates.
- No unexpected changes to WebView session/profile paths.
- Security review covers lock data, notification redaction, bridge, path, CSS, URL, and updater trust boundaries.
- User-facing documentation, installer identity, updater repository, and fork attribution agree.
