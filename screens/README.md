# Prepeet — high-fidelity HTML prototype

Prepeet is a multi-tenant, voice-based AI interview platform with two distinct modes:

- **`practice`** — candidate-focused interview preparation: scoring, coaching, retries and long-term progression.
- **`screen`** — recruiter-controlled candidate assessment: candidates see only a submission confirmation, while recruiters receive evidence-backed evaluations and review tooling.

It is delivered as **53 HTML screens** — 51 user-facing application routes plus the design-system
showcase and the prototype index.

This repository is a **complete, connected, front-end-only prototype**. Every screen is real HTML with
seeded data, working navigation, interaction states and both themes. There is no backend, no build step
and no package installation.

---

## 1. Preview

### Option A — open the file directly

```
open prepeet/screens.html
```

`screens.html` is the prototype index: every route, grouped by audience, with a short description
of what the screen does. Start there.

`index.html` is the **marketing landing page** (application route `/`), not the index of mockups.

### Option B — serve it locally (recommended)

A static server avoids any `file://` quirks and makes query-string states behave exactly as they
would in the real app:

```bash
cd prepeet
python3 -m http.server 4173
# then open http://localhost:4173/screens.html
```

Any static server works (`npx serve`, `php -S localhost:4173`, VS Code Live Server, …).

### Network assets

Two resources load from a CDN, matching the convention used by the reference project in `../Tsakiris`:

| Asset | Source | Fallback if offline |
| --- | --- | --- |
| Figtree, Fraunces, JetBrains Mono | Google Fonts | System font stacks are declared in `tokens.css`; layout is unaffected |
| Lucide icons | `unpkg.com/lucide` | Icons do not render; **all icons are decorative and every control keeps a text label**, so nothing becomes unusable |

There are **no local image assets to miss** — the logo, favicon, avatars, charts and the QR placeholder
are all inline SVG or CSS.

---

## 2. Project structure

```
prepeet/
├── README.md                 ← you are here
├── screens.html              ← prototype index (start here)
├── index.html                ← marketing landing page (route "/")
├── design-system.html        ← design-system showcase and documentation
│
├── login.html  register.html  oauth-callback.html  … (13 public/auth screens)
├── candidate-*.html                                  (14 candidate screens)
├── admin-*.html                                      (21 recruiter/tenant/platform screens)
├── error-403.html  error-404.html  error-500.html    (3 system screens)
│
├── assets/
│   ├── css/
│   │   ├── tokens.css        ← brand, light/dark themes, type, spacing, motion, chart + score palettes
│   │   ├── base.css          ← reset, typography defaults, focus, reduced motion, utilities
│   │   ├── components.css    ← every reusable component
│   │   ├── layout.css        ← app shell, page headers, grids, auth layout, live-interview layout
│   │   └── marketing.css     ← public website only
│   └── js/
│       ├── theme.js          ← theme + motion preference, persisted, runs before paint
│       ├── data.js           ← seeded mock data and stable IDs
│       ├── ui.js             ← declarative component behaviours (tabs, dialogs, toasts, rings, …)
│       └── shell.js          ← renders sidebar / topbar / mobile nav from one permission-aware config
│
└── docs/
    ├── route-inventory.md    ← every route, verified, with its file and status
    ├── inferred-screens.md   ← screens added beyond the brief, and why
    ├── design-system.md      ← design-system summary
    ├── component-inventory.md
    ├── navigation-map.md     ← navigation and user-flow map
    ├── assumptions.md
    ├── open-questions.md     ← unresolved product questions
    ├── accessibility.md
    └── qa-checklist.md       ← QA checklist with results
```

Pages are **flat files in the project root** so that any screen can be opened directly, with plain
relative links between them. Shared chrome is not duplicated per page: `shell.js` renders the sidebar,
topbar and mobile navigation from a single config, so navigation is consistent and permission-aware
everywhere.

---

## 3. How to drive the prototype

The prototype is meaningfully interactive. These controls are built into the chrome:

| Control | Where | What it does |
| --- | --- | --- |
| **Theme toggle** | Every page header / topbar | Switches light ⇄ dark and persists in `localStorage`. Marketing and the live interview default to dark; the app defaults to light. |
| **Mock-data mode** | Candidate topbar (`Practice` / `Screen`) | Switches the seeded data between a practice candidate and a screen-mode (employer-invited) candidate, so you can verify the mode boundary. Also settable with `?mode=practice\|screen`. |
| **Viewing as** | Admin topbar | Switches role between Recruiter, Tenant admin, Platform admin and Super administrator. **Navigation and in-page affordances change** — you only see links to what the role can reach. Also settable with `?role=…`. |
| **Preview state** | Most data screens | Flips between default / loading / empty / error (and screen-specific states) via `?state=…`. |
| **Prototype simulation** | Live interview screen | Jumps the simulated session to connecting, unstable, reconnecting, muted-warning, final phase, etc. |

