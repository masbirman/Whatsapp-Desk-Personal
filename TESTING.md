# Testing and Validation Strategy

## 1. Evidence Labels

- **VERIFIED:** The specific test/build/runtime action was run and passed in the named environment.
- **PARTIALLY VERIFIED:** Some code paths or environments were checked; scope and omissions are stated.
- **NOT RUNTIME VERIFIED:** Source/test design exists or static inspection was done, but the target app/OS/live WhatsApp DOM was not run.

Go unit tests, DOM fixtures, OS cross-builds, desktop runtime, and live WhatsApp Web are separate evidence. A source-string assertion never counts as live runtime validation.

## 2. Test Layers

### 2.1 Go Unit Tests

Cover platform-independent policy and storage:

- current `AppSettings` defaults/validation and migration into schema store;
- atomic replace, backup recovery, malformed JSON, unknown schema, idempotent migration;
- path traversal, absolute path, normal path, symlink/dangling symlink and allowed-open directory;
- lock/recovery KDF salt generation/unique salts, correct/incorrect credential and recovery code, parameter bounds, attempt backoff, reset preserving WebView profile;
- profile merge/effective privacy policy and locked-state floor;
- notification decision table, sender/body redaction and quiet hours around midnight/timezone;
- adapter identity validation/provenance DTOs;
- local pin ordering/no silent cap, bookmark minimal data, labels/notes validation;
- notification driver failure mapping, bridge enum/size/path/url rejection;
- updater fork host/path/redirect/asset/checksum/size/architecture validation.

Use temporary test directories and injected clocks/randomness/network drivers. Tests must not access real user WhatsApp profile data.

### 2.2 JavaScript and DOM Fixture Tests

Extend the existing `testdata/init_script_harness.js` pattern or introduce a maintained JS test harness without adding a runtime app dependency. Fixtures should represent supported current patterns, not attempt to clone all of WhatsApp.

For every adapter semantic method, test:

1. preferred selector found;
2. fallback found when preferred selector absent;
3. missing target returns no target with diagnostic reason;
4. two plausible targets return ambiguous/unresolved rather than first-match wrong action;
5. added subtree updates only relevant observers;
6. listener/observer teardown and hidden/scroll throttling;
7. localized accessible labels used only as bounded fallbacks.

Feature DOM tests cover each privacy surface/reveal mode, app lock overlay interaction, notification event normalization, unread title fallback, chat/message identity, currently open status image/video, custom CSS isolation and recovery, and Controls at minimum window layout. These fixtures do not prove compatibility with live WhatsApp.

### 2.3 Security Tests

Use `SECURITY.md` gates. Add adversarial cases for bridge-originated invalid data, HTML and CSS injection, filename traversal, symlink redirects, page-crafted open paths, notification content leakage, PIN verifier exposure, malformed store KDF parameters, updater wrong owner/redirect and missing/mismatched checksums. Assert the page bridge exposes no credential verification method; Windows/Linux native lock dialogs collect and verify credentials outside the WebView.

Assert sensitive text does not appear in logs, crash reports, notification driver calls when redacted, or default bookmark records.

## 3. Windows Validation

### Static/build

- `gofmt` changed Go files.
- `go test ./...` with vendored dependencies when environment supports Go/toolchain.
- Windows x64 cross-build and `go vet`/static checks where possible; state if cgo/toolchain prevents it.
- Validate resource IDs, AppUserModel identity, notification activation, Shell tray callbacks, DPI assets, taskbar fallback, and cleanup paths through unit/test seams.

### Runtime checklist (Windows 10/11)

- WebView2 installed/missing, profile persistence across restart, normal QR/login, lock does not logout.
- Toast enabled/denied, sender/body/generic, group/private, focused/background, quiet hours, locked; activation foregrounds app.
- Tray open/privacy/lock/notification/settings/quit, minimize/close to tray, restore, Explorer restart/re-add icon, no tray mode.
- Taskbar unread appears/updates/clears; numeric overlay or fallback behavior is visually verified.
- Startup toggle add/remove/query, single-instance activation, lock on startup, monitor/window restore.
- Password/PIN entered in native lock UI; correct, incorrect, timeout, backoff, reset/recovery; JS bridge/custom CSS cannot submit or hide/bypass lock.
- Download image/video/status, path chooser, duplicate, unsafe path, OS notification failure.

