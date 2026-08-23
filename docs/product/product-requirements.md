# Product Requirements

**Status:** Proposed baseline  
**Owner:** Product  
**Last updated:** 2026-08-23

## Product definition

Prepeet is a multi-tenant, AI-powered, voice-first interview platform serving candidates across professions. It has two modes:

- **Practice:** candidate-owned preparation with coaching, redos, articulation improvement, readiness, and progression.
- **Screen:** employer-configured interviews producing evidence for named human reviewers without making hiring decisions.

## Product principles

1. Every material evaluation traces to evidence.
2. Unknown, insufficient, and unassessable are not low performance.
3. A human owns every hiring decision.
4. Candidate practice history never leaks into employer screening.
5. Sessions pin all configuration and intelligence artifacts.
6. Coaching is specific, actionable, and fact-preserving.
7. Accent, personality, emotion, honesty, health, and protected characteristics are not scored or inferred.
8. Accessibility and accommodations are correctness requirements.
9. Confidence and coverage are visible without false precision.
10. Consent, recording, retention, and employer access use plain language.

## Actors

| Actor | Goal |
|---|---|
| Practice candidate | Improve interview content and delivery privately |
| Screening candidate | Complete a fair, accessible, disclosed employer interview |
| Recruiter | Configure screens and review authorized evidence |
| Hiring reviewer | Inspect evidence and record a reasoned human decision |
| Tenant administrator | Manage members, configuration, retention, quota, and integrations |
| Platform operator | Maintain the service under least privilege |
| Content specialist | Author, test, approve, and publish interview artifacts |

## Required capabilities

### Identity and tenancy

- Registration, verification, login, logout, recovery, magic link, OTP, and configured OAuth.
- Multiple tenant memberships with explicit active-tenant context.
- Candidate, recruiter, tenant-admin, platform, and service authority boundaries.
- Enterprise SSO/SCIM extension path without requiring it for the first release.

### Candidate profile

- Profile, CV, job context, goals, target roles, preferences, consent, and accessibility settings.
- Structured extraction with source provenance, confidence, and candidate correction.
- Private practice evidence bank excluded from employer access.
- Data export, correction, retention visibility, and deletion request.

### Interview creation and preparation

- Discipline, role, seniority, shape, duration, persona/style, pressure, CV/JD, and accommodations.
- Server-provided catalogs rather than hardcoded technology roles.
- Immutable session composition before start.
- Consent, microphone, speaker, network, captions, push-to-talk, and device checks.

### Live interview

- Browser-to-provider WebRTC media.
- Visible phase, progress, timer, interviewer, speaking, microphone, recording, and connection state.
- Captions, caption history, push-to-talk, reduced motion, and keyboard operation.
- Recovery from refresh, duplicate tab, device loss, network change, event replay, and provider interruption.
- Explicit completion with durable receipt.

### Evaluation and learning

- Per-turn and whole-session evidence-linked evaluation.
- Competency coverage, sufficiency, claims, contradictions, and job-requirement evidence.
- Articulation profile for structure, conciseness, fluency, pace, pausing, precision, signposting, intelligibility, delivery, and responsiveness.
- Fact-preserving coaching, drills, redos, and original-versus-redo comparison.
- Role-specific readiness, skills, goals, and longitudinal progression.

### Recruiting

- Campaigns, invitations, disclosure, consent, accommodation, delivery, expiry, and revocation.
- Evidence-first candidate review with transcript/audio access controlled and audited.
- Human decisions, override rationale, review history, and re-review/appeals.
- Tenant rubric/calibration publication and immutable version history.
- Signed webhooks and future ATS adapters.

### Platform operations

- Aggregate analytics, session/realtime health, workflow backlogs, evaluation quality, provider usage/cost, quota control, elevated operations, and privileged audit.

## Detailed functional requirements

### Public and authentication

The system shall provide a public site that explains practice and screening without conflating them; candidate and organization registration; password and configured identity-provider login; logout, refresh, email verification, magic link, OTP, and recovery; safe preservation of a post-login destination; and distinct unauthenticated, no-workspace, forbidden, expired, not-found, and system-error states.

Authentication flows must prevent open redirects, token leakage, account enumeration, and invitation confusion. A user may belong to several tenants, but every request operates under one explicit active tenant. Enterprise SSO and SCIM are deferred until demand is validated.

### Candidate profile and documents

Candidates shall be able to upload, replace, inspect, and delete a CV; manage disciplines, roles, seniority, career context, goals, preferences, and accommodations; inspect and correct extracted facts; and understand which source span produced each fact.

Each document and extracted fact retains content digest, version, provenance, confidence, purpose, consent, correction state, and processing status. Deleting or replacing a document must not silently rewrite the configuration of an already-started session.

### Interview creation

