# Verified route inventory

The brief warned that its route count might be inaccurate, and it was. Counting the rows of the three
supplied tables gives **29** user-facing routes, not 27. The supplied list also omitted screens that its
own journeys require, and left the `review` / `results` overlap unresolved.

**Verified totals**

| | Count |
| --- | --- |
| Routes listed in the brief's tables | 29 — public/auth 4, candidate 14, admin 11 |
| Supplied routes implemented | **29 (100%)** |
| Additional user-facing routes inferred and implemented | 22 |
| Routes added to close gaps found against `docs/` | 3 |
| **Total user-facing application routes** | **54** |
| Non-route prototype pages (`design-system.html`, `screens.html`) | 2 |
| **Total HTML files** | **56** |

Counted by layer: public and authentication 13 · candidate 15 · recruiter and tenant administration 15 ·
platform administration 8 · system states 3 · non-route pages 2.

The three added routes are `/candidate/session/[id]/articulation`, `/admin/campaigns` and
`/admin/campaigns/[id]`. Each closes a capability that the specification in `docs/` requires and the
original brief's route tables did not list — see [inferred-screens.md](inferred-screens.md).

BFF/API endpoints are not counted as screens anywhere in this document.

Legend — **Status:** ✅ implemented · **Source:** `brief` = in the supplied list, `inferred` = added to
complete a journey, `spec-gap` = added to cover a capability required by `docs/` that no screen
implemented (rationale in [inferred-screens.md](inferred-screens.md)).

---

## 1. Public and authentication — 13 routes

| # | Route | File | Source | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| 1 | `/` | `index.html` | brief | ✅ | Marketing landing. Dark by default with a persistent theme toggle. All 12 required sections. |
| 2 | `/login` | `login.html` | brief | ✅ | Password, dynamically loaded OAuth providers, `?next=` redirect handling, `?error=oauth_denied` failure state |
| 3 | `/register` | `register.html` | brief | ✅ | Candidate or organisation sign-up |
| 4 | `/oauth/callback` | `oauth-callback.html` | brief | ✅ | Handoff and redirect processing, plus failure and slow variants |
| 5 | `/forgot-password` | `forgot-password.html` | inferred | ✅ | |
| 6 | `/check-email` | `check-email.html` | inferred | ✅ | One screen for three flows via `?for=reset\|verify\|magic`; real resend cooldown |
| 7 | `/reset-password` | `reset-password.html` | inferred | ✅ | Live requirements checklist; `?state=expired` |
| 8 | `/verify-email` | `verify-email.html` | inferred | ✅ | verifying / verified / already-verified / failed |
| 9 | `/auth/magic` | `magic-link.html` | inferred | ✅ | Signing in / success / already used / wrong device |
| 10 | `/auth/otp` | `otp.html` | inferred | ✅ | Auto-advance, paste support, resend countdown, recovery-code link |
| 11 | `/auth/expired` | `auth-expired.html` | inferred | ✅ | Expired or invalid authentication link |
| 12 | `/access-denied` | `access-denied.html` | inferred | ✅ | Authenticated but no workspace access — distinct from the in-app 403 |
| 13 | `/invite/[token]` | `invitation-accept.html` | inferred | ✅ | Candidate invitation acceptance with consent and full disclosure. Stable token `inv_QX72HL` |

---

## 2. Candidate application — 15 routes

