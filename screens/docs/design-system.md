# Prepeet design system — summary

The live, interactive version of this document is [`design-system.html`](../design-system.html).
This file is the written summary: the decisions and the rules, not the swatches.

---

## 1. Design intent

Prepeet is used by three audiences with very different emotional states, in the same week:

| Audience | State of mind | What the design owes them |
| --- | --- | --- |
| Candidate | Often anxious, frequently on a phone, sometimes at 06:00 before a shift | Calm, uncluttered, one obvious next action, no gamified scoring theatre |
| Recruiter | Making a consequential judgement about a person | Evidence first, uncertainty made visible, no false precision, everything traceable |
| Platform administrator | On call, scanning for the thing that is wrong | Density, precision, status that reads at a glance, no decoration |

The identity that satisfies all three is **warm, editorial and quiet**: a deep teal-green primary
rather than the default SaaS blue or violet, a warm grey neutral instead of a cold slate, a serif
display face for human moments and a clean grotesque for interface text. Colour is used sparingly and
almost always to carry meaning; large areas of flat surface are deliberate.

Explicitly avoided: gradient meshes on every surface, glassmorphism, neon, dark-mode-only "AI"
aesthetics, progress-bar confetti, badge/streak gamification of hiring outcomes.

---

## 2. Colour

### Brand ramps (`tokens.css`)

| Ramp | Role | Anchor value |
| --- | --- | --- |
| **Reef** (teal-green) | Primary. Actions, active navigation, the candidate's own signal. | `--reef-600 #177068` light · `--reef-400 #43A797` dark |
| **Ember** (amber) | Accent and warning. Attention without alarm. | `--ember-500 #D98E04` |
| **Plum** | Screen mode, AI-speaking states, the second categorical series. | `--plum-500 #7C4D9E` |
| **Coral** | Danger, destructive actions, contradictions. Chosen over pure red so error states are readable rather than shouty. | `--coral-500 #D9634C` |
| **Moss** | Success and supporting evidence. | `--moss-500 #2F8F4E` |
| **Sky** | Information. | `--sky-500 #3A7BBF` |
| **Stone** | Warm neutral, 0 → 950. Every surface, border and text colour. | `--stone-500 #736E66` |

### Semantic tokens

Pages never reference a brand ramp directly. They use semantic tokens that are redefined per theme:

`--bg --bg-subtle --surface --surface-2 --surface-3 --surface-inset --overlay`
`--border --border-strong --border-subtle`
`--fg --fg-2 --fg-3 --fg-muted --fg-inverse`
`--primary --primary-hover --primary-active --primary-fg --primary-soft --primary-soft-fg --primary-border`
`--success --warning --danger --info --neutral-soft` (each with `-soft`, `-fg`, `-border` variants)
`--focus-color --shadow-xs|sm|md|lg`
`--sidebar-*  --skeleton*  --waveform-*`

This is why adding a theme is a matter of redefining one block, and why no page contains a hard-coded
hex value for anything meaningful.

### Chart palette

Seven categorical colours in a fixed order (`--chart-1 … --chart-7`): reef, plum, ember, sky, coral,
moss, stone. The order is chosen so the first three remain distinguishable in the most common forms of
colour vision deficiency and in greyscale. Dark theme substitutes lighter variants.

---

## 3. Theming

- Two complete themes. `data-theme="light|dark"` on `<html>`.
- Each page declares its own default with `data-theme-default`. **Marketing and the live interview
  default to dark** (focus, cinema-like calm); **the application defaults to light** (long reading
  sessions, dense tables, printed-document familiarity for recruiters).
- The user's explicit choice always wins and persists in `localStorage` under `prepeet.theme`.
- `theme.js` runs synchronously in `<head>` so there is no flash of the wrong theme.
- Every `[data-theme-toggle]` button anywhere in the DOM is wired automatically and keeps its
  `aria-pressed`, label and icon in sync.

---

## 4. Typography

| Family | Use | Notes |
| --- | --- | --- |
| **Figtree** | All interface text | Humanist grotesque; open apertures read well at 13px in dense tables |
| **Fraunces** | Display: marketing headlines, pull quotes, coach summaries | A variable serif with optical sizing; used sparingly, never for UI chrome |
| **JetBrains Mono** | IDs, timestamps, routes, code, audit values | Tabular by construction |

Scale (`--text-2xs` 11px → `--text-5xl` 60px) with four line-height tokens. Interface base is 15px;
`data-density="dense"` drops it to 14px for platform screens. Numerals in metrics, tables and timers
use `font-variant-numeric: tabular-nums` so figures do not jitter as they update.

---

## 5. Spacing, grid, layout

- 4px base scale, `--sp-1` … `--sp-24`.
- Three content widths: `--content-max 1320px` (default app), `--content-max-narrow 880px` (reading
  and forms), `--content-max-dense 1600px` (platform admin).
- Sidebar 264px, collapsible to 72px on desktop, off-canvas below 1024px.
- Standard page grids: `.grid-2/3/4`, `.grid-main-aside` (content + 340px rail), `.metrics-row`
  (auto-fit, min 190px).

### Data-density rules