Until run on actual Windows, every Windows runtime item remains NOT RUNTIME VERIFIED even if cross-build passes.

## 4. Ubuntu/Debian Linux Validation

### Build/static

- Run Go tests with vendored dependencies.
- Build against target WebKitGTK 4.0 and/or 4.1 configuration as claimed by package.
- Validate pkg-config/deb dependencies, XDG autostart Exec quoting/identity, icon install, `notify-send` fallback and DBus methods.

### Runtime checklist

- Ubuntu/Debian target version, architecture, desktop shell and X11/Wayland recorded.
- Login/session persistence across close/reopen, normal QR flow, lock transitions preserve session.
- Privacy selectors/reveal modes with chat list, archived chat, private/group message, images/video/sticker/viewer and voice note fixture/live observation.
- Desktop notifications allow/deny/missing daemon, privacy modes, lock, focus, groups, quiet hours.
- StatusNotifier watcher/menu exists and absent; action dispatch and tray exit/restore; notification indicator.
- X11 window position and Wayland size-only restore; startup entry on/off; single instance.
- Story Save/Save As, image/video, missing media URL, path traversal and duplicate.
- Custom CSS invalid/global/remote/import cases and native reset path.

Ubuntu runtime does not prove Debian package behavior; each target/package claim needs at least one corresponding verification or a stated limitation.

## 5. macOS Scope

No macOS feature work, build, packaging, notification, menu bar, app-lock, or runtime test is planned. Shared changes should retain source compatibility with the existing `app_darwin.go` at minimum. Do not claim a macOS build/test as a gate for this project.

## 6. Live WhatsApp DOM Validation

Live testing requires an authorized logged-in test account/window; never use production personal chat data in automated fixtures or log it. Record WebView engine/version, WhatsApp page date/build if observable, target selector, expected behavior, result and screenshot only if it contains no private data.

Live DOM validation is required for any feature that depends on a current chat row, message, media viewer, status/story viewer, or notification mechanism before that feature is called runtime verified. If no safe test account/session is available, mark those criteria NOT RUNTIME VERIFIED and document impacted features.

## 7. Manual Regression Matrix

| Area | Regression checks |
|---|---|
| Session | Sign-in, restart, upgrade/store migration, lock/unlock, no logout |
| Privacy | Every selector surface, reveal mode, profile change, stale selector recovery |
| Lock | Startup/manual/idle/minimize/tray trigger, wrong/correct credential, backoff, reset |
| Notification | Full/redacted/generic/off and no leak in locked state |
| Desktop | Tray actions, window restore, unread updates, startup, OS permission failures |
| Productivity | Persistent pins/bookmarks, unresolved target, no wrong chat, deletion, no body copy |
| Story | Explicit current viewer only, save image/video, safe file path, no background scan |
| Appearance | compact/density/hide/reset, scale/minimum size, custom CSS recovery |
| Updater | Only personal fork, checksum, platform asset, no upstream overwrite |

## 8. Validation Record — Milestone 3

- **VERIFIED — Go tests and Linux build:** `go test -count=1 ./...` and `go build` passed in a Debian Trixie container with Go 1.26.8, Node.js 20.19.2, GTK 3, and WebKitGTK 4.1 (2.52.6). The build used the repository's WebKitGTK 4.1-to-4.0 pkg-config alias expected by its Linux script. Dependencies were installed in the temporary container, not the host workspace.
- **VERIFIED — Windows x64 cross-build:** `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` passed in the same Linux container.
- **VERIFIED — JavaScript/DOM fixtures:** injected JavaScript parsed and Node DOM adapter fixtures passed for preferred/fallback/missing/ambiguous selectors, including each registered privacy surface.
- **VERIFIED — Ubuntu desktop runtime, native lock dialog (M3-05):** run on the user's Ubuntu GNOME Wayland session with an isolated `XDG_CONFIG_HOME` profile seeded via the real `buildLockConfig`/store path (Argon2id PIN, startup lock, 12s idle timeout). Driven over AT-SPI against the built binary: startup lock dialog appeared before WhatsApp Web navigation with the main window hidden; a wrong credential produced the modal "App lock" failure dialog; the correct credential unlocked and restored the main window (two independent cycles); closing/cancelling the locked prompt re-prompts (fail-closed). After two full lock/unlock cycles the WebView reconnected to WhatsApp (MQTT 5222) and the store file stayed valid with the lock still enabled. Tray registration failure on GNOME degraded to a log line without crashing.
- **VERIFIED — Ubuntu desktop runtime, idle trigger (M3-06):** with `idle_timeout_seconds: 12`, the native watcher re-locked the app after an untouched period and the unlock cycle completed again.
- **NOT RUNTIME VERIFIED:** Windows native dialog behavior (cross-build only); minimize trigger on a real window manager; manual Ctrl+Shift+L page shortcut against live WhatsApp Web; recovery-code unlock through the native dialogs; live WhatsApp DOM privacy surfaces. Tray lock wiring remains deferred to the tray milestone (M4).

