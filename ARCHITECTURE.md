# Architecture — WhatsApp Desk Personal

**Status:** Milestone 2 foundation and Milestone 3 privacy/app-lock implementation are present. Linux build/tests and a Windows x64 cross-build pass; Windows/Linux GUI runtime and live WhatsApp DOM remain unverified. Tray lock wiring and later feature milestones remain future work.

## 1. Initial Architecture (Audited Before M2)

- Go `package main` owns app startup, updater, settings, file operations, and platform selection.
- `main.go:getInitScript` returns a large JavaScript/CSS string. It contains privacy, notifications, download handling, settings UI, appearance, shortcuts, and DOM observers.
- `app_windows.go` creates WebView2 with an explicit user data path and binds Go functions into the page. `app_linux.go` creates WebKitGTK and implements tray through GDBus StatusNotifierItem. `app_darwin.go` contains WKWebView/Cocoa; macOS is not a current development target.
- Before M2, `settings.go` persisted a flat `AppSettings` JSON file. Page-local preferences also use WebView localStorage.
- Bridge registrations are repeated in platform files. The Go side has no cohesive service registry yet.
- At the M1 baseline, DOM selectors and heuristics were spread through the injected script, with no central DOM compatibility layer.
- Updater identity is upstream-bound. New fork releases must not be installed until this is corrected in the planned hardening milestone.

Source baseline: `REPOSITORY_AUDIT.md`, `main.go`, `app_*.go`, `settings.go`, `updater.go`, `checksum.go`.

## 2. Milestone 2 Foundation (Implemented)

- `dom_adapter.js` is embedded into the injected WebView script and provides a versioned selector registry, preferred/fallback resolution, ambiguity reporting, scoped close-control fallback, and DOM-order candidate lookup where legacy behavior depends on it. Existing selector use has been moved incrementally; feature policy remains in the current script.
- `bridge_contract.go` defines typed, bounded requests for the existing native bridge operations. Platform bridge callbacks validate their inputs before invoking existing OS or file operations.
- `app_store.go` owns version 1 of `app_store.json`, stored beside legacy `settings.json`. It atomically replaces files, retains a valid backup, recovers from corrupt data, and migrates settings idempotently. Migration leaves the existing `settings.json` and `window_state.json` paths alone and does not access, move, or reset WebView profile data.
- `app_state.go` provides the shared native settings, profile, privacy, and lock state with revisioned typed events. Native lock credentials are not exposed through a page bridge.
- Linux unit tests/build and a Windows x64 cross-build cover the compiled implementation. Platform GUI runtime integration has not been exercised.

## 2.1 Milestone 3 — Privacy and App Lock

- `privacy_profile.go` defines five built-in profile records, granular per-surface privacy policy, migration from legacy avatar blur, reset/copy behavior, and profile overrides that can only tighten global lock settings.
- `dom_adapter.js` owns selectors for chat-list fields, messages, media, quotes, voice-note details, conversation header, and media viewer. The injected privacy controller tags each surface independently and provides hover, click, modifier, keyboard, and manual privacy controls.
- `app_lock.go` stores separate Argon2id verifiers for the app credential and one-time recovery code, enforces bounded derivation cost and process-local backoff, and keeps setup pending until the user confirms saving the recovery code.
- `app_lock_ui.go` owns lock transitions and native recovery/setup flows. `native_lock_windows.go` provides native Win32 credential prompts; Linux uses GTK dialogs. The WebView window is hidden while locked. Only verified native flow can mark the app unlocked.
- Manual, shortcut, idle, minimize, and startup paths are implemented. Tray lock is not connected yet. Notification redaction remains the next milestone, and native dialogs have not been GUI runtime-verified.

## 3. Target Runtime Flow

```text
Go startup
  ├─ load/migrate local settings and feature data
  ├─ start platform WebView + minimal NativeHost
  ├─ register validated native bridge methods
  └─ inject app modules before navigating to WhatsApp Web
        ├─ app-owned UI shell (isolated from user CSS)
        ├─ DOM compatibility adapter / selector registry
        ├─ feature controllers (privacy, lock, notifications, productivity...)
        └─ typed bridge requests for OS/file/storage actions
```

The main WebView profile remains owned by the current WebView implementation. App lock changes visibility/input and notification policy; it never clears browser website data.

