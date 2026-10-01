# Security Policy

## Supported Versions

Security updates are currently provided for the latest version available on the
`main` branch.

| Version or Branch | Supported |
| ----------------- | --------- |
| `main`            | ✅ Yes     |
| Older releases    | ❌ No      |

Older versions may contain known security issues and are not guaranteed to
receive security updates.

## Reporting a Vulnerability

Please do not report security vulnerabilities through public GitHub issues.

If GitHub Private Vulnerability Reporting is enabled for this repository, please
use that feature to submit your report.

If private reporting is not available, contact the project maintainer privately
through the maintainer's GitHub profile before disclosing any vulnerability
details publicly.

Please include:

- A clear description of the vulnerability.
- The affected version, branch, or commit.
- Steps to reproduce the issue.
- The potential impact.
- Proof-of-concept code or screenshots, if applicable.
- Any suggested mitigation or fix.

Please remove or redact passwords, tokens, phone numbers, chat content, personal
data, and other sensitive information before submitting a report.

## Disclosure Policy

Please allow the maintainers reasonable time to investigate and address the
issue before making vulnerability details public.

The maintainers may:

- Acknowledge the report.
- Request additional information.
- Confirm or reject the vulnerability.
- Prepare and release a fix.
- Credit the reporter if they agree.

Please do not publicly disclose the vulnerability before coordinating with the
maintainers.

## Scope

This policy covers security issues in the WhatsApp Desktop application and its
source code.

Issues caused by WhatsApp's servers, WhatsApp accounts, operating systems, or
third-party dependencies may need to be reported to the relevant vendor.

## WhatsApp Desk Personal Development Security Baseline

## 1. Assets and Threat Model

Assets include WhatsApp Web session data, visible chat/media content, local PIN verifier, local metadata (pins/bookmarks/notes), downloaded files, notification previews, updater binaries, and native bridge capabilities.

The app lock is intended to protect against casual access to an unattended app window. It does not protect against a compromised OS account, administrator, malware running as the same user, screen capture before lock, physical memory inspection, or a copy of the user's profile. Local pins/bookmarks/notes may contain sensitive identifying metadata and should be treated as private.

## 2. Lock Credential

- Store only algorithm/version, random salt, bounded KDF parameters, and derived verifier. Never store the PIN/password itself.
- M3 uses `golang.org/x/crypto/argon2` v0.57.0. The configured baseline is 19 MiB memory, 2 iterations, and one lane. Hard bounds are 19–256 MiB, 2–10 iterations, and 1–4 lanes. The cost has not yet been benchmarked on minimum supported Windows/Linux hardware; do not silently lower it on slow systems.
- Generate at least a 16-byte cryptographically random salt. Use a 32-byte derived verifier and constant-time comparison.
- Accept a 6+ digit numeric PIN or a passphrase of at least 8 characters; recommend a longer passphrase. Clearly tell the user that a short PIN has limited entropy and that the local KDF raises offline guess cost but does not make a copied profile immune to guessing.
- Apply in-process exponential delay after repeated failures, cap the delay, and do not permanently brick the profile based on a mutable local JSON counter. Never expose whether part of a credential matched.
- Generate a one-time random recovery code during setup. Display it once and require the user to confirm it was saved. Store only a separate salted Argon2id verifier; recovery code resets the app credential but never changes WebView data. If both secrets are lost, do not add a silent in-app bypass.
- Keep credential bytes ephemeral; avoid storing them in JS localStorage, native bridge return values, crash logs, or notification payloads.
- Normal credential recovery or disable changes only lock credentials/configuration and preserves WebView session/cookies and local productivity records. If the app store itself is corrupt, the separate native-confirmed recovery action resets the app store to defaults; it still preserves WebView session/cookies.

