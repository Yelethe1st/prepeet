# Accessibility notes

Target: **WCAG 2.2 level AA**. This document records the approach, the patterns used, what was
verified, and — importantly — what a prototype of this kind cannot honestly claim.

---

## 1. Approach

Accessibility is implemented in the shared layer rather than per page, so it cannot be forgotten on
screen 43 of 52:

- `base.css` defines one focus treatment, the skip-link, the `.sr-only` utility and reduced-motion
  handling. No component overrides them.
- `shell.js` injects the skip link, the landmark structure, `aria-current="page"` on the active
  navigation item, and the "opens in a new tab" note on external links, on every application screen.
- `ui.js` owns the keyboard contracts for tabs, accordions, dialogs, drawers, menus and tooltips, so
  those behave identically everywhere.
- `Prepeet.announce()` provides a single polite live region for status changes.

---

## 2. Structure and semantics

- One `<h1>` per page; heading levels are contiguous.
- Landmarks on every application screen: `<nav aria-label="Primary">` (sidebar),
  `<nav aria-label="Primary, mobile">` (bottom bar), `<nav aria-label="Breadcrumb">`, `<main
  id="main-content">`, and `<aside aria-label="…">` for side rails.
- Secondary navigations and sections use `aria-labelledby` pointing at their visible heading, so a
  screen-reader rotor lists meaningful region names.
- Lists are lists, tables are tables. No `<div>` grids masquerading as tabular data.
- Every page begins with a skip link that is visible on focus.

---

## 3. Keyboard

Everything is operable without a pointer. Contracts:

| Component | Keys |
| --- | --- |
| Tabs | `←/→` or `↑/↓` move between tabs, `Home`/`End` jump to first/last, activation is automatic; `Tab` moves into the panel |
| Accordion | `Enter`/`Space` toggles; `aria-expanded` reflects state |
| Dialog | Native `<dialog>` + `showModal()` — focus is trapped and restored, `Esc` closes, backdrop click closes |
| Drawer | Same as dialog; opens from the right, returns focus to its trigger |
| Menu | `Enter`/`Space` opens and moves focus to the first item, `Esc` closes and restores focus, click-outside closes |
| Sidebar (mobile) | Opening moves focus to the close button; `Esc` closes and returns focus to the toggle |
| Segmented control | Buttons with `aria-pressed`; each is individually tabbable |
| Option cards | A real `<input type="radio">`/`checkbox` visually replaced — arrow keys work as they do natively |
| OTP input | Auto-advance on entry, `Backspace` moves back, paste of a full code fills every box, one grouped label |
| Audio player | Scrubber is `<input type="range">` — arrow keys seek; all transport controls are buttons with labels |
| Live interview | `M` mute, `C` captions, `Space` push-to-talk (hold), `Esc` opens the leave dialog; shortcuts listed in an in-page help popover |
| Wizard | Back/Next are buttons; failing validation moves focus to the first invalid field |
| Tables | Row actions are real buttons/links in the tab order; nothing is hover-only |

Focus is never lost: any control that removes or replaces its own container moves focus somewhere
sensible and announces the change.

---

## 4. Focus visibility

A single 3px ring in `--focus-color`, applied through `:focus-visible` so pointer users are not
distracted by it while keyboard users always see it. The ring colour differs per theme to maintain
contrast against both light and dark surfaces. It is never removed — including on custom controls
(evidence spans, option cards, segmented buttons, the scrubber, OTP boxes).

WCAG 2.2 additions specifically addressed:

- **2.4.11 Focus Not Obscured (Minimum)** — sticky headers use `scroll-margin-top` on anchor targets;
  the bottom navigation does not overlay focusable content because content has bottom padding
  reserved for it.
- **2.5.8 Target Size (Minimum)** — interactive targets are at least 24×24px; primary candidate
  controls are 40–44px. The live-interview microphone button is 64px.
- **3.3.7 Redundant Entry** — the wizard preserves entries across back-navigation; the invitation flow
  does not re-ask for information the invitation already carries.
- **3.2.6 Consistent Help** — the help entry point sits in the same place in the candidate sidebar
  footer on every screen.

---

## 5. Colour and contrast

- Body text and interactive labels meet 4.5:1 against their surface in both themes; large text and
  non-text indicators meet 3:1.
- Semantic colours were chosen for contrast first: coral rather than pure red, moss rather than a
  bright green, and lighter variants swapped in for dark theme so foregrounds stay legible.
- **No information is carried by colour alone, anywhere.** Specifically:
  - every score band prints its word (Strong / Solid / Developing / Needs work / Insufficient
    evidence) beside the colour;
  - status dots always sit next to a status word;
  - evidence spans differ by underline style as well as hue, and every transcript has a legend;
  - chart series carry a legend and a text summary; the "insufficient evidence" bar is hatched;
  - table warning rows carry an icon and text, not a tint.