## 4. Incremental Module Boundaries

The first refactor should preserve the current Go module and `package main`, use cohesive Go files and extracted embedded assets, and avoid premature framework/package churn. Later package boundaries are allowed when tests and platform-specific imports remain simple.

| Boundary | Responsibilities | Must not own |
|---|---|---|
| `internal/dom` or equivalent JS adapter assets | Selector registry, candidate resolution, chat/message/media/status detection, stable adapter DTOs, compatibility version and diagnostics. | Persistent feature state, OS calls, private WhatsApp APIs. |
| `internal/privacy` or equivalent | Granular policy, profile application, privacy visibility/reveal behavior, lock-aware floor. | Native notifications, WebView profile/session. |
| `internal/lock` | Credential KDF metadata, verify/backoff policy, lock state machine and timeout policy. | Raw credential persistence, session deletion, UI/OS-specific calls. |
| `internal/notifications` | Notification event normalization, quiet hours, profile/lock redaction, policy decision. | Direct platform APIs or unredacted logging. |
| `internal/store` | Versioned local records, validation, atomic write, backup/recovery/migration. | DOM scanning, WebView cookies, remote sync. |
| `internal/productivity` | Pins, bookmarks, labels, notes and minimum identity metadata. | Full conversation storage or server mutations. |
| `internal/appearance` | Density/layout settings, safe custom CSS validation/scope and reset. | Ability to override app-owned lock/control UI. |
| `internal/bridge` | Typed request/result contracts and shared validation/policy dispatch. | General-purpose eval, filesystem paths accepted without validation. |
| Platform host files | WebView creation and Windows/Linux native notification, tray, taskbar and window operations. Existing macOS code retained with minimal shared compatibility. | Feature policy duplicated independently on each OS. |

Exact package names can follow Go idiom and existing dependency constraints during Milestone 2; do not create package cycles to force these conceptual boundaries.

## 5. DOM Compatibility Layer

Feature modules request semantic objects such as `getVisibleChatRows`, `getActiveConversation`, `getMessageFromTarget`, `getOpenMediaViewer`, `getOpenStatusViewer`, and `getUnreadSignal`. The adapter owns all WhatsApp selectors and fallback ordering. It returns small DTOs with:

- element reference for an immediate UI action;
- optional local identity and identity confidence/source;
- visible text only when a current feature needs it;
- adapter/selector version and a diagnostic reason when unresolved.

Prefer `data-testid`, role/ARIA labels, and visible semantic structure. Keep multilingual labels as fallbacks. Do not expose raw internal state or invent selectors from private APIs. Adapter fixture tests must cover missing/changed selectors and ambiguous candidates. Adapter identity is a best-effort DOM hint, not a permanent server identifier.

## 6. Feature State and Policy

- **Privacy state:** active profile plus explicit per-surface settings. Manual privacy toggle composes with profile policy; a privacy floor can only become stricter when locked.
- **Profile state:** one current profile identifier and versioned profile definitions in local app storage. A profile can only tighten lock policy (shorter idle timeout or additional minimize/tray/startup trigger), never silently weaken a user-enabled trigger.
- **Lock state:** `disabled`, `unlocked`, `locked`, `verifying`, and `backoff` states owned by native Go policy; a native lock window/dialog collects credentials and covers/disables the WebView. Page UI cannot unlock itself by dispatching activity or submitting a credential.
- **Notification state:** event → profile/lock/focus/quiet-hours decision → already-redacted payload → platform driver. The native driver never receives hidden sender/body fields.
- **Productivity state:** records keyed by local opaque adapter identity with schema version; missing/low-confidence identities cannot be pinned/bookmarked silently.

State transitions must be idempotent and have a single owner. A profile switch, lock event, app focus event, and settings update cannot each maintain conflicting copies of the same effective policy.

## 7. Native Bridge Contract

Define a small typed bridge contract (method name, validated payload, result/error) for settings, lock-state display (not credential verification), notification policy, tray commands, local records and safe file saving. Keep UI-only WhatsApp behavior in JavaScript; native policy and security validation stay in Go. For notification delivery, send only `{title, body, category, activationTarget?}` after redaction. Credential entry and verification occur in the native lock window/dialog, never through a page bridge; never return the stored verifier or salt to the page. Do not pass arbitrary JavaScript or command lines over the bridge.

