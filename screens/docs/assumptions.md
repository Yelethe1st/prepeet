# Assumptions

Everything here was decided in the absence of a specification. Each entry states the assumption, why
it was made, and what would change if it turns out to be wrong.

---

## Product model

**A1 — Practice and screen are modes of one session object, not two products.**
A session carries a `mode`; evaluation runs identically in both, and the difference is entirely in
*who is shown what*. This keeps one interview engine and one evaluation pipeline.
*If wrong:* the mode switch in the candidate topbar becomes a product switch, and the shared session
routes would need to fork.

**A2 — A single user account can hold both a practice history and screening invitations.**
Daniel Okonkwo appears both as a practice candidate and in a recruiter's roster. Practice data is
never visible to any tenant; the account is shared, the data is not.
*If wrong:* screening would need anonymous, invitation-scoped accounts, and `invitation-accept.html`
would become an account-creation flow rather than a consent flow.

**A3 — Screening candidates never see any evaluation output, including after a hiring decision.**
The strictest reading of the brief. Candidates can request their data under the privacy settings, but
the product surface shows only a submission confirmation.
*If wrong (e.g. a jurisdiction requires disclosure):* a fourth disclosure level would be needed, and
`candidate-session-complete.html` would gain a post-decision state.

**A4 — Scores are 0–100 with five bands** (Strong 80+, Solid 65–79, Developing 50–64, Needs work <50,
Insufficient evidence). Thresholds are tenant-configurable in calibration.
*If wrong:* thresholds are already tokenised and configurable; only the band words would change.

**A5 — A competency needs at least three evidence spans before it is scored**, and an overall band
needs a minimum rubric coverage. Below that the product says "Insufficient evidence" rather than
producing a number. Both are configurable per tenant in calibration.

**A6 — Confidence is derived from evidence quantity and consistency, not from anything about the
speaker.** No acoustic, prosodic, emotional or personality signal is used anywhere. This is a hard
product boundary, not a phase-one limitation.

**A7 — The recruiter's decision is the decision.** Prepeet suggests a band; a named human records the
outcome; disagreement requires a written override reason that is surfaced in the audit trail. There is
no auto-advance and no auto-reject.

**A8 — Candidates can request a re-review of a screening.** Implemented as `admin-appeals.html`.
Assumed to be a platform-level fairness commitment rather than a per-tenant option.

---

## Roles and tenancy

**A9 — Four roles in a strict hierarchy:** recruiter → tenant admin → platform admin → super
administrator, each inheriting the one below. Real deployments usually need non-hierarchical scoping
(a hiring manager who sees only their own roles); the members screen models scoped roles as an
attribute, but navigation gating is hierarchical for the prototype.

**A10 — Super-administrator access is time-boxed and reason-bound.** The god console shows an active
elevation window with a countdown and a ticket reference. Assumed to be granted through an external
on-call process, not from inside the product.

**A11 — Impersonation never exposes candidate practice data**, is visible in the impersonated tenant's
own audit log, and requires a ticket reference.

**A12 — One active tenant per admin session.** Northwind Health System throughout. Multi-tenant
switching for a user who belongs to several tenants is not modelled.

---

## Interview mechanics

**A13 — Interviews are voice-only.** No video, no coding editor, no screen share. Technical
interviews are verbal reasoning about systems and trade-offs.

**A14 — Interviews are single-session and time-boxed** (15 / 25 / 40 / 60 minutes, constrained by
shape). Practice sessions can be abandoned and resumed as a *new* session; screening sessions cannot
be restarted once ended.

**A15 — A dropped connection has a 10-minute reconnect window with the session clock stopped.**
Beyond it the session is finalised with whatever was captured, and a screening in that state is
flagged for the recruiter as low coverage rather than silently scored.

**A16 — Personas are interview *styles*, not assessors.** Ama, Ravi, Lena and Marcus differ in pacing
and pressure, not in scoring. The same rubric produces the same evaluation regardless of persona.
Stated explicitly wherever a persona is chosen.

**A17 — Push-to-talk is an accessibility and environment fallback, not a mode with different scoring.**

**A18 — Captions are generated from the same transcription used for evaluation**, so what a candidate
reads is what is evaluated.

---

## Data and compliance

**A19 — Default retention:** audio 18 months, transcripts 18 months, evaluations 18 months for screen
mode; practice data until the candidate deletes it. Tenant-configurable.

**A20 — Data residency is per tenant** (EU Frankfurt for Northwind Health). Assumed to apply to audio,
transcripts and evaluations, but not to aggregate platform metrics.

**A21 — The audit log is append-only, hash-chained, and survives tenant deletion.** Viewing it is
itself audited.

**A22 — Practice data is never used for model training by default.** The opt-in exists in candidate
settings and is off.

---

## Content and data in this prototype

**A23 — Today is 23 August 2026.** All relative dates are anchored to it.

**A24 — All organisations, people, quotes, metrics and costs are invented** and chosen to be
plausible. Northwind Health System, Orbital Labs, Meridian Schools Trust, Caldera Capital and
Brightpath Commercial do not exist. No real person is depicted.

**A25 — Model names in the AI-usage screens are generic role descriptions** (realtime voice,
transcription, primary evaluation, triage, embedding, fallback) rather than named vendor models, so
the prototype makes no claims about any specific vendor's behaviour or pricing.

**A26 — Currency is USD** on platform and billing screens; the tenant is European, so a real product
would need per-tenant currency.

**A27 — Locale is en-GB for product copy** (organisation, personalise, résumé) with US-formatted
currency, matching a UK-facing product billed in dollars.

---

## Technical

**A28 — Flat HTML files in one directory, opened directly or from any static server.** No build step,
no framework, no package manager — matching the delivery convention of the reference project in
`../Tsakiris` while using a modular CSS/JS structure rather than one oversized file.

**A29 — Shared chrome is rendered by `shell.js` rather than duplicated per page.** This is the
prototype's answer to "components are reused rather than duplicated": there is exactly one sidebar
implementation and one topbar implementation. The trade-off is that navigation requires JavaScript;
in a real build this would be server-rendered.

**A30 — Query parameters stand in for router state.** `?id=`, `?mode=`, `?role=`, `?state=`, `?tab=`,
`?step=`, `?range=` and filter parameters are read on load and written back with
`history.replaceState`, so any screen state is directly linkable — which is also how the prototype
exposes loading, empty, error and permission states for review.

**A31 — Audio playback is simulated.** No media files ship with the prototype. Players animate a
timeline and a playhead so the interaction can be evaluated; the controls are labelled honestly.

**A32 — Charts are hand-authored inline SVG.** No charting library, no external images, so the
prototype has no missing local assets and every chart can carry its own accessible description.

**A33 — Two CDN dependencies** (Google Fonts, Lucide icons), matching the reference project. Both
degrade safely: system font fallbacks are declared, and every icon is decorative with a text label
alongside it.

**A34 — `localStorage` is used for theme, motion preference, role and mode.** All reads and writes are
wrapped in `try/catch` so the prototype works in private windows and with site data blocked.