Dynamic routes use **stable mock IDs** so any screen can be opened directly:

| Entity | ID | Example |
| --- | --- | --- |
| Practice session | `ses_7Kq2XA` | `candidate-session-review.html?id=ses_7Kq2XA` |
| Screen session | `ses_4Bz9QD` | `candidate-session-complete.html?id=ses_4Bz9QD&mode=screen` |
| Candidate | `cand_R4T9MP` | `admin-recruiter-detail.html?id=cand_R4T9MP` |
| Invitation | `inv_QX72HL` | `admin-invitation-detail.html?id=inv_QX72HL` |
| Tenant | `tn_northwind` | `admin-platform-quotas.html?tenant=tn_northwind` |

Every dynamic-route page falls back to its stable ID when no `?id=` is supplied.

---

## 4. The two modes, and the boundary between them

This is the single most important product rule in the prototype, and it is enforced on every screen
that could leak.

| | Practice candidate | Screen candidate | Recruiter |
| --- | --- | --- | --- |
| Overall score | ✅ sees it | ❌ never | ✅ with confidence and coverage |
| Competency breakdown | ✅ | ❌ | ✅ per-competency evidence sufficiency |
| Coaching / model rewrites | ✅ | ❌ | ❌ (not a recruiter concept) |
| Redo a question | ✅ | ❌ | — |
| Transcript | ✅ own | ❌ not shown back | ✅ with evidence spans |
| Audio replay | ✅ own | ❌ | ✅ |
| Progression over time | ✅ | ❌ | — |
| Submission confirmation | — | ✅ **only this** | — |
| Reviewer decision + override reason | ❌ | ❌ | ✅ recorded and audited |

Screens that can render in both modes read `Prepeet.getMode()` and render mutually exclusive
`[data-mode-view]` blocks; `candidate-session-review.html` and `candidate-session-results.html` refuse
to render for a screen-mode session at all and explain why instead.

---

## 5. What this prototype deliberately does not claim

Prepeet produces **evidence**, not verdicts. Throughout the copy:

- Prepeet does not decide hiring outcomes — a named human records the decision, and disagreeing with
  the suggested band requires a written override reason that is shown in the audit trail.
- There is no personality scoring, emotion detection, sentiment analysis, honesty inference or
  "culture fit" score anywhere in the product.
- Scores are never shown without their confidence and coverage. A session with too little evidence
  says **"Insufficient evidence"** rather than producing a low number.
- Candidates are never reduced to a leaderboard; comparisons surface only differences whose confidence
  ranges do not overlap, and say so explicitly when a comparison is too close to call.

---

## 6. Documentation

| Document | Contents |
| --- | --- |
| [docs/route-inventory.md](docs/route-inventory.md) | Every route in the brief, verified, mapped to its file, plus the real total |
| [docs/inferred-screens.md](docs/inferred-screens.md) | The screens added beyond the supplied list and the journey each one completes |
| [docs/design-system.md](docs/design-system.md) | Tokens, scales, rules and the reasoning behind the visual identity |
| [docs/component-inventory.md](docs/component-inventory.md) | Every reusable component, its class names and where it is used |
| [docs/navigation-map.md](docs/navigation-map.md) | Navigation structure per role and the end-to-end user flows |
| [docs/assumptions.md](docs/assumptions.md) | Product and technical assumptions made while building |
| [docs/open-questions.md](docs/open-questions.md) | Unresolved product questions that need a decision |
| [docs/accessibility.md](docs/accessibility.md) | WCAG 2.2 AA approach, patterns and known limits |
| [docs/qa-checklist.md](docs/qa-checklist.md) | The verification checklist and its results |

---

## 7. Relationship to `../Tsakiris`

`Tsakiris` was inspected as a reference for **implementation completeness and project organisation
only** — flat, directly-openable HTML screens; a single shared head convention; a screen index page;
a dedicated design-system page; CDN fonts and icons; realistic seeded content rather than wireframes.

Prepeet's visual identity, information architecture, component library, colour system, typography and
content are entirely its own. **No file in `Tsakiris` was modified.**
