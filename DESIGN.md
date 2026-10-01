# UX Design — WhatsApp Desk Personal

## 1. Product Surface

WhatsApp Web remains the main window and dominant visual surface. WhatsApp Desk controls appear as a compact, app-owned Control Center and small contextual actions; the app must not become a generic dashboard.

The Control Center is organized into GENERAL, PRIVACY, NOTIFICATIONS, APP LOCK, PRODUCTIVITY, APPEARANCE, DOWNLOADS, SHORTCUTS, and ABOUT / UPDATE. Use a compact scrollable panel/sheet that works at 450×320 and does not permanently cover chat content. Remember the selected section only as a local UI preference.

## 2. Shared Interaction Rules

- Every toggle has a plain-language label, current value, and concise explanation of the effect.
- Every action gives success/failure feedback. A failed native setting write must not be displayed as saved.
- Keyboard focus is visible; tab order follows visual order; dialogs have labels and Escape behavior that does not accidentally close a WhatsApp conversation.
- Controls are reachable when WhatsApp's header/rail changes. Provide a stable app shortcut and native tray/menu route to Settings.
- The app-owned surface remains visually distinct from WhatsApp, but uses existing WhatsApp-adjacent colors and typography.
- Privacy state, lock state, and notification policy are shown as separate concepts.

## 3. General and Profile Selector

The top of Control Center shows the active profile and a profile selector for NORMAL, OFFICE, PRESENTATION, MAXIMUM PRIVACY, and CUSTOM. A profile change previews its main effects and applies immediately when confirmed if it changes lock or visibility behavior. Show a short summary, such as “Names blurred · Generic notifications · Lock after 5 min”. Include “Edit profile” only for CUSTOM or an explicit edit action; built-in profiles can be restored to defaults.

## 4. Privacy Settings

Use surface groups, not one large list:

- **Chat list:** names, avatars, previews, timestamps, unread count.
- **Conversation:** message text, images, video, stickers, quoted/replied content, voice-note identifying details.
- **Header:** name, avatar, subtitle/status.
- **Viewer:** media viewer blur.
- **Reveal:** hover, click, or modifier key; keyboard instructions are always visible when modifier mode is chosen.

Each control can be independently toggled. Include an “Apply profile defaults” action and a temporary visual preview that never writes or sends WhatsApp data. Explain that blur is visual concealment and does not secure the account/session.

## 5. App Lock

When lock is not configured, the section explains that lock is separate from privacy blur and offers “Set PIN” or “Set password”. Show a confirmation field and generate a one-time recovery code after successful setup. The user must confirm they saved the code before lock is enabled; show it once and never store it in plaintext.

When enabled, offer manual lock, idle timeout, lock on minimize, lock on entering tray, and lock on startup independently. Changing or removing the credential requires successful current unlock. A user who forgets the credential may use the one-time recovery code to reset the app credential; this does not erase WebView session data. The recovery path is available in the native lock dialog and does not rely on the WebView page being visible. If both the credential and recovery code are lost, do not offer a silent in-app bypass; explain the local recovery limitation.

**Lock screen:** a native Windows/Linux modal lock window/dialog covers or disables the WebView and owns the credential field; no credential is entered into a WhatsApp page element or sent through a JS bridge. It shows app identity, locked status, Unlock, and a non-sensitive failure message. It must not expose chat title, message count, preview, or WebView controls. Support keyboard submit, focus, paste policy, and screen readers. After failed attempts show a wait duration; do not reveal whether a credential prefix was correct.

## 6. Notifications

Present an explicit **Preview policy** with examples:

| Policy | Example |
|---|---|
| Sender and message | “Ari: Are we still meeting?” |
| Sender only | “Message from Ari” |
| Generic | “New WhatsApp message” |
| Off | No desktop notification |

Below, show focused/background behavior, private/group rules, quiet hours, sound where supported, and locked-state behavior. Locked state defaults to Generic and is the strictest privacy floor. Explain that OS notification history is controlled by the OS and may outlive the notification banner.

## 7. Productivity

Provide separate local Pins and Bookmarks lists. Pins show a chosen local display label, reorder/remove actions, and an Open action only when the adapter can resolve the chat. Bookmarks show chat label, date/time, optional user excerpt only if the user enables it, and optional tags/categories. Provide “Remove”, “Clear all” with confirmation, and “Export local data” only if later approved in the plan; export is not part of initial scope.

If the adapter cannot identify a target reliably, show the record as “Saved, target may have changed” and offer delete; do not silently open a different chat/message.

## 8. Story/Status Download

When an image/video status is open, show a small app action “Save status”. The save dialog reuses the configured download directory and filename preview. It indicates that only the currently opened item is saved. No automatic download toggle exists. Unsupported media produces an explanatory message and no background retry/archive.

## 9. Appearance

Include compact mode, density, font/UI scale, sidebar customization, and separate toggles for hiding Channels, Communities, Status/Updates, and navigation elements. Preview changes live. Always show “Reset appearance” and keep app controls/lock surface visible even if WhatsApp selectors stop matching.

Custom CSS is an advanced, collapsed section. It has an editor, enable checkbox, validation result, Save, Reset, and Disable/Recovery action. Show unsupported syntax explicitly. Applying invalid CSS never replaces the last known-good configuration. Provide a shortcut and native tray route that can disable custom CSS even when it obscures WhatsApp content. CSS cannot reach the native lock dialog.

## 10. Tray Flows

**Windows menu:** Open WhatsApp, Privacy (submenu or toggle), Lock, Notifications (enabled/disabled and mute sound), Settings, Quit. Single-click/double-click behavior must be documented consistently. Minimize/close behavior follows settings. Reopen restores the same WebView session and window state.

**Linux menu:** Open WhatsApp, Privacy, Lock, Notifications (enabled/disabled and mute sound), Settings, Quit through StatusNotifierItem/dbusmenu. If the shell has no compatible tray watcher, opening the app and using shortcuts still work; explain that tray integration is unavailable in this session.

## 11. General, Downloads, Shortcuts, About

- **General:** startup behavior, minimize/close behavior, launch at login, window state.
- **Downloads:** existing folder chooser, monthly organization, open/reset folder.
- **Shortcuts:** searchable/listed bindings with modifier labels for current OS; avoid duplicate assignments.
- **About / Update:** WhatsApp Desk Personal identity, version, fork URL, upstream attribution/license, update source, and support/report link for the fork. Do not show upstream update actions as the fork's own releases.

## 12. Empty, Error, and Recovery States

- Empty pins/bookmarks state has a short explanation and no fabricated sample data.
- Missing DOM target does not offer an action likely to affect another chat.
- Store corruption offers safe defaults and preserves a backup for recovery.
- Notification/tray permission or daemon failure is described with a retry/settings action.
- If the settings UI fails to mount, the native tray or stable shortcut can reopen/recover it.
- If custom CSS hides page UI, native recovery can disable it; if lock is active, unlock is still required and cannot be bypassed by CSS.
