# Frontend design system — "warm graphite & amber"

Replaces the three-times-patched "Midnight soundroom" theme in `src/App.css`
(a light theme nobody sees, overridden by a dark-theme `:root`, overridden
again by a "contrast pass" of hardcoded hex values — three layers fighting
over the same colors). This doc is the single source of truth for the new
tokens; implement by rewriting `src/App.css` and the `.ui-*` classes it
defines, not by patching on top of what's there.

Visual reference (mockup only, not shippable code):
https://claude.ai/code/artifact/82e3aad7-c72a-4dc5-b5b8-10ea461c2be8

The `components/ui/` API (`Button`, `IconButton`, `TextField`, `SelectField`,
`Notice`, `Spinner`) does not change — same props, same exported names.
Only the CSS backing them changes.

## Tokens

Add fonts once, top of `App.css`:

```css
@import url('https://fonts.googleapis.com/css2?family=Geologica:wght@600;700;800&family=Inter:wght@400;500;600;700&display=swap');
```

Replace the `:root` block (and delete the two override passes below it)
with:

```css
:root {
  --night: #14120f;
  --canvas: #1a1815;
  --paper: #221f1a;
  --paper-raised: #2a2620;
  --ink: #f5f1ea;
  --muted: #a39a8c;
  --line: #35312a;

  --accent: #e2973c;
  --accent-strong: #c97a1f;
  --accent-soft: #3a2c17;
  --accent-ink: #241804;

  --danger: #e2574a;
  --danger-soft: #341f1c;

  --radius-sm: 10px;
  --radius-md: 16px;
  --radius-lg: 24px;
  --radius-pill: 999px;

  --shadow-sm: 0 2px 8px rgb(0 0 0 / 30%);
  --shadow-md: 0 12px 28px rgb(0 0 0 / 35%);
  --shadow-lg: 0 28px 60px rgb(0 0 0 / 45%);
  --shadow-accent: 0 10px 24px rgb(226 151 60 / 30%);

  font-family: Inter, "Segoe UI", sans-serif;
}
body { background: var(--night); }
```

There is no light theme and no theme toggle anywhere in the app — don't
carry the old light `:root` forward "just in case."

## Type scale

| Token | Font | Weight | Size | Letter-spacing | Used for |
|---|---|---|---|---|---|
| display-xl | Geologica | 800 | 32px | -0.03em | empty-chat headline, auth `h1` |
| display-lg | Geologica | 700 | 22px | -0.02em | chat header `h1`, call `h2` |
| label-md | Inter | 700 | 15px | — | identity name, conversation name |
| body-md | Inter | 400 | 14px | — | message text, base body copy |
| body-sm | Inter | 500 | 13px | — | button labels, meta text |
| eyebrow | Inter | 600 | 11px | 0.08em, uppercase | section titles ("Диалоги") |

## Components

**Button** (`.ui-button`) — pill (`--radius-pill`), `min-height: 42px`,
`padding: 10px 20px`, `font: 700 13px/1.15 Inter`.
- `primary`: bg `--accent`, text `--accent-ink`, shadow `--shadow-accent`;
  hover bg lightens toward `#f0aa54`.
- `secondary`: bg `--paper-raised`, 1px border `#3a3630`, text `--ink`;
  hover border `#4a453c`, bg `#332e26`.
- `ghost`: no bg/border, text `--muted`; hover text `--accent`.
- `danger`: bg `--danger`, text `#fff`, shadow `0 8px 18px rgb(226 87 74 / 25%)`.
- `disabled`: `opacity: 0.4`, `cursor: not-allowed` on all variants (not the
  old `0.55` — the new palette needs the extra contrast drop to read as
  disabled against the darker surfaces).

**IconButton** (`.ui-icon-button`) — 36px circle, transparent by default,
`--paper-raised` on hover, icon color `--ink` / `--muted`.

**TextField / SelectField** (`.ui-field`) — `min-height: 44px`,
`border-radius: var(--radius-sm)`, border `#3a3630`, bg `#151310`, text
`--ink`, placeholder `#756e5f`. Focus: border `--accent`, bg lightens one
step (`#1c1912`), `box-shadow: 0 0 0 4px rgb(226 151 60 / 16%)`.

**Notice** (`.ui-notice`) — `border-radius: 12px`, border
`rgb(226 87 74 / 35%)`, bg `--danger-soft`, text `#f4a89f`.

**Spinner** (`.ui-spinner`) — 22px ring, border `#3a3630`, top border
`--accent`, same `spin` keyframe as before.

## Screens

