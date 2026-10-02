# Changelog

All notable changes to WhatsApp Desk Personal are documented here.
Format follows Keep a Changelog; versions are `MAJOR.MINOR.PATCH[.BUILD]`.

## [1.5.9.9] - 2026-10-02

> First personal-fork release: local-first privacy, app lock, desktop
> integration, productivity, and appearance tooling on the upstream 1.5.9.8
> base. Upstream attribution and license are preserved.

### Added
- Privacy profiles (NORMAL / OFFICE / PRESENTATION / MAXIMUM PRIVACY / CUSTOM)
  with adapter-backed per-surface blur and hover/click/modifier reveal.
- App lock: Argon2id credentials, one-time recovery code, native Windows/GTK
  prompt, startup/idle/minimize/tray triggers (Ctrl+Shift+L; fail-closed).
- Notification policy owned in Go: locked floor (generic-or-suppress), quiet
  hours, focus behavior, sender/body redaction before the OS driver; Windows
  toast with policy-driven sound; Linux notify-send driver with safe failure.
- System tray: Windows tray with menu and minimize/close-to-tray (settings,
  default off); Linux StatusNotifierItem with a full dbusmenu (Open, privacy,
  lock, notifications toggle, Control Center, Quit).
- Numeric unread taskbar overlay on Windows (progress-bar fallback documented).
- Local productivity: chat pins (alias, reorder, open via adapter identity),
  message bookmarks (metadata only, no bodies), labels, notes, and a shared
  local filter in the Control Center.
- Story support: detect only the currently open status viewer and save its
  media manually through the validated download path — no discovery/archiving.
- Appearance: compact chat list, density, 80-130% scale, hide unread badges /
  archived row; custom CSS with Go-side validation, last-known-good recovery,
  and Ctrl+Shift+X instant disable.
- Versioned local store (`app_store.json`) with atomic writes, backups,
  recovery, and idempotent migration from `settings.json`.

### Changed
- Updater, release scripts, README, and issue reporting point exclusively at
  this fork; upstream release URLs are rejected by the update allow-list.
- Updates require SHA256SUMS; a digest mismatch is always fatal and the sums
  are always fetched from the same release as the artifact.
- WebKitGTK 4.1 build variant supported on Linux (Ubuntu 24.04+).

### Security
- Typed native bridge with Go-side validation for every operation; file opens
  jailed to download/preview directories with symlink-aware checks.
- Full surface review with accepted residual risks documented in SECURITY.md.

### Notes for this fork
- Windows behavior is build/vet verified only (no Windows runtime session yet).
- Live WhatsApp DOM validation is performed through daily use; adapter
  selectors are centralized in `dom_adapter.js` with fixtures.
