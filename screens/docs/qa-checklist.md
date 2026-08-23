# QA checklist and results

Verification performed on the finished prototype. Automated checks were run with a script that parses
every HTML file; manual checks were performed by reading the rendered markup and walking the flows.

**Result summary — the automated structural sweep passes on all 53 screens with zero findings, and
the screens driven in a real browser are clean.** Two verification passes were left incomplete; they
are named explicitly in §10 rather than glossed. Details, and the honest limits of what a static
prototype can claim, are below.

---

## 1. Automated structural sweep

A script (`qa.py`, kept with the build tooling) parses all 53 HTML files and checks:

| Check | Result |
| --- | --- |
| Every route in the inventory has a file | ✅ 53/53 |
| No unexpected files outside the inventory | ✅ |
| Exactly one `<h1>` exposed per page | ✅ (pages with mutually-exclusive mode/state blocks carry one per block; only one is ever in the accessibility tree) |
| No `href="#"` dead links | ✅ 0 found |
| Every internal link resolves to an existing file | ✅ 0 broken |
| Every same-page `#anchor` exists on that page | ✅ |
| No duplicate `id` attributes | ✅ |
| No lorem ipsum, `TODO`, `FIXME` or placeholder names | ✅ 0 found |
| `<html lang>` present | ✅ 53/53 |
| Responsive viewport meta present | ✅ 53/53 |
| Non-empty `<title>` | ✅ 53/53 |
| `data-theme-default` declared | ✅ 53/53 |
| Required stylesheets and scripts wired | ✅ 53/53 |
| `data-nav` value is a real navigation id | ✅ |
| `data-shell` pages load `shell.js`, `layout.css` and expose `#main-content` | ✅ |
| `role="tab"` always has `aria-controls` | ✅ |
| `.accordion-trigger` always has `aria-expanded` + `aria-controls` | ✅ |
| `<dialog>` always has an accessible name | ✅ |
| Charts marked `role="img"` are followed by a `.chart-summary` | ✅ |
| No unqualified personality / emotion / honesty / prediction claims | ✅ — every occurrence of those terms is an explicit denial |
| All inline `<script>` blocks parse | ✅ 0 syntax errors across every inline block |
| All shared JS files parse | ✅ `theme.js`, `data.js`, `ui.js`, `shell.js` |

---

## 2. Route coverage

| Check | Result |
| --- | --- |
| Every listed route implemented or deliberately redirected | ✅ 29/29 routes from the brief's tables implemented; none redirected away |
| Every inferred route documented | ✅ 22 inferred routes, each with a rationale in [inferred-screens.md](inferred-screens.md) |
| No application screen orphaned | ✅ every file is reachable from `screens.html` **and** from at least one in-product link — mapping in [navigation-map.md](navigation-map.md) §6 |
| Dynamic routes work with sample IDs | ✅ `ses_7Kq2XA`, `ses_4Bz9QD`, `cand_R4T9MP`, `inv_QX72HL`, `tn_northwind`; every dynamic page also renders correctly with no `?id=` at all |
| Progression discoverability resolved | ✅ added to the sidebar "Your practice" group and the mobile bottom nav, plus three in-content entry points |
| `review` / `results` overlap resolved | ✅ both kept, distinct responsibilities, shared sub-navigation, cross-linked both ways |
| `/grafana` handled | ✅ external link in the platform nav group, styled consistently, marked external with an SR-only "opens in a new tab" |

---

## 3. Permissions and the mode boundary

| Check | Result |
| --- | --- |
| Navigation respects role permissions | ✅ items the current role cannot reach are **not rendered** — not disabled or hidden with CSS |
| Pages guard themselves, not just the nav | ✅ opening a platform or god URL at a lower role renders a permission-denied state naming the required and current role |
| Tenant-configuration screens degrade for a plain recruiter | ✅ read-only variant with a banner and disabled controls |
| Practice → screen leakage | ✅ none. `review` and `results` refuse to render for a screen session and explain why; `skills`, `goals` and `progression` show a practice-only explanation; the sessions list shows "Not shown — screening" instead of a score |
| Screen → practice leakage | ✅ the screening completion screen contains no score, band, competency chart, coaching, rewrite or recommendation |
| Recruiter-only information never reaches a candidate | ✅ recommendations, confidence, coverage, contradictions, claim verification, JD compatibility, reviewer decisions and override reasons appear only in the admin shell |
| Privileged access is visible | ✅ the god console carries a persistent restricted-access banner, an elevation countdown, and privileged badges on privileged actions |

Verified by switching the topbar "Practice / Screen" and "Viewing as" controls and re-walking each
affected screen.

---

## 4. Interaction