OWASP's current password storage guidance recommends Argon2id and lists a baseline of 19 MiB, two iterations, one degree of parallelism: [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). Go's package documentation recommends `argon2.IDKey` for Argon2id: [Go x/crypto/argon2](https://pkg.go.dev/golang.org/x/crypto/argon2). Final parameter choice must be measured on supported hardware.

## 3. Lock State and Session Safety

- Lock policy is owned in native Go state; page activity cannot unlock the app.
- A native Windows/Linux modal lock window/dialog blocks the main window and covers the WebView until the native verifier approves the credential. The lock credential is never collected or verified in page JavaScript, and no page bridge method accepts it.
- Notify policy observes lock state before native delivery. On lock, purge queued unredacted notification payloads and use generic text. (Implemented in M4-01: a single Go resolution point applies the master switch, locked floor, quiet hours, focus behavior, and sender/body redaction before the OS driver; the stored-policy corruption path degrades to content-free presentation.)
- Lock/unlock, startup lock, minimize-to-tray, and profile switches never clear WebView cookies, localStorage, IndexedDB, or website data.
- Keep a native recovery route if WebView DOM/CSS fails. Do not provide an unlock bypass through tray, debug bridge, custom CSS, or app reopen.

M3 implementation status: credential setup, verification, recovery, profile policy, and native Windows/GTK prompt code are present. Idle, minimize, startup, manual, and keyboard lock paths are implemented. Ubuntu runtime verification (isolated profile, AT-SPI-driven) passed for: startup lock before WebView navigation, wrong-credential failure dialog, correct-credential unlock (two cycles), fail-closed re-prompt on cancel/close, 12-second idle re-lock, and WebView session reconnection after unlock. Tray-trigger wiring is pending the tray integration milestone. Windows native dialog behavior, minimize trigger, page shortcut, and recovery-code unlock have not yet been manually verified. Lock-aware notification redaction is a later milestone and is not implemented yet.

## 4. Native Bridge Boundary

- The page runs on `web.whatsapp.com` and can reach registered bridge methods. Treat all values crossing from page JS as untrusted.
- Keep the bridge API small and explicit; validate type, length, enum, state transition, identifier provenance, URL and path in Go.
- Do not expose arbitrary path reads, command execution, process launching, generic eval, lock verifier export, or raw settings-file access.
- File open remains jailed to the download and preview directories. New story/file flows reuse existing path canonicalization, symlink handling, basename sanitization, and payload limits.
- External links use HTTPS/HTTP as intended and exact host or dot-boundary subdomain checks. Never use a bare suffix check that accepts `evilwhatsapp.com`.
- Native handlers that touch GTK/AppKit/WebView2 run on the proper thread; callbacks fail safely rather than panic.

## 5. Notifications and Local Data

- Apply one native notification policy before platform code: app focus, private/group rule, active profile, quiet hours, and lock state.
- Locked state forces generic title/body regardless of per-profile preferences. Hidden sender/body fields must not enter logs or native event metadata.
- WebView notification permission spoofing in the current script is a compatibility shim, not proof that every WhatsApp notification event is captured.
- Store minimum local metadata. Do not save full conversations or message bodies by default. Message excerpts are opt-in and visibly disclosed.
- Use current-user config directory and restrictive file permissions where supported. Atomic replace and a previous valid backup prevent partial writes. Validate imported/recovered data before use.
- A corrupted store must not disable lock protections or silently downgrade the KDF; if lock metadata cannot be verified, keep the app locked and offer explicit credential reset while preserving session data.

## 6. Custom CSS

Custom CSS is user-authored page code in the sense that it can alter displayed UI and is untrusted input. It must:

- remain local and optional;
- have no JavaScript execution, remote import, remote URL loading, or arbitrary native bridge access;
- be restricted to WhatsApp content and not reach app-owned controls or lock surface;
- be parsed/validated before replacing last-known-good CSS;
- provide reset/disable from native tray/shortcut when WhatsApp content is hidden.

Do not claim regex blocking alone provides a complete CSS security boundary. Choose a parser/scoping design during implementation and test selector escapes, nested at-rules, and malformed CSS.

## 7. Updater and Fork Identity

- The audited code uses `vianziro/Whatsapp-Dekstop` for release checks and permitted downloads. That is unsafe for a fork if it can overwrite the fork binary with an upstream version.
- Before enabling updates in the personal fork, centralize release identity as `masbirman/Whatsapp-Desk-Personal`, validate HTTPS, exact repository owner/name, release path, asset platform/architecture and size, and constrain redirects.
- Verify a checksum from the same release. Plan for required checksums for all newly published fork assets; legacy missing-checksum exceptions must be a deliberate compatibility policy, not accidental acceptance.
- Keep issue/report links, About, Windows product identity, Linux desktop file, installer and release metadata aligned with fork identity. Preserve upstream attribution and license.

## 8. Logging and Diagnostics

- Continue local-only, opt-in diagnostics. Redact home paths, usernames, phone numbers, chat names, message text, media URLs, and credential data.
- Bound log length/rotation. Never log notification body or bridge credential payloads.
- A report preview must show all included diagnostic text before the user sends it. No automatic upload or telemetry.

## 9. Security Test Gates

- Lock: KDF salt uniqueness, correct/incorrect verify, constant-time comparison path, parameters bounds, attempt delay, recovery verifier/reset, persistence corruption, reset preserves profile/session.
- Storage: traversal, absolute path, symlink and dangling symlink, malformed JSON, partial write, backup restoration, schema migration, oversized record.
- Bridge: invalid enum, oversized strings, malformed identifiers, arbitrary path, invalid URL, repeated calls while locked.
- Notification: sender/body redaction across profile, lock, focus, group/private, quiet hours, OS failure; assert hidden text never reaches native driver/log.
- CSS: `@import`, `url`, global selector, app control/lock selectors, malformed/nested at-rule, last-known-good recovery.
- Updater: upstream repository URL rejected; fork URL accepted; malformed owner/path, HTTP, redirect outside allowed policy, checksum missing/mismatch, oversized asset rejected.

Security reporting for public release follows this repository's private vulnerability handling and keeps attribution intact.
