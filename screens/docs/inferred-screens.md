# Inferred screens — what was added beyond the supplied route list, and why

The brief's three route tables list 29 user-facing routes (not the 27 quoted), and it warned that the
count might be inaccurate. Building the journeys end to end surfaced further gaps where a flow started but had nowhere to land. This document lists
every screen added beyond the supplied list, the journey it completes, and what would have been broken
without it.

Screens the brief *asked us to assess the need for* are marked **(prompted)**. Screens we identified
independently are marked **(identified)**.

---

## Authentication — 9 inferred screens

The brief supplied `/login`, `/register` and `/oauth/callback`, and asked us to infer the rest of a
complete authentication journey.

| File | Route | Why it exists |
| --- | --- | --- |
| `forgot-password.html` **(prompted)** | `/forgot-password` | `login.html` offers "Forgot password". Without this the link is a dead end. |
| `check-email.html` **(prompted)** | `/check-email` | Three separate flows (reset, verification, magic link) all need a "we've sent it" state. One screen, parameterised with `?for=`, rather than three near-identical pages. |
| `reset-password.html` **(prompted)** | `/reset-password` | The destination of the reset email. Also carries the expired-token variant that hands off to `auth-expired.html`. |
| `verify-email.html` **(prompted)** | `/verify-email` | Candidate self-sign-up creates an unverified account; something has to consume the verification link. |
| `magic-link.html` **(prompted)** | `/auth/magic` | Candidates invited to a screening should not have to create a password to answer an employer's invitation. Magic-link entry is the lowest-friction path, and it needs a landing state. |
| `otp.html` **(prompted)** | `/auth/otp` | Recruiter and admin accounts handle candidate personal data; MFA is assumed for those roles, so a second-factor entry screen is required. |
| `auth-expired.html` **(prompted)** | `/auth/expired` | Every emailed link expires. Without a dedicated screen, an expired link produces an error with no recovery path. |
| `access-denied.html` **(prompted)** | `/access-denied` | Distinct from `error-403.html`: this is the *authentication-level* case — you are signed in, but this account has no access to the workspace you asked for. It offers switching account or requesting access, which an in-app 403 cannot. |
| `invitation-accept.html` **(prompted)** | `/invite/[token]` | The candidate-facing half of the screen-mode invitation. It is where consent to recording is captured and where the candidate is told exactly what the employer will and will not receive. Without it, screen mode has no entry point at all. |

---

## Recruiter and tenant administration — 9 inferred screens

| File | Route | Why it exists |
| --- | --- | --- |
| `admin-recruiter-detail.html` **(prompted)** | `/admin/recruiter/[candidateId]` | The single most important recruiter screen and the one the brief specified in most detail. The roster lists candidates; this is where a reviewer actually inspects evidence, transcript, audio, coverage, contradictions, claim verification, JD compatibility and records a decision with an override reason. Without it the roster links nowhere. |
| `admin-invitations.html` **(prompted)** | `/admin/invitations` | Screen mode is recruiter-initiated. Without an invitation list there is no way to see who was invited, who has not responded, or what is about to expire. |
| `admin-invitation-new.html` **(prompted)** | `/admin/invitations/new` | The action that starts every screen-mode session. Also the only place the candidate-facing transparency text is composed. |
| `admin-invitation-detail.html` **(prompted)** | `/admin/invitations/[id]` | Carries the lifecycle timeline and, critically, **revocation** — which the brief called out explicitly and which needs its own consequences explanation (revoking access is not the same as deleting an evaluation). |
| `admin-members.html` **(prompted)** | `/admin/members` | The brief requires permission-aware navigation. Somewhere has to grant those permissions, and the role matrix has to be legible to the person granting them. |
| `admin-tenant-settings.html` **(prompted)** | `/admin/settings` | Retention, data residency, candidate-facing copy and interview defaults are all tenant-level decisions with a compliance dimension. `admin-dashboard` is an overview, not a settings surface. |
| `admin-rubrics.html` **(prompted)** | `/admin/rubrics` | Calibration tunes anchors *within* a rubric for a role. Rubrics themselves need to be created, cloned, versioned, archived and assigned to roles — a different job at a different altitude. |
| `admin-integrations.html` **(prompted)** | `/admin/integrations` | Webhooks, ATS sync and API keys. The platform emits `evaluation.ready`; something has to configure where that goes, and failed deliveries need a visible log. |
| `admin-appeals.html` **(prompted)** | `/admin/recruiter/appeals` | A candidate whose audio dropped mid-answer needs a route to a re-review, and the platform needs somewhere to surface its own low-confidence evaluations for human attention. This is the fairness backstop; without it, "evidence-driven and trustworthy" is a claim with no mechanism behind it. |