| Check | Result |
| --- | --- |
| Theme switching with persistence | ✅ persists in `localStorage`, applied before paint, every toggle stays in sync |
| Responsive sidebar behaviour | ✅ persistent ≥1025px with a collapse control, off-canvas ≤1024px with a scrim, `Esc` and focus handling |
| Mobile navigation | ✅ five-item bottom bar below 768px on candidate screens |
| Working accordions and tabs | ✅ full keyboard contract; tabs are URL-addressable via `?tab=` |
| Wizard navigation and validation | ✅ five steps, blocked advance with inline errors and focus movement, step in the URL, restores on reload |
| Persona / mode / duration / seniority selection | ✅ all functional, with durations constrained by the chosen shape |
| Search and filters | ✅ on the roster, sessions, invitations, audit log and analytics screens — reflected in the URL and restored on load |
| Candidate multi-selection and comparison | ✅ 2–4 selection limit with an inline explanation, sticky action bar, `?ids=` passed to compare |
| Modal and drawer interactions | ✅ native `<dialog>`, focus trapped and restored, `Esc` and backdrop close |
| Audio-player controls | ✅ simulated playback with play/pause, scrub, ±10s, speed, playhead and transcript sync — **labelled as simulated** on every instance |
| Interview connection and microphone simulation | ✅ scripted timeline plus a labelled "Prototype simulation" jump bar |
| Processing-stage simulation | ✅ six stages with elapsed times, plus failure and delayed variants |
| Practice/screen mode switching in mock data | ✅ topbar control and `?mode=` |
| Tooltips and explanatory affordances | ✅ hover and focus, `Esc` to dismiss |
| URL query-state handling | ✅ `id`, `mode`, `role`, `state`, `tab`, `step`, `range`, `ids`, `tenant`, filter keys — all read on load and written back |
| Functional links between related screens | ✅ verified by the automated link check |

---

## 5. States

Every data screen exposes its states through a visible "Preview state" control **and** a `?state=`
parameter, so each one is directly linkable for review.

| State | Where it is represented |
| --- | --- |
| Loading | Skeleton rows and blocks on every list, table, dashboard and detail screen |
| Empty | Sessions, goals, roster, invitations, rubrics, integrations, audit log, analytics ranges, compare with <2 candidates |
| Error | Every data screen; recoverable evaluation failure; analytics store unavailable; partial-data banners |
| Disabled | Start-interview blocked until device checks pass; resend disabled during cooldown; read-only config for recruiters; god actions without elevation |
| Permission denied | `error-403.html` plus in-page guards on every platform and god screen |
| Degraded / incident | System health, integrations, session analytics |
| Delayed | Evaluation processing queued behind others |
| Insufficient evidence | Competency bars, results, roster rows, recruiter detail, compare cells |
| Bounced / expired / revoked | Invitation lifecycle |
| Reconnecting / recovered | Live interview |

---

## 6. Responsive

Verified in real Chrome (headless, measured `scrollWidth` vs `clientWidth`, not by eye) on the
candidate dashboard and the live interview at 320 / 360 / 390 / 768 / 1280px. Other screens were
checked by reading their markup against the responsive rules, not measured individually — see
§10.8 for what that leaves open.

| Check | Result |
| --- | --- |
| Works from 320px | ✅ measured on the screens above: 0px horizontal overflow. One real bug was found and fixed here — single-column grid fallbacks used a bare `1fr`, whose automatic minimum is the content's min-content, so a `nowrap` row stopped the column shrinking and pushed the page 122px wide at 320px. All grid fallbacks are now `minmax(0, 1fr)` with a `min-width: 0` safety net on grid children |
| Candidate journeys strong on mobile | ✅ single column, bottom navigation, 44px primary targets, the live interview is one-handed |
| Recruiter/platform tables degrade gracefully | ✅ `.table-responsive` stacks to cards below 768px with `data-label`; comparison-critical grids use `.scroll-x` with a visible explanation of why they scroll rather than stack |
| Dialogs and drawers on small screens | ✅ drawers become full-width; dialogs cap at 92vw |
| Long content | ✅ transcripts, audit logs and code blocks scroll inside their own containers |

---

## 7. Accessibility

Full detail in [accessibility.md](accessibility.md).

