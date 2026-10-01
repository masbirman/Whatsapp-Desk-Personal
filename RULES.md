# Engineering Rules

These rules translate the project contract into review checks. `AGENTS.md` defines the operating contract; this file is the concise implementation checklist.

## Scope and Code Shape

- Target Windows and Ubuntu/Debian Linux only. Keep existing macOS code coherent with minimum shared changes; do not implement or validate new macOS features.
- Preserve Go + native WebView. Keep work incremental and focused. Do not put another large feature into `main.go` or make broad formatting/refactor changes alongside feature work.
- Prefer small standard-library solutions and existing platform APIs. Add a dependency only when it materially improves correctness/security; document license, maintenance, cross-build, and vendor impact first.
- Keep upstream license, copyright, and attribution intact. Do not edit generated/vendor code unless no supported integration point exists.

## JavaScript, CSS, and DOM

- All WhatsApp-specific selectors go into the DOM adapter/registry. Feature modules consume semantic adapter methods.
- Prefer roles, `data-testid`, accessible labels, and visible semantic relationships. Obfuscated class names require a documented reason and fallback.
- All observer scans are targeted, coalesced, visibility-aware, and measured against a fixture. Register cleanup for observers/listeners/timers with module lifecycle.
- Do not scan full chat history or hidden virtualized data. Do not use private WhatsApp protocol/API.
- App-owned controls must not be selectable/overridden by custom CSS. The Windows/Linux lock prompt is native UI, never a DOM credential form. Reject remote `@import`/URLs and unsafe selectors; maintain last-known-good CSS and native recovery.
- Do not interpolate chat content, filenames, status titles, or release metadata into HTML without context-appropriate escaping. Prefer `textContent`.

## Go, Storage, and Bridge

- Keep cross-platform policy independent of OS; OS calls stay in platform-specific files or narrow drivers.
- Validate native bridge payloads in Go. Use enums/typed structs, bounded string lengths, safe identifiers, and explicit results.
- Persistent records use the versioned contract in `DATA_CONTRACT.md`, atomic writes, backup/recovery, validation, and migration tests.
- Keep browser session/profile paths unchanged. No lock/privacy/profile transition may clear website data.
- Keep local data local; never add account, sync, analytics, or telemetry calls.
- File operations reuse validated saver/open path rules. Never construct shell command strings from user/page data.

## Security and Privacy

- No raw lock credential in memory longer than needed, logs, page UI state, bridge result, crash report, or disk. Collect/verify it in native Windows/Linux UI, not in WhatsApp's page. Zero temporary byte buffers where practical.
- Use Argon2id with a random per-credential salt and calibrated bounded parameters; constant-time verifier comparison; rate-limit failed app attempts. Explain the local-user threat model.
- Redact sender/body before invoking native notification APIs. Locked-state generic notification is a mandatory privacy floor.
- URL validation uses HTTPS and exact host/repository or dot-delimited subdomain matching. Updater accepts only personal-fork releases.
- Error messages must not include message body, phone number, PIN, or sensitive path unless required for a local diagnostic and explicitly scrubbed.

## Tests and Review

- Tests should assert behavior/policy, not merely repeat implementation text.
- Add fixtures for DOM selector fallback/ambiguity and regressions. Run Go tests, JavaScript harness, relevant Windows cross-build/static check, and Linux build/runtime validation appropriate to the change.
- State what was actually verified. Source inspection, unit tests, builds, OS runtime, and live WhatsApp DOM are separate levels.
- Check working tree to ensure only intended files changed. Update PRD/architecture/data/security/testing docs when behavior/contracts change.
- User-visible changes get concise release-note entries under `CHANGELOG.md` Unreleased unless the change is docs-only.

## Upstream and Milestones

- `origin` is the personal fork; `upstream` is the source project. Never push upstream or force-push.
- Identify expected upstream conflict areas for shared files and avoid unnecessary divergence.
- Complete one authorized milestone, report it in the required format, then stop. Do not begin the next milestone until the user says `lanjut`.
