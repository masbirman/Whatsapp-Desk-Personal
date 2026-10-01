# Agent Operating Contract — WhatsApp Desk Personal

## Project Goal

Improve this personal desktop client for WhatsApp Web while keeping WhatsApp Web as the communication core. Enhancements are local-first. This is not a SaaS, CRM, bot, bulk messaging system, contact scraper, or private-protocol client.

## Supported Development Platforms

- Windows 10/11 x64 using Go, WebView2, and native Windows APIs.
- Ubuntu/Debian desktop Linux using Go, WebKitGTK, GTK/GIO/DBus.
- macOS source inherited from upstream remains in the repository and must not be deliberately broken. macOS feature parity, new features, runtime tests, builds, and packaging are outside current scope. Make only the minimum shared-code adjustment needed to keep existing source compiling or coherent.

## Architecture Constraints

- Preserve the native Go/WebView architecture. Do not rewrite to Electron, Flutter, Tauri, or another desktop framework.
- Refactor incrementally. Do not perform a big-bang rewrite or keep adding unrelated features to `main.go`.
- Move new page behavior toward modules behind a WhatsApp DOM adapter and selector registry. New WhatsApp selectors belong in that compatibility layer, not scattered feature files.
- Keep OS behavior behind small native interfaces with Windows and Linux implementations. Preserve macOS stubs/shared-code compatibility with the smallest change possible.
- Keep persistent structured application data in the native local store. Use browser storage only for transient page state that does not need schema, migration, or cross-WebView ownership.
- Avoid vendor changes. Explain and test a vendor change if no supported integration point exists.

## WhatsApp DOM Rules

- Do not use private WhatsApp APIs, scrape all contacts/chats/statuses, or bypass server-side limits.
- Prefer semantic attributes and accessible labels; add bounded fallbacks in the adapter. Avoid obfuscated CSS classes where a more stable selector exists.
- Treat DOM identity as potentially unstable. Do not persist a feature action unless the adapter returns a usable identifier and confidence/context. Keep only metadata needed to restore that action.
- Use targeted observers, added-subtree scans, debounce/throttle, visibility/scroll gates, and cleanup. Avoid full-document polling and broad observers unless justified and measured.
- Add DOM fixture tests for selector success, fallback, missing elements, and ambiguous matches. A source-string guard is not runtime validation.

## Native Bridge Rules

- Expose only the minimum functions required. Validate every argument again in Go; never trust page JavaScript to enforce a security rule.
- Use typed request/result structures for new bridge operations. Avoid arbitrary paths, shell strings, or general-purpose native dispatch.
- Do not let notification, lock, or storage bridge failures panic or crash the app. Report bounded actionable errors without message content or credentials.
- Keep UI/WebView-thread requirements explicit and dispatch calls onto the owning platform thread.

## Security Rules

- Never persist a PIN/password in plaintext. Use a salted, memory-hard password derivation for lock credentials, constant-time comparison, and bounded attempts/backoff. Treat app lock as protection from casual local access, not from a compromised OS account or disk image.
- Lock and privacy must never delete, reset, or migrate WebView cookies/session data as a side effect.
- Preserve path traversal, symlink, filename, size, and allowed-open-path validation. Do not turn user-controlled page data into shell commands or arbitrary file access.
- Notifications must apply privacy-profile and lock redaction before reaching the native OS notification API. Do not log message body, sender, PIN, or chat contents.
- Custom CSS is untrusted input. It must not override app-owned controls or lock UI; provide native/keyboard recovery and reset-to-default.
- Keep updater URLs restricted to this fork's explicitly configured release host/path. Never install upstream binaries into the personal fork automatically.
- Preserve upstream LICENSE and required attribution.

## Local-First Rules

- Pins, bookmarks, labels, notes, privacy profiles, notification rules, appearance, and lock settings live on the user's machine only.
- Do not add a backend, account system, cloud sync, analytics, telemetry, proxy, or message relay.
- Do not save full conversation history for bookmarks or local metadata.

## Testing Requirements

- Add Go unit tests for storage validation/migration, lock derivation/verification, bridge argument validation, and platform-independent policy.
- Add JavaScript/DOM fixture tests for the adapter and feature behavior.
- Use platform-specific build/static checks for Windows and Linux. Only report a platform as runtime-tested if it was actually run there.
- Maintain a manual checklist for live WhatsApp Web, notification permissions, app lock, tray, WebView session, and window lifecycle. Do not claim live DOM validation without doing it.
- Clearly label validation as VERIFIED, PARTIALLY VERIFIED, or NOT RUNTIME VERIFIED.

## Upstream Rules

- `origin` is the personal fork; `upstream` is `vianziro/Whatsapp-Dekstop`.
- Never push to upstream, force-push, or use destructive reset/rebase without the user's explicit direction.
- Keep personal enhancements separated and explain likely conflicts when a change touches upstream-owned shared code.
- Do not remove attribution/license notices.

## Definition of Done

A task is done when its acceptance criteria pass, relevant tests/checks are run and honestly reported, docs/data contracts are synchronized, no unrelated files changed, and risks or unverified behavior are stated. Do not claim completion while leaving a required gate unaddressed.

## Autonomy and Milestone Boundaries

- Follow the user's master prompt and milestone order. Do not repeat product discovery for decisions already made here.
- Complete the authorized current milestone, then stop and provide its report. Never start the next milestone without the user's instruction `lanjut`.
- Do not commit or push during Planning Package. Do not push in later milestones unless explicitly instructed.
- Ask only when there is a real blocker: missing external secret/permission, destructive data/session risk, an unresolved fundamental product choice, or a dependency that requires user action. Continue independent work while waiting.
- Preserve current context and artifacts. When asked to continue, reread the planning/status artifacts and proceed to the next unfinished milestone without requesting this prompt again.

## Reporting

End each milestone with the requested format: MILESTONE, STATUS, COMPLETED, FILES CREATED, FILES MODIFIED, TEST / VALIDATION, TECHNICAL DECISIONS, ISSUES / LIMITATIONS, TASK STATUS, NEXT MILESTONE. Then stop.
