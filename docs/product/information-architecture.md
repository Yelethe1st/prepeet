# Information Architecture

**Status:** Proposed from high-fidelity mockups  
**Owner:** Product design and frontend  
**Last updated:** 2026-08-23

## Navigation

### Candidate practice

```text
Prepare: Dashboard · Start an interview
Your practice: Sessions · Skills · Progression · Goals
Account: Profile and CV · Settings
```

Proposed mobile navigation: Home, Sessions, Practise, Progress, You.

Screen candidates receive invitation-scoped navigation only: consent, preparation, live session, completion/status, help/privacy, and necessary account actions.

### Tenant/recruiter

```text
Hiring: Overview · Campaigns · Candidates · Compare · Invitations · Re-review
Tenant configuration: Calibration · Rubrics · Members · Integrations · Settings
```

### Platform

```text
Platform: Analytics · Sessions · Evaluation quality · System · Model usage · Quotas
Privileged: Elevated operations · Audit
```

Navigation is capability-driven, but Go authorization remains authoritative.

## Route map

### Public/authentication

`/`, `/login`, `/register`, `/oauth/callback`, `/forgot-password`, `/check-email`, `/reset-password`, `/verify-email`, `/auth/magic`, `/auth/otp`, `/auth/expired`, `/access-denied`, `/invite/[token]`.

### Candidate

| Route | Responsibility |
|---|---|
| `/candidate` | Resolve first-run, returning, or invited destination |
| `/candidate/dashboard` | Practice readiness or screen status without leakage |
| `/candidate/start-interview` | Practice configuration wizard |
| `/candidate/session/[id]/prepare` | Brief, consent, accommodations, devices |
| `/candidate/session/[id]` | Live interview |
| `/candidate/session/[id]/complete` | Receipt and processing state |
| `/candidate/session/[id]/results` | Practice outcome, evidence, transcript, audio |
| `/candidate/session/[id]/review` | Practice coaching, drills, redo |
| `/candidate/session/[id]/articulation` | Delivery measurement, dimensions, drills, original/redo comparison |
| `/candidate/sessions` | Session history and status-aware navigation |
| `/candidate/skills` | Competency evidence |
| `/candidate/progression` | Longitudinal trends |
| `/candidate/goals` | Targets and cadence |
| `/candidate/profile` | Profile, CV, private evidence |
| `/candidate/settings` | Account, accessibility, privacy, defaults |

The `results`/`review`/`articulation` split is proposed: results answers “what happened and why”; review answers “what should I improve about the content”; articulation answers “how did it land.” They share one sub-navigation. Consolidate if research shows confusion.

Screen candidates receive invitation-scoped navigation only — status, invitation and consent, help/privacy, and account actions. Practice destinations are not rendered for them, and each practice page also refuses the mode.

### Recruiter/tenant

`/admin/dashboard`, `/admin/campaigns`, `/admin/campaigns/[id]`, `/admin/recruiter`, `/admin/recruiter/[candidateId]`, `/admin/recruiter/compare`, `/admin/recruiter/calibration`, `/admin/recruiter/appeals`, `/admin/invitations`, `/admin/invitations/new`, `/admin/invitations/[id]`, `/admin/members`, `/admin/settings`, `/admin/rubrics`, `/admin/integrations`.

### Platform/system

`/admin/platform/analytics`, `/admin/platform/analytics/sessions`, `/admin/platform/analytics/evaluations`, `/admin/platform/system`, `/admin/platform/system/llm-usage`, `/admin/platform/quotas`, `/admin/god`, `/admin/god/audit`, plus `/403`, `/404`, and `/500`.

## Page composition rules

- Candidate pages are calm, mobile-first, and present one primary action.
- Recruiter pages are evidence-first and show uncertainty with every material score.
- Platform pages are dense, operational, and optimized for scanning.
- Color never carries status or score alone.
- Charts include text summaries and table alternatives.
- Tables adapt to cards when rows are independent; comparison grids scroll with explanation.
- Loading skeletons match the final shape; errors preserve safe content and recovery.
- Mode and role boundaries are visible but never leak inaccessible data.

## Accessibility

Target WCAG 2.2 AA: skip links, landmarks, logical headings, visible focus, full keyboard operation, labeled forms, status live regions, reduced motion, 24px minimum controls and 44px primary candidate controls, accessible dialogs, captions, push-to-talk, chart alternatives, and screen-reader validation.

## Content rules

- Candidate: supportive, specific, and non-scolding.
- Recruiter: observable evidence and uncertainty, not character judgment.
- Platform: concise operational nouns, numbers, and actions.
- “Unverified” does not mean false.
- “Contradiction” means clarification is needed.
- “Insufficient evidence” describes coverage, not candidate quality.