| # | Route | File | Source | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| 14 | `/candidate` | `candidate.html` | brief | ✅ | Routing state. Three resolved outcomes (returning / first-run / invited) plus an error state |
| 15 | `/candidate/dashboard` | `candidate-dashboard.html` | brief | ✅ | Mode-aware: practice readiness view and screen submission view |
| 16 | `/candidate/start-interview` | `candidate-start-interview.html` | brief | ✅ | Five-step wizard, URL-addressable via `?step=`, validated. Blocked in screen mode |
| 17 | `/candidate/session/[id]/prepare` | `candidate-session-prepare.html` | brief | ✅ | Brief, persona preview, interactive device checks, saved-progress notice |
| 18 | `/candidate/session/[id]` | `candidate-session-live.html` | brief | ✅ | Live realtime voice interview. All 12 required elements; see §6 |
| 19 | `/candidate/session/[id]/complete` | `candidate-session-complete.html` | brief | ✅ | Six processing stages plus complete-practice, complete-screen, failed and delayed |
| 20 | `/candidate/session/[id]/review` | `candidate-session-review.html` | brief | ✅ | **Coaching review** — per-turn evaluation, rewrites, redo, drills. Practice only |
| 21 | `/candidate/session/[id]/results` | `candidate-session-results.html` | brief | ✅ | **Outcome & evidence** — score, competencies, evidence, transcript, replay. Practice only |
| 22 | `/candidate/sessions` | `candidate-sessions.html` | brief | ✅ | State-aware links per status; filters persisted in the URL |
| 23 | `/candidate/skills` | `candidate-skills.html` | brief | ✅ | Competency breakdown with the evidence behind each score |
| 24 | `/candidate/goals` | `candidate-goals.html` | brief | ✅ | Goals, milestones, cadence, streaks |
| 25 | `/candidate/profile` | `candidate-profile.html` | brief | ✅ | Profile, résumé, career context, evidence bank |
| 26 | `/candidate/settings` | `candidate-settings.html` | brief | ✅ | Account, notifications, accessibility, privacy, interview defaults |
| 27 | `/candidate/progression` | `candidate-progression.html` | brief | ✅ | Long-term progression. **Now in the sidebar** — see §7 |
| 27a | `/candidate/session/[id]/articulation` | `candidate-session-articulation.html` | spec-gap | ✅ | **Delivery** — deterministic measurement, ten dimensions, assessability, timestamped observations, drills, original/redo comparison, personal baseline. Practice only |

---

## 3. Recruiter and tenant administration — 15 routes

| # | Route | File | Source | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| 28 | `/admin/dashboard` | `admin-dashboard.html` | brief | ✅ | Tenant overview for Northwind Health System |
| 29 | `/admin/recruiter` | `admin-recruiter.html` | brief | ✅ | Roster with filters, decisions and a pending-review state |
| 30 | `/admin/recruiter/compare` | `admin-recruiter-compare.html` | brief | ✅ | Evidence-based comparison, 2–4 candidates, uncertainty made explicit |
| 31 | `/admin/recruiter/calibration` | `admin-recruiter-calibration.html` | brief | ✅ | Per-tenant, per-role rubric anchors, thresholds, weights, versioning |
| 32 | `/admin/recruiter/[candidateId]` | `admin-recruiter-detail.html` | inferred | ✅ | The recruiter review detail. All 14 required inspection capabilities; see §6 |
| 33 | `/admin/invitations` | `admin-invitations.html` | inferred | ✅ | Screen-mode invitation list |
| 34 | `/admin/invitations/new` | `admin-invitation-new.html` | inferred | ✅ | Create invitation with a live email preview |
| 35 | `/admin/invitations/[id]` | `admin-invitation-detail.html` | inferred | ✅ | Lifecycle, delivery, and revocation with consequences |
| 36 | `/admin/members` | `admin-members.html` | inferred | ✅ | Members, roles and the permission matrix |
| 37 | `/admin/settings` | `admin-tenant-settings.html` | inferred | ✅ | Organisation, defaults, candidate experience, retention, notifications, billing |
| 38 | `/admin/rubrics` | `admin-rubrics.html` | inferred | ✅ | Rubric library above calibration |
| 39 | `/admin/integrations` | `admin-integrations.html` | inferred | ✅ | ATS, webhooks, delivery log, API keys |
| 40 | `/admin/recruiter/appeals` | `admin-appeals.html` | inferred | ✅ | Candidate appeals and system-flagged low-confidence evaluations |
| 40a | `/admin/campaigns` | `admin-campaigns.html` | spec-gap | ✅ | Campaign list, scoped to the campaigns a recruiter belongs to, with the create flow |
| 40b | `/admin/campaigns/[id]` | `admin-campaign-detail.html` | spec-gap | ✅ | Pinned configuration, invitation funnel, candidates awaiting a human review, append-only history |

---

## 4. Platform administration — 8 routes

| # | Route | File | Source | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| 41 | `/admin/platform/analytics` | `admin-platform-analytics.html` | brief | ✅ | Cross-tenant analytics |
| 42 | `/admin/platform/analytics/sessions` | `admin-platform-analytics-sessions.html` | brief | ✅ | Session analytics and realtime health |
| 43 | `/admin/platform/analytics/evaluations` | `admin-platform-analytics-evaluations.html` | brief | ✅ | Evaluation quality and outcomes |
| 44 | `/admin/platform/system` | `admin-platform-system.html` | brief | ✅ | Services, queues, failed jobs, latency, runtime policy versions |
| 45 | `/admin/platform/system/llm-usage` | `admin-platform-system-llm-usage.html` | brief | ✅ | Model and provider usage, token and audio cost |
| 46 | `/admin/platform/quotas` | `admin-platform-quotas.html` | inferred | ✅ | Quota **control** surface, separated from cost reporting |
| 47 | `/admin/god` | `admin-god.html` | brief | ✅ | Restricted super-administrator console, elevation-gated |
| 48 | `/admin/god/audit` | `admin-god-audit.html` | brief | ✅ | Privileged append-only audit viewer |