The design-system page includes a contrast table for the key pairings.

---

## 6. Forms

- Every control has a programmatic label — a visible `<label>` wherever possible, `.sr-only` where the
  visual design genuinely cannot carry one (topbar search, table row checkboxes).
- Hints are associated with `aria-describedby`; error messages are too, and set `aria-invalid="true"`.
- Validation is on submit rather than on every keystroke, error text is specific about what to do, and
  focus moves to the first invalid field.
- Grouped controls use `<fieldset>` + `<legend>` (wizard steps, radio card groups, the OTP input,
  notification matrices).
- Required fields are marked in text, not with an asterisk alone.
- Destructive confirmations require a typed value or an explicit reason — never a single ambiguous
  click.

---

## 7. Status, motion and time

- Status changes are announced through a polite live region: device-check results, connection state,
  processing stage transitions, filter result counts, save confirmations and toasts.
- Toasts use `role="status"` and are dismissible; nothing important is *only* in a toast.
- `prefers-reduced-motion: reduce` collapses all animation. Users can also force it from
  Settings → Accessibility, which sets `data-motion="reduced"` and persists.
- Under reduced motion the live waveform and persona orb render as static, still-informative states
  rather than disappearing — the information they carry (who is speaking) remains in the text
  indicators regardless.
- No time limit in the interface causes loss of work. The interview timer is informational; the OTP
  resend cooldown is a courtesy, not a deadline; the session-expiry countdown on the routing screen
  can be cancelled with a visible button.

---

## 8. Charts and data

Every chart is hand-authored inline SVG with:

1. `role="img"` and an `aria-label` summarising what it shows,
2. a visible `<p class="chart-summary">Text summary: …</p>` carrying the actual numbers,
3. a "View as table" toggle revealing a real `<table>` wherever the chart is the primary content.

Tables that must stay tabular on small screens scroll horizontally inside `.scroll-x` with a visible
explanation of why; tables where each row is independent become stacked cards below 768px using
`.table-responsive`, with `data-label` supplying the column name for each cell.

---

## 9. Interview-specific accessibility

The live interview is the hardest screen to get right, and it is where accessibility work matters
most:

- **Captions** can be toggled on and persist; caption history is available in a drawer so a candidate
  can re-read what was asked.
- **Push-to-talk** is offered as a first-class alternative to open-mic, operable by space bar or
  pointer, with its state announced.
- **Extra thinking time** is an accommodation offered at setup and at invitation, with an explicit
  statement that it does not affect evaluation.
- Speaking state is conveyed three ways — a text pill, an icon, and motion — so it survives the loss
  of any one channel.
- The end-interview confirmation states the consequences in plain language, differently for practice
  and screening.
- Accommodation requests are captured in the invitation flow rather than requiring a candidate to
  disclose to a recruiter directly.

---

## 10. What has been verified, and how

| Check | Method | Result |
| --- | --- | --- |
| Heading structure, one `h1` per page | Automated pass over all HTML files | Pass |
| No `href="#"` dead links | Automated | Pass |
| Every internal link resolves to a real file | Automated link check across all pages | Pass |
| Every `<img>`/inline SVG is decorative or labelled | Automated + manual | Pass — the prototype contains no raster images |
| Form controls have labels | Automated pass for `id`/`for`, `aria-label`, `aria-labelledby` | Pass |
| Tab / accordion / dialog ARIA wiring | Automated attribute check + manual keyboard pass | Pass |
| Colour contrast on key pairings | Computed from token values | Pass at AA; documented in `design-system.html` |
| Keyboard-only traversal of core journeys | Manual | Pass |
| Reduced motion | Manual, both via OS preference and the in-app toggle | Pass |

### Limits of these claims

This is a prototype, and the honest caveats are:

- **No screen-reader testing has been performed** with NVDA, JAWS or VoiceOver. ARIA has been written
  to spec and the patterns are conventional, but spec-correct is not the same as tested-correct. This
  is the single largest gap.
- **No testing with real assistive-technology users.** Nothing here has been validated with the people
  it is designed for.
- **Contrast ratios were computed from token values**, not sampled from rendered pixels, so
  antialiasing and any browser colour management are unaccounted for.
- **Simulated audio** means the real player's accessibility — buffering announcements, media keys,
  caption timing against real speech — is untested.
- **Automated tooling** (axe, Lighthouse, WAVE) has not been run in a browser; the checks above were
  scripted against the source, which cannot see computed contrast, focus order after dynamic
  rendering, or reflow.

Before any of this ships, it needs: an axe/Lighthouse pass per screen, a manual screen-reader pass on
the interview flow at minimum, and testing with disabled candidates — the interview screen in
particular, where the cost of getting it wrong falls on someone applying for a job.