**AuthScreen** (`.auth-page` / `.auth-card`) — background `#100e0b` with two
blurred radial blobs (amber `#e2973c` top-left, copper `#8a5a2c`
bottom-right, `filter: blur(90-100px)`, opacity ~0.24-0.28) — same
composition as before, new colors. Card: `rgb(24 21 17 / 72%)` +
`backdrop-filter: blur(20px)`, `border-radius: var(--radius-lg)`,
`box-shadow: var(--shadow-lg)`. Fields on the card get a translucent
white treatment (`background: rgb(255 255 255 / 6%)`, `border: 1px solid
rgb(255 255 255 / 14%)`) rather than the solid field style — same rule as
the old theme, don't lose it in the rewrite. Brand mark: 30px rounded
square, bg `--accent`, text `--accent-ink`, the letter "p" in Georgia
italic (unchanged from `Brand.tsx`).

**Sidebar** (`.sidebar`, 328px) — background
`linear-gradient(180deg, #100e0b 0%, #161310 68%, #0c0a08 100%)`, right
border `--line`. Identity card: translucent white panel, avatar circle bg
`--paper-raised` text `#e6b578`. Active conversation item:
`background: linear-gradient(100deg, var(--accent-strong), var(--accent))`,
text `--accent-ink` (not white — the amber gradient is light enough that
dark text reads better than the old white-on-violet treatment). Unread
badge: bg `--accent`, text `--accent-ink`.

**Chat panel** — header background `rgb(26 24 21 / 88%)` +
`backdrop-filter: blur(16px)`. Message bubbles: incoming
`border: 1px solid var(--line); background: var(--paper);` radius
`6px 18px 18px 18px`; own messages
`background: linear-gradient(135deg, var(--accent-strong), var(--accent));
color: var(--accent-ink);` radius `18px 6px 18px 18px` (tail corner
flipped, same asymmetric-bubble idea as before). Realtime-status dot:
`background: var(--accent)` when live, `#4a453c` when not, with the halo
`box-shadow: 0 0 0 4px rgb(226 151 60 / 16%)` only in the live state.

**CallOverlay** — background
`radial-gradient(circle at 75% 20%, #3a2a14 0%, #191510 39%, #0c0a08 100%)`.
Participant cards: `border-radius: var(--radius-lg)`, bg
`rgb(10 8 6 / 38%)`. Two behavior changes from the current implementation,
confirmed against the mockup — **implement both**:

1. **A participant sharing their screen shows two tiles, not one.**
   `CallOverlay.tsx`'s `ParticipantSection` already renders one
   `VideoTile` per stream and lays them out with
   `.call-participant__media { grid-template-columns:
   repeat(auto-fit, minmax(190px,1fr)) }` — that part is already correct.
   What's missing is styling each tile so two sit side by side at roughly
   equal width when a participant has both a camera and a screen-share
   stream, instead of stacking full-width. Keep the existing
   `auto-fit`/`minmax` rule; just make sure the tile's own `min-height`
   (~175px when there are 2, ~220px when there's 1) doesn't force a
   single column.

2. **Every video tile needs a maximize/minimize control** — this is new,
   not in the current `CallOverlay.tsx` at all. Each `.call-video` figure
   gets a small square icon-button overlaid top-right (`position:
   absolute; top: 10px; right: 10px`, 26-28px, `border-radius: 8px`,
   `background: rgb(10 8 6 / 58%)`) with an expand icon (four corner
   brackets pointing outward — see the mockup's inline SVG,
   `M9 3H3v6M15 3h6v6M3 15v6h6M21 15v6h-6`). Clicking it should focus that
   one stream: the grid collapses to a single large tile (the clicked
   one) plus a thin filmstrip row of ~110px-tall thumbnails for every
   other stream below it, each thumbnail keeping its own small maximize
   button so focus can move again. The maximized tile's button becomes a
   minimize icon (corners pointing inward,
   `M9 3v6H3M15 3v6h6M3 15h6v6M21 15h-6v6`) that restores the full grid.
   This needs a small piece of local state in `CallOverlay.tsx` — e.g. a
   `focusedStreamID: string | null` — and two layout branches in the JSX;
   no backend or `useCall`/`useLiveCall` hook changes are needed, it's
   purely how the existing streams are arranged and which one renders
   large. See the mockup's `CallMaximized` artboard for the exact
   proportions (one flexed figure filling the available height, a
   110px-tall row of equal-width thumbnails below it).

**IncomingCallDialog** — same composition, new colors: card
`linear-gradient(145deg, #332616, #161310)`, pulse circle bg `--accent`
with an outward `box-shadow` ring animation, icon is an inline phone SVG
(not the `⌁` glyph currently in the code — replace it, dingbats don't
belong in a redesigned icon set).

## What this doc is not

It's a token/behavior spec, not a diff. Whoever implements this still
needs to read the current `App.css` and `components/ui/*.tsx` /
`components/*.tsx` to know exactly which selectors exist and rewrite them
in place — the class names (`ui-button`, `message--mine`,
`conversation-item--active`, `call-video`, etc.) don't change, only the
declarations inside them.