**External operational link.** `/grafana` remains an external link in the platform navigation group.
It is styled identically to the other platform items, carries the `.ext` indicator,
`target="_blank" rel="noopener noreferrer"` and a screen-reader-only "(opens in a new tab)". It is not
counted as a screen.

---

## 5. System states — 3 routes

| # | Route | File | Source | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| 49 | in-app permission denied | `error-403.html` | inferred | ✅ | Rendered inside the admin shell; names required vs current role |
| 50 | not found | `error-404.html` | inferred | ✅ | Shows the attempted path; destinations for all three audiences |
| 51 | application error | `error-500.html` | inferred | ✅ | Standalone. Correlation id, what is already safe, retry and status-page link |

---

## 6. Required-element verification for the two most-specified screens

### `/candidate/session/[id]` — live interview

| Required element | Implemented |
| --- | --- |
| Current interview phase | ✅ Phase chip, advances through the scripted session |
| Interviewer persona | ✅ Orb, name, style, speaking animation |
| Connection state | ✅ connecting → connected → unstable → reconnecting → recovered, dot **and** text |
| Microphone state and controls | ✅ `aria-pressed` mute, input-level meter with a text label |
| Audio waveform / voice activity | ✅ `.waveform`, switches between user and AI |
| Candidate and interviewer speaking indicators | ✅ Two `.speaking-pill`s in a `role="status"` row |
| Session timer | ✅ Counts up, `role="timer"`, pauses during reconnection, "5 minutes left" announcement |
| Question / phase progress | ✅ `.question-progress` dots plus "Qn / N" text |
| Reconnection and recovery state | ✅ `role="alertdialog"` overlay, retry counter, timer paused, recovery restores |
| Optional live captions | ✅ Toggle + caption history drawer |
| Accessible push-to-talk fallback | ✅ Pointer and space-bar hold, announced, with visible instructions |
| End-interview confirmation | ✅ Native dialog, mode-specific consequences |
| Calm, distraction-free layout | ✅ No sidebar, no topbar, dark by default |

### `/admin/recruiter/[candidateId]` — recruiter review detail

| Required inspection | Implemented |
| --- | --- |
| Overall recommendation | ✅ Framed as an evidence summary; the decision is stated to belong to the reviewer |
| Competency scores | ✅ Five ICU competencies with band words |
| Confidence and evidence sufficiency | ✅ Per competency, not only overall |
| Transcript evidence | ✅ Highlighted spans with a legend and an evidence drawer |
| Audio replay | ✅ Simulated player with jump-to-evidence |
| Coverage | ✅ What the conversation reached and what it did not |
| Contradictions | ✅ Both quotes with timestamps, framed neutrally |
| Claim verification | ✅ Marked "unverified — ask directly", with "unverified ≠ untrue" stated |
| JD compatibility | ✅ Requirement-by-requirement, no headline match percentage |
| Missing evidence | ✅ With suggested follow-up questions |
| Reviewer decision | ✅ Advance / Hold / Decline / Request re-review with rationale |
| Override reason | ✅ Required when the decision disagrees with the suggested band |
| Previous review activity | ✅ Who looked, when, what changed |
| Audit history | ✅ Event log; full log link shown only at `god` role |

---

## 7. The two route questions the brief asked us to resolve

**Progression was missing from the candidate sidebar.** Resolved by adding
`/candidate/progression` to the **"Your practice"** navigation group between *Skills* and *Goals*, and
to the mobile bottom navigation as **Progress**. It is additionally reachable from the dashboard
readiness card, the skills page and the review screen's patterns panel.

**`review` and `results` overlapped.** Both are kept with visibly distinct responsibilities —
`results` answers *what happened and what is the evidence*, `review` answers *what do I do next* —
joined by a shared sub-navigation and a one-line explainer on both pages. Neither is orphaned; the
completion screen links to both. Full reasoning in [inferred-screens.md](inferred-screens.md).

---

## 8. Non-route pages

| File | Purpose |
| --- | --- |
| `design-system.html` | Design-system showcase and documentation. Required by the brief; not an application route |
| `screens.html` | Prototype index. Exists because `/` is the marketing page and cannot double as a mockup gallery |
