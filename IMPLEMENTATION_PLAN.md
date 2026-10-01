# Implementation Plan — WhatsApp Desk Personal

This plan follows the user's milestone checkpoints. Dependencies below are logical gates; finishing a milestone does not authorize the next one. Each milestone ends with tests/validation, a report, and a stop for user instruction.

## 1. Dependency Overview

```text
M1 Planning Package
  ↓ user says "lanjut"
M2 Foundation: characterization → DOM adapter → typed bridge → versioned store/migration
  ↓ user says "lanjut"
M3 Privacy policy/profiles → native app lock → lock-aware state persistence
  ↓ user says "lanjut"
M4 Notification policy → Windows/Linux notification drivers → taskbar unread → tray
  ↓ user says "lanjut"
M5 local chat identity from adapter → pins → bookmark/labels/notes
  ↓ user says "lanjut"
M6 appearance foundations → status viewer/download → custom CSS/recovery
  ↓ user says "lanjut"
M7 fork identity/updater → security/performance/regression → release readiness
```

Storage, adapter, typed bridge, and security policies are prerequisites for features relying on them. Notification redaction depends on a single effective privacy/lock/profile state, so it follows Milestone 3. Local productivity waits for adapter identity confidence and versioned storage. Story download waits for viewer/media adapter and validated saver integration. No feature milestone may silently roll into the next.

## 2. Milestone 1 — Planning Package

Create and cross-check project requirements, architecture, UX, data contract, security, rules, execution ledger, and testing design. Do not modify application source, install dependencies, build, run app, or implement features. Current status is reported separately by the milestone report.

## 3. Milestone 2 — Foundation and Architecture

### Sequence

1. Add behavior characterization for the existing injected script and platform bridge contracts without changing behavior.
2. Extract/integrate a DOM compatibility adapter and selector registry. Move existing selectors behind adapter methods progressively; preserve current visible behavior.
3. Add adapter fixture tests (preferred selectors, fallbacks, ambiguous/missing nodes, locale labels) and observer lifecycle/performance guards.
4. Define typed native bridge DTOs and validation. Keep current platform behavior while replacing ad hoc parameters on touched paths.
5. Introduce versioned native JSON storage, atomic write/backup/recovery, and idempotent migration from current settings; preserve WebView profiles and window state.
6. Add shared app controller/state owner for profile/privacy/lock/notification policy foundations; do not ship a feature UI or continue into M3.

### Exit gates

- Existing settings values migrate correctly and repeat migration is harmless.
- No WebView session/profile path changes.
- Tests cover adapter outcomes, bridge invalid input, storage corruption/migration, and baseline behavior.
- Linux build/test passes in available environment; Windows cross-build/static checks pass if toolchain supports them. Runtime scope is labeled honestly.

## 4. Milestone 3 — Privacy and App Lock

### Sequence

1. Implement the granular privacy policy model and profile definitions/defaults in native store.
2. Implement adapter-backed per-surface blur selectors, hover/click/modifier reveal and keyboard accessible controls.
3. Implement profile selector and profile-local notification/appearance privacy rules.
4. Implement salted Argon2id credential and recovery-code verifiers, setup/change/recovery, bounded KDF params and attempt backoff in Go.
5. Implement native-owned lock state machine and Windows/Linux modal lock prompt with native credential input; cover/disable WebView, and add manual/shortcut/idle/minimize/tray/startup triggers where available.
6. Ensure unlock requires native verification; ensure lock/profile transitions do not touch WebView data; add notification privacy floor contract for M4.

### Exit gates

- Every PRD privacy surface can be toggled independently and has adapter fixture coverage.
- Profiles persist, switch, and reset; lock cannot be bypassed by JS events, page bridge credentials, custom CSS, or a weaker profile.
- Credential and recovery code are never persisted/logged in plaintext; recovery reset does not log out WhatsApp.
- Tests cover state transitions, KDF, backoff, profile migration, lock recovery, and privacy selector fallbacks.

## 5. Milestone 4 — Notifications and Desktop Integration

### Sequence