## 9. Validation Record — Milestone 4

- **VERIFIED — Go tests, Linux build, Windows x64 cross-build + vet:** every M4 commit was gated on `go test -count=1 ./...`, `go build`, `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` and `go vet` in the same Debian Trixie container.
- **VERIFIED — Ubuntu runtime, notification pipeline (M4-01/M4-03):** a live GNOME desktop notification was delivered through the real controller + policy + notify-send driver path; the daemon ping check passed.
- **VERIFIED — Ubuntu session bus, tray menu (M4-06):** with the built binary running (no tray host installed), the `com.canonical.dbusmenu` object was exercised with `gdbus`: GetLayout returned the full item tree, GetGroupProperties returned labels plus the live notifications toggle-state, AboutToShow/AboutToShowGroup/EventGroup answered, and a clicked Event toggled the notification policy with persistent store effect (True→False→True) without crashing. No-tray-host degradation observed safe.
- **NOT RUNTIME VERIFIED (Windows):** toast sound/activation behavior, numeric taskbar overlay rendering, tray icon/menu interactions, and minimize/close-to-tray lock enforcement — all gated by cross-build, vet, and source-string tests only.
- **NOT RUNTIME VERIFIED:** Control Center tray card interaction (UI rendered code reviewed; not driven in a live page), recovery-code flow in the native dialogs, and live WhatsApp DOM surfaces.

## 10. Validation Record — Milestone 5

- **VERIFIED — Go tests, Linux build, Windows x64 cross-build:** every M5 commit passed `go test -count=1 ./...`, `go build`, and the Windows cross-build in the Debian Trixie container.
- **VERIFIED — Adapter fixtures:** the Node fixture suite (extended for adapter v2) covers identity key opacity/determinism, data-id vs title-fallback confidence, rename sensitivity, hash-collision ambiguity fail-closed, message/latest-message identity, and messageNodeByKey missing/ambiguous outcomes.
- **VERIFIED — Store CRUD:** pin lifecycle across controller reloads, immutable identity fields through updates, full-order reorder validation, duplicate rejection, label cascade detach, note bounds and single-anchor rules.
- **NOT RUNTIME VERIFIED:** all Control Center productivity cards against live WhatsApp Web (pin from a real chat, opening rows, bookmarking a real message), since a live logged-in session was not used. Identity keys are hash-based, so adapter changes are expected to surface as unresolved pins/bookmarks by design.

## 11. Validation Record — Milestone 6

- **VERIFIED — Go tests, Linux build, Windows x64 cross-build:** every M6 commit passed `go test -count=1 ./...`, `go build`, and the Windows cross-build in the Debian Trixie container.
- **VERIFIED — Adapter fixtures:** openStoryViewer missing/ambiguous/image/video/unknown outcomes; existing identity fixtures still pass.
- **VERIFIED — CSS validator tests:** @import, url(), expression/javascript, style breakout, unbalanced braces, oversize, and density/scale bounds are rejected; malformed saves never clobber the last-known-good CSS; disable keeps the recovery copy; corrupt stored appearance degrades to defaults while keeping the recovery point.
- **NOT RUNTIME VERIFIED:** story save against a real logged-in status viewer, appearance rules against live WhatsApp DOM, custom CSS rendering in both WebViews, and a manual keyboard/recovery pass on Windows/Linux desktops.

## 12. Release Validation Record

For every milestone report, list command or manual action, OS/version/architecture, result, and label. Record unavailable tests explicitly. A release candidate must include Windows and Linux result sets separately; no cross-platform generalization from one OS is allowed.