| Surface | Density | Rules |
| --- | --- | --- |
| Candidate | Generous | 15px base, 44px minimum touch target, one primary action per view, cards over tables, bottom navigation on mobile |
| Recruiter | Balanced | 15px base, 12px table cells, tables allowed but every row carries a band word and a confidence, not just a number |
| Platform admin | Dense | `data-density="dense"`, 14px base, 8px table cells, more columns, sparklines instead of full charts, horizontal scroll preferred over stacking where comparability matters |

---

## 6. Radii, borders, elevation

Radii: 4 / 6 / 10 / 14 / 20 / 28px plus pill. Cards use 14px, inputs and buttons 10px, dialogs 20px.

Four elevation levels. Elevation communicates **layering**, never importance: `xs` for resting cards,
`sm` for hover, `md` for popovers and floating controls, `lg` for dialogs, drawers and toasts.
Borders do most of the work; shadows are deliberately soft and low-contrast, especially in dark theme.

---

## 7. Motion

Three durations (120 / 180 / 320ms) and two easings (`--ease-out` for entrances, `--ease-in-out` for
state changes). Rules:

- Motion never carries information on its own. Every animated state also has a text or icon change.
- Nothing loops in the periphery except the live waveform and the connection indicator, which are
  genuinely conveying live state.
- `prefers-reduced-motion: reduce` collapses all animation to ~0ms, and users can force the same
  behaviour from Settings → Accessibility (`data-motion="reduced"`), which persists.
- The live-interview orb and waveform fall back to a static, still-legible representation under
  reduced motion rather than disappearing.

---

## 8. Breakpoints

| Name | Width | What changes |
| --- | --- | --- |
| xs | 320px | Single column everywhere; bottom nav; tables stack to cards; type does not shrink below 13px |
| sm | 480px | Two-up metric cards |
| md | 768px | Bottom navigation disappears; tables return to rows; two-column grids |
| lg | 1024px | Sidebar becomes persistent rather than off-canvas; side rails appear |
| xl | 1280px | Full `grid-main-aside`; compare view shows three columns |
| 2xl | 1600px | Platform admin uses the wide content max |

Candidate journeys are designed mobile-first — the prepare, live interview and completion screens are
built to work one-handed on a phone before anything else.

---

## 9. Iconography

Lucide, stroke-based, four sizes (`.ic-sm` 14 / `.ic` 18 / `.ic-lg` 22 / `.ic-xl` 28) with stroke
weight decreasing as size increases so optical weight stays constant. Icons are always decorative
(`aria-hidden`) and always accompanied by a text label or an `aria-label` on their control. Icons are
never the sole carrier of status.

---

## 10. Score and evidence visualisation — the product-specific rules

These are the rules that make the product trustworthy, and they are enforced in CSS and in copy.

### Bands

| Band | Range | Colour token | Word shown |
| --- | --- | --- | --- |
| Strong | 80–100 | `--score-strong` | "Strong" |
| Solid | 65–79 | `--score-solid` | "Solid" |
| Developing | 50–64 | `--score-developing` | "Developing" |
| Needs work | 0–49 | `--score-concern` | "Needs work" |
| Insufficient evidence | no score | `--score-insufficient` (hatched) | "Insufficient evidence" |

**The band word is always rendered next to the colour.** No score is ever communicated by colour alone.

### Uncertainty

- A score is never displayed without its confidence and the evidence count behind it.
- `.comp-bar` renders a confidence-range marker (`.ci`) in its own lane directly beneath the bar,
  with end caps at the bounds, plus the numeric bounds printed beside the score. It is not drawn
  over the value: an overlay legible against the dark track disappears against a saturated fill.
  Where two candidates' ranges overlap, the comparison screen states in words that the difference
  is not meaningful.
- A competency with too little evidence renders as a hatched "Insufficient evidence" bar and an
  explicit `n/a`, never as a low score. Fabricating a number from thin evidence is treated as a bug.
- Coverage (how much of the rubric the conversation actually reached) is shown wherever a score is.

### Evidence spans

Four kinds, each with a colour, an underline style and a legend entry:

| Kind | Meaning |
| --- | --- |
| **Supporting** (`--ev-supporting`) | A quote that evidences a competency |
| **Contradiction** (`--ev-contradiction`) | Two statements that differ — surfaced neutrally, as something to clarify, never as an accusation |
| **Claim** (`--ev-claim`) | An assertion the conversation could not corroborate — "unverified" never means "untrue" |
| **Gap** (`--ev-gap`, dashed) | A place where an expected answer did not appear |

Every score traces back to spans; every span traces back to a timestamp in the transcript and a
position in the audio.

---

## 11. Focus and keyboard

A single focus treatment: a 3px ring in `--focus-color`, applied via `:focus-visible` so pointer users
do not see it. It is defined once in `base.css` and inherited by every component; no component removes
it. Custom controls (segmented buttons, option cards, OTP inputs, the scrubber, evidence spans) all
carry it. A skip link is injected on every app page.

---

## 12. Content rules baked into the system

- Candidate copy: supportive and direct. Name the single highest-value change; never scold.
- Recruiter copy: objective and evidence-first. Describe what was said, not what it means about
  the person.
- Platform copy: concise and operational. Nouns, numbers and verbs.
- Never claim personality, emotional state, honesty, or certainty about future job performance.
- Never present the platform as making a hiring decision. A named human records the decision, and
  disagreeing with the suggested band requires a written reason that appears in the audit trail.