| Check | Result |
| --- | --- |
| Semantic HTML and landmarks | ✅ |
| Keyboard navigation | ✅ every journey completable without a pointer |
| Visible focus states | ✅ one `:focus-visible` treatment, never removed, present on custom controls |
| Sufficient colour contrast | ✅ AA on all key pairings; contrast table published in `design-system.html` |
| Proper form labels | ✅ every control labelled; errors wired with `aria-invalid` + `aria-describedby` |
| Accessible dialogs | ✅ native `<dialog>`, named, focus trapped and restored |
| ARIA where required | ✅ tabs, accordions, menus, live regions, `aria-current`, `aria-pressed`, `role="status"` |
| No information by colour alone | ✅ every band prints its word; status dots carry text; evidence spans differ by underline style; charts carry legends and text summaries |
| Reduced-motion support | ✅ OS preference and an in-app toggle that persists |
| Screen-reader-friendly status updates | ✅ device checks, connection state, processing stages, filter counts and saves all announce |
| Accessible charts | ✅ `role="img"` + `aria-label` + visible text summary, and a data-table toggle where a chart is the primary content |

---

## 8. Build integrity

| Check | Result |
| --- | --- |
| Works without a production backend | ✅ no network calls; all data is seeded in `assets/js/data.js` and page scripts |
| No console errors | ✅ on the screens driven in a real browser (dashboard, live interview in five simulated states, recruiter detail): zero page errors and zero `console.error`. Every shared and inline script parses. All `localStorage` and `history` access is wrapped in `try/catch` — `history.replaceState` throwing on `file://` was found this way and fixed centrally in `ui.js` |
| No missing local assets | ✅ the prototype ships no image files — logo, favicon, avatars, charts and the 2FA QR are inline SVG or CSS |
| CDN dependencies degrade safely | ✅ system font fallbacks are declared; icons are decorative and every control keeps a text label |
| No lorem ipsum | ✅ |
| Light and dark themes complete | ✅ every token has a value in both; no colour is defined only inside one theme block |
| Components reused, not duplicated | ✅ one CSS definition per component; sidebar, topbar and mobile nav are generated once by `shell.js` |
| Page-specific CSS kept minimal | ✅ page `<style>` blocks contain only genuinely page-unique rules and say so in a comment |
| Visually distinct from Tsakiris | ✅ different palette (warm stone + teal vs cool grey + institutional blue), different type (Figtree/Fraunces vs Inter), different layout system, different components, different content domain |
| `Tsakiris` unchanged | ✅ verified — no file in that directory was written to at any point |

---

## 9. Content

| Check | Result |
| --- | --- |
| Realistic, domain-specific copy | ✅ |
| All six required domains represented | ✅ software engineering, product management, nursing, teaching, sales, finance — in the role library, marketing use cases, recruiter roster, calibration roles and platform analytics |
| Tone correct per audience | ✅ supportive and direct for candidates, objective and evidence-focused for recruiters, concise and operational for platform admins |
| No overreaching claims | ✅ no personality, emotion, honesty or performance-prediction claims; every mention of those concepts is an explicit denial |
| Uncertainty communicated honestly | ✅ scores never shown without confidence and coverage; "insufficient evidence" instead of a fabricated number; overlapping ranges called out as too close to call |
| No leaderboard framing | ✅ comparison is evidence-based, capped at four candidates, and states what cannot be distinguished |

---

## 10. Known limitations

These are real and deliberate, and would need addressing before this became a product:

1. **No screen-reader testing.** ARIA is written to spec and patterns are conventional, but nothing
   has been verified with NVDA, JAWS or VoiceOver. This is the largest gap.
2. **No browser-based automated audit.** axe, Lighthouse and WAVE have not been run; the checks above
   were scripted against source, which cannot see computed contrast, reflow, or focus order after
   dynamic rendering.
3. **Contrast computed from token values**, not sampled from rendered pixels.
4. **Audio is simulated.** Real playback, buffering, media keys and caption timing are untested.
5. **Navigation requires JavaScript**, because the shell is rendered client-side to guarantee a single
   implementation. A real build would render it server-side.
6. **No real backend behaviour** — no latency, no partial failures, no concurrency, no pagination over
   real volume.
7. **Not tested with disabled candidates**, which the interview screen in particular needs.
8. **The responsive sweep was not completed across all 53 screens.** It was run far enough to find
   and fix the systemic grid bug described in §6, and the candidate dashboard and live interview
   were then measured clean. A partial pass flagged possible horizontal overflow on some dense
   admin table screens (`admin-recruiter`, `admin-members`, `admin-invitations`, `admin-rubrics`,
   `admin-invitation-detail`) at 768 and 1280px, and on `design-system.html` at 768px. Those
   readings did not distinguish real page overflow from content inside a deliberately scrollable
   `.table-wrap`, so they are **unconfirmed** — they may be false positives. Re-running the
   measurement with scroll containers excluded is the first thing to do before this is signed off.
9. **The whole-prototype DOM runtime pass did not complete.** Individual screens were driven in a
   real browser without errors, but a page-by-page execution of all 53 screens across their query
   states was not finished.