Keep UI thread ownership explicit: GTK and AppKit operations must run on their main loop; WebView2 callbacks use their expected apartment/thread. Background file/network work returns results through the platform dispatch function.

## 8. Storage and Migration

Use one versioned native JSON store under the current user's existing WhatsAppDesk config/support directory unless measured size or concurrency requires separate files. Include schema version, settings, profiles, lock verifier metadata, notification rules, pins, bookmarks, labels/notes, and appearance. Keep window geometry in the existing window state file if that avoids coupling settings migration. Use temp write + flush/close + atomic replace, retain one previous valid backup, validate before applying, and provide defaults/recovery when both current and backup are invalid.

The data contract in `DATA_CONTRACT.md` is normative for new persistence. Existing `settings.json` values must migrate without moving or replacing WebView profile directories. Browser localStorage is gradually reduced to transient UI state; migrations must be idempotent.

## 9. Windows Integration

- Keep WebView2 and the existing persistent `DataPath`.
- Present the lock prompt as a native owner-modal window/dialog with native credential control; disable/cover the WebView window until Go verifies the credential. The WebView page never receives the credential or a verify/unlock bridge method.
- Add a native notification-area icon via Win32 Shell notification APIs and message callbacks; tray actions dispatch typed commands to the app controller.
- Keep Toast delivery native and route activation to the window. Redaction happens before Toast construction.
- Evaluate a numeric taskbar overlay using the existing taskbar COM interface; retain an accessible native fallback if icon overlays/counts fail.
- Handle Shell/taskbar recreation, DPI-aware icon resources, tray cleanup, single-instance activation, startup registry behavior, and minimize/close policy.

## 10. Linux Integration

- Keep GTK/WebKitGTK and current X11/Wayland-aware window state behavior.
- Present the lock prompt as a native GTK modal/transient dialog with a native entry; block the main window until Go verifies the credential. The WebView page never receives the credential or a verify/unlock bridge method.
- Extend existing GDBus StatusNotifierItem using a dbusmenu-compatible menu contract; preserve no-tray-host graceful behavior.
- Use desktop notification integration available to Ubuntu/Debian (existing `notify-send` path may remain an initial adapter); do not block UI or fail the app when a daemon is absent.
- Keep startup as an opt-in XDG autostart entry. Validate/quote executable paths and identify the app by its personal fork identity.
- Avoid new global timers; marshal GTK operations to the GTK main context.

## 11. Story Download and Appearance

Status download uses the DOM adapter to identify only the currently opened image/video and invokes the validated existing file saver. No background discovery. Appearance state is local; app-owned controls/lock UI should be isolated in an app-owned shadow root or native-owned surface. Custom CSS must be scoped to WhatsApp content, reject remote imports/URLs and unsafe global selectors, and expose a stable reset/recovery path. Hiding a WhatsApp element remains selector-dependent and must fail closed without breaking the main chat view.

## 12. Updater, Identity, and Attribution

Fork release identity must be centralized and used consistently by version check, allowed URL parser, checksum lookup, issue reporter, About UI, installer/product metadata, Linux desktop entry, Windows AppUserModel identity, and bundle/resource values where relevant. Update URL validation is exact-owner/repository path validation, with HTTPS and redirect policy. Upstream is a source for updates by maintainer workflow only, never an automatic binary source. Preserve LICENSE, copyright, and upstream attribution.

## 13. Refactor and Compatibility Strategy

1. Add characterization tests for current init script and bridge/settings behavior before moving code.
2. Extract DOM adapter without changing feature behavior; migrate selectors feature by feature.
3. Introduce versioned native storage with a tested `AppSettings` migration.
4. Move privacy/lock policy to explicit native state and keep JS as UI/DOM renderer.
5. Route native notifications through a single redaction policy before adding desktop surfaces.
6. Add desktop integration independently per supported OS behind small interfaces.
7. Port remaining features in dependency order from `IMPLEMENTATION_PLAN.md`.

At each stage retain existing WebView profile paths and default behavior where the product requirements do not intentionally change it. macOS-specific work is limited to keeping shared code compilable/coherent.