---

## Platform administration — 1 inferred screen

| File | Route | Why it exists |
| --- | --- | --- |
| `admin-platform-quotas.html` **(prompted)** | `/admin/platform/quotas` | `llm-usage` *reports* consumption. Quotas are the *control* surface: allowances, soft and hard limits, overage policy, spend caps, per-model rate limits and temporary headroom grants. Reporting and control were separated so that a read-only cost review cannot accidentally change a tenant's limits. |

---

## System states — 3 inferred screens

| File | Route | Why it exists |
| --- | --- | --- |
| `error-403.html` **(prompted)** | in-app permission denied | Rendered inside the admin shell so the user keeps their navigation and can see what their role *can* reach. Names the required role versus the current one. |
| `error-404.html` **(prompted)** | not found | Standalone. Shows the attempted path and offers destinations for all three audiences. |
| `error-500.html` **(prompted)** | application error | Standalone. States what has already happened automatically (error reported, in-flight interviews preserved) and gives a correlation id a user can quote to support. |

---

## Meta — 1 added screen

| File | Why it exists |
| --- | --- |
| `screens.html` **(identified)** | The prototype index. `/` is the marketing landing page and cannot double as a mockup index, so the route-by-route gallery lives here. It is not an application route and is documented as prototype scaffolding. |

`design-system.html` is required by the brief rather than inferred, and is likewise not an application
route.

---

## Two route decisions the brief asked us to resolve

### 1. Candidate progression was missing from the sidebar

**Resolved by adding it to navigation, not by hiding it behind another screen.**
`/candidate/progression` now sits in the **"Your practice"** sidebar group, between *Skills* and
*Goals*, because that group is exactly "your practice over time" and progression is the temporal view
of it. It also appears in the mobile bottom navigation as **Progress** (one of five slots), because
long-term improvement is the reason a candidate returns to the product and burying it behind a
sub-navigation on mobile would make the retention loop invisible on the device most candidates use.

Additional discoverable entry points: the readiness card on the dashboard links to it, the skills page
cross-links to it, and the review screen's "patterns across sessions" panel links to it.

### 2. `review` and `results` overlapped

**Resolved by keeping both with visibly distinct responsibilities**, because they answer different
questions and are used at different moments.

| | `/candidate/session/[id]/results` | `/candidate/session/[id]/review` |
| --- | --- | --- |
| Question it answers | *What happened, and what is the evidence?* | *What do I do differently next time?* |
| Content | Score, competency breakdown with confidence, coverage, evidence spans, full transcript, audio replay | Per-turn evaluation, what worked / what to change, model rewrites, redo a question, drills |
| Posture | Read-only, factual, exportable | Active, prescriptive, feeds goals |
| Typical moment | Immediately after processing, and again when revisiting a session | The evening before the next practice session |

Neither route is orphaned. Both carry the **same shared sub-navigation** — a two-item tab pair with a
one-line explainer of the split — so a user landing on either always sees the other and understands
why there are two. They cross-link in the body as well, and the completion screen offers both.

Had we collapsed them, the results view would have become a long scroll mixing "here is your score"
with "here are six rewrites of your answers", which is precisely the moment an anxious candidate stops
reading. Splitting them keeps the factual record calm and the coaching actionable.