Practice creation collects discipline, role family, role, seniority, interview shape, duration, interviewer style where permitted, optional CV/JD, pressure preference, accommodations, recording preference, and consent. Available combinations come from server metadata with explicit validation and duration limits.

Screen creation lets an authorized recruiter choose a campaign/role, published configuration, job context, candidate disclosure, accommodation policy, invitation expiry, and communication template. Recruiters preview consequences before issue or revocation.

### Session preparation

Before start, show mode, purpose, role, shape, duration, interviewer, consent/recording status, accommodations, microphone selection/input, speaker output where relevant, network/provider connectivity, captions, push-to-talk, expiry, and what happens after completion.

Start is blocked when mandatory consent or checks are incomplete. Failing validation moves focus to the first problem and preserves previously entered data.

### Live interview

The live screen shall provide current phase/question progress without exposing hidden evaluation policy; interviewer identity/style; connection, microphone, recording, caption, and speaker state through text plus visual indicators; mute and push-to-talk; caption history; elapsed active time; reconnection; and an explicit mode-aware end confirmation.

It must safely handle browser refresh, duplicate tabs, sleep/wake, network handoff, device removal, stale credentials, duplicate/out-of-order events, partial transcript correction, provider interruption, and session expiry. Reconnection pauses candidate time under the approved policy.

### Completion and processing

After completion, show durable receipt and processing stages without promising an exact completion time. Distinguish queued, delayed, partial, recoverable failure, terminal failure, insufficient evidence, and complete. The user can navigate away and later return.

Screen candidates receive confirmation/status only unless an approved disclosure policy grants more. Practice candidates are notified when results/review are ready.

### Practice results and review

Practice shall expose overall evidence summary, coverage, competencies, sufficiency, per-turn feedback, transcript, authorized audio, claims, contradictions, stronger structures, articulation, drills, redo, next-session recommendations, and readiness/progression updates.

`results` is proposed to answer what happened and what evidence supports it; `review` is proposed to answer what to improve. Usability research decides whether they remain separate.

### Skills, progression, and goals

Candidates can inspect competency evidence, assessed/unassessed status, trend, evidence freshness, target-role readiness, goals, milestones, and practice cadence. They can start a session targeting a gap. Streaks may encourage practice but must not become punitive gamification.

### Recruiter review and comparison

Review includes candidate/invitation context, pinned configuration, evidence summary, competency anchors, coverage, sufficiency, uncertainty, transcript/audio, neutral contradictions and unverified claims, job-requirement checklist, missing evidence, suggested human follow-ups, prior activity, and audit.

If comparison is approved, restrict it to two to four candidates for one role and comparable rubric, show uncertainty/coverage, state indistinguishable differences, avoid ranking, and require individual evidence inspection before decision.

### Tenant administration

Tenant administrators manage organization settings, branding, members and scoped roles, role/rubric library, calibration drafts/publication/history/impact preview, disclosures, accommodations, retention, notifications, quotas/billing visibility, API keys, ATS/webhooks, and delivery history.

Published configuration is immutable. Updates create a new version and never mutate in-flight or historical sessions.

### Appeals and re-review

Support candidate-initiated and system-flagged review with eligibility, reason, evidence, owner, SLA, status, independent assignment where required, response, disclosure, and append-only audit. Legal/product governance decides whether it is a right, global policy, or tenant option.

### Platform administration

Operators require privacy-controlled aggregate analytics, realtime/session health, workflow backlog and failed-work recovery, evaluation quality and artifact/model versions, provider usage/cost, quota control, time-bound elevated operations, and privileged audit. Cross-tenant access is exceptional rather than ordinary navigation.

## Non-functional requirements

- WCAG 2.2 AA plus real assistive-technology and disabled-user testing.
- Candidate journeys mobile-first from 320px.
- Tenant isolation across API, SQL, objects, workflows, caches, analytics, and telemetry.
- No restricted content in ordinary logs, traces, metrics, or error reporting.
- Important operations are idempotent, observable, auditable, and recoverable.
- English/en-GB first, with localization-ready content and formatting.

## Success measures

Practice: completion, review usage, redo/drill use, targeted improvement, usefulness, and retention from learning value.

Screen: invitation completion, insufficiency rate, evidence inspection, time to human decision, override/re-review patterns, accommodation parity, incidents, and webhook reliability.

Platform: journey SLOs, security findings, AI invalid-output/unsupported-fact rate, cost per completed session, and deletion/retention SLA.

## Highest-risk open decisions

1. Screening candidate disclosure and access rights by jurisdiction.
2. Appeals as right, tenant option, or platform policy.
3. Defensible confidence semantics.
4. Supported language/accent matrix and transcript-quality thresholds.
5. Screening reconnect, pause, restart, and re-invitation.
6. Evidence-first versus band-first recruiter review.
7. Retention conflicts and legal hold.
8. Billing unit and quota behavior.
9. Shared versus separate practice/screen brand.
10. Candidate comparison approval.
