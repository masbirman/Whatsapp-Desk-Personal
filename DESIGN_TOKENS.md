# Design Tokens — WhatsApp Desk Personal

Tokens extend the existing compact WhatsApp-adjacent visual language. Exact values are a starting point and can be tuned against screenshots on Windows/Linux; names and semantic intent should remain stable.

## 1. Color Roles

| Token | Dark default | Light default | Use |
|---|---:|---:|---|
| `surface.app` | `#111B21` | `#F7F9FA` | Control Center base |
| `surface.raised` | `#202C33` | `#FFFFFF` | Cards, fields, menus |
| `surface.overlay` | `rgba(0,0,0,.72)` | `rgba(17,27,33,.48)` | Modal backdrop |
| `text.primary` | `#E9EDEF` | `#111B21` | Main text |
| `text.secondary` | `#AEBAC1` | `#54656F` | Supporting text |
| `text.disabled` | `#667781` | `#8696A0` | Disabled controls |
| `border.default` | `#33434C` | `#D8DFE3` | Dividers and outlines |
| `accent.primary` | `#00A884` | `#008069` | Primary action/focus |
| `state.warning` | `#F0B232` | `#9A6500` | Warnings and timed lockout |
| `state.danger` | `#F15C6D` | `#B42332` | Destructive/error |
| `state.success` | `#06AE74` | `#087F5B` | Confirmed save |
| `privacy.blur` | `rgba(0,0,0,.12)` | `rgba(17,27,33,.10)` | Privacy overlay tint only |

Use text and icon/shape along with color for all states. Meet WCAG AA contrast for small text on supported themes. Do not use a color token to encode locked vs privacy-active as the sole cue.

## 2. Spacing and Sizing

Base spacing: `4px`. Scale: `space.1=4`, `space.2=8`, `space.3=12`, `space.4=16`, `space.5=20`, `space.6=24`, `space.8=32`.

- Compact row: 36–40 px; standard row: 44–48 px.
- Icon button target: at least 36×36 px; preferred touch target 40×40 px.
- Control Center width: 360–440 px, max 90vw; height no more than 90vh.
- Lock screen content width: 320–420 px; keep the window usable down to the current 450×320 minimum.
- Form controls: 36 px minimum height; use 44 px where primary/touch action.

## 3. Radius, Elevation, and Borders

- `radius.sm=4px`, `radius.md=8px`, `radius.lg=12px`, `radius.pill=999px`.
- Use 1 px border for cards/fields. Avoid nested card borders inside every settings section.
- `shadow.panel=0 10px 32px rgba(0,0,0,.28)`; `shadow.toast=0 4px 14px rgba(0,0,0,.24)`.
- Do not place app chrome shadow above the opaque lock surface.

## 4. Typography

- Native system stack: `-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif`.
- Body: 13–14 px; supporting text: 11–12 px; section title: 14–16 px, 600 weight; page title: 17–19 px, 600 weight.
- Shortcuts and technical status: 11–12 px monospace.
- Scale user-facing UI with the appearance setting; never scale credential field text below 14 px.

## 5. Focus and Interaction States

- Focus ring: 2 px `accent.primary`, offset 2 px; never remove the browser/native outline without an equivalent.
- Hover changes surface by a small contrast step, not just color hue.
- Disabled controls retain readable labels and explain why when relevant.
- Pressed state uses a clear surface change and immediate state update.
- Error state includes icon/label/text; do not rely on red border alone.
- Lock state has a persistent lock icon and “Locked” label in the app-owned layer.

## 6. Privacy Blur

- Default blur 6 px for identifying text/avatars; 8 px for message content/media preview; tune to avoid legible text at common display scales.
- Add a subtle opaque tint behind blur to prevent high-contrast image detail from remaining readable.
- Reveal transition: 100–140 ms; disable transitions under `prefers-reduced-motion`.
- Do not blur app lock credential UI. Blur is concealment, not cryptographic protection.

## 7. Motion

- Small state transition: 120–160 ms ease-out.
- Panel enter/exit: 160–200 ms, opacity plus no more than 6 px translation.
- Avoid animated polling or indefinite spinners; use progress only for operations with measurable progress.
- Respect reduced motion and avoid animation when hidden/backgrounded.

## 8. Z-Index Layers

Define semantic layers: WhatsApp page < app toast < Control Center < modal backdrop < modal panel < app lock. Use a small centralized scale rather than unrelated maximum integers. The lock layer must be app-owned and above page CSS; custom CSS cannot set or override its layer.

## 9. Token Governance

Store app UI tokens separately from user custom CSS. Apply the same semantic tokens in settings, lock screen, toast, and tray-related dialogs. Validate dark and light modes, focused/unfocused, disabled/error, minimum window, and 125–200% UI scale on Windows and Linux.