1. Normalize intercepted WebView notification events into a local event DTO without persisting content.
2. Apply one policy for enabled, sender/body/generic, focused/background, private/group, quiet hours, profile, sound and locked state. Redact before native dispatch.
3. Implement Windows toast behavior/activation with lock/profile redaction and native window focus.
4. Implement Ubuntu/Debian desktop notification driver with graceful failure and safe retry.
5. Improve unread signal through the adapter; test title-based fallback. Add numeric taskbar overlay if the existing taskbar API is reliable, else native fallback.
6. Add Windows tray and menu actions, tray lifecycle and minimize/close behavior.
7. Extend Linux StatusNotifierItem with dbusmenu-compatible actions and graceful no-watcher behavior.
8. Wire settings and shortcuts; validate startup state accuracy and lock-on-tray integration.

### Exit gates

- Hidden sender/body never reaches OS API in any lock/profile/focus case.
- Tray actions work or degrade with clear unavailable state; no tray failure crashes the app.
- Unread does not use private WhatsApp APIs and updates/clears from supported DOM signal.
- Windows and Linux targeted build/static/runtime checks and notification/tray matrix are reported separately.

## 6. Milestone 5 — Local Productivity

### Sequence

1. Finalize adapter chat/message identity provenance, confidence, unresolved behavior and migration.
2. Implement persistent unlimited local pins, ordering, alias/display label, open when resolvable, remove/reorder.
3. Implement bookmark add/remove/list, minimal message metadata and no excerpt by default.
4. Implement labels/categories and personal notes over local records.
5. Implement local filtering of pins/bookmarks/labels; do not filter server-side WhatsApp data.
6. Add clear/delete flows, corruption/recovery and privacy disclosure in the UI.

### Exit gates

- Local data survives restart, has no app-imposed pin count cap, and is never synced.
- Missing/ambiguous adapter identity is visible and cannot open a wrong chat/message.
- No full conversation or message body is copied by default; clear individual/all tests pass.

## 7. Milestone 6 — Story and Appearance

### Sequence

1. Extend adapter to detect only currently opened status/story viewer and visible media.
2. Add explicit Save/Save As action for image/video and route through validated existing saver.
3. Implement appearance layout settings: compact/density, scale, sidebar, hide section/navigation options with reset. Lock prompt remains native and outside the CSS surface.
4. Implement custom CSS storage, parser/scope/validation, last-known-good restore, native/keyboard recovery, app-control isolation.
5. Update Control Center organization and responsive/accessibility states.

### Exit gates

- No automatic status discovery/download/archive exists.
- Download uses current path/filename protections and fails safely if viewer identity is missing.
- CSS cannot reach app-owned controls/lock UI, remote CSS is rejected, and malformed CSS cannot replace last-known-good.
- Appearance reset/recovery works at minimum window size and across supported WebView fixtures.

## 8. Milestone 7 — Hardening and Release Readiness

### Sequence

1. Centralize fork identity. Update updater, release metadata, issue reporter, About, installer, Linux desktop entry, and Windows app identity to the personal fork; preserve attribution.
2. Require and verify fork release checksums; test redirects, wrong owner/path, missing/mismatch checksum, asset bounds, and platform architecture.
3. Complete security review for bridge, paths/symlinks, lock, notification redaction, custom CSS, URLs, and local records.
4. Run regression/unit/DOM tests; Linux Ubuntu/Debian runtime checklist; Windows build/static and runtime only if Windows is available.
5. Review performance: observer scope, scan budgets, payload copies, quiet background, and logging defaults.
6. Validate installers/package names/startup identities and reconcile all docs/changelog with actual behavior.
7. Document remaining limitations, unsupported DOM behaviors, and known OS-shell constraints.

### Exit gates

- No update path can install upstream binary into this fork.
- Required tests/build/package checks pass or have a precise blocker; runtime claims match actual target testing.
- No new critical/high security issue remains unresolved; known risk accepted/documented.
- User receives the milestone report and the workflow stops.

## 9. Cross-Milestone Rules

- Any data contract change updates `DATA_CONTRACT.md` and migration tests in the same milestone.
- Any selector behavior change updates adapter fixtures and `ARCHITECTURE.md`/`TESTING.md` as appropriate.
- Any user-visible behavior change updates `PRD.md`, `DESIGN.md`, and release docs only when the behavior actually changes.
- Test before expanding the scope. Do not run macOS-specific build/runtime/package work.
- No milestone automatically creates a commit or pushes; pushes require explicit user instruction.
