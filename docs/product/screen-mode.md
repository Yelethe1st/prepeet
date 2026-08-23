# Screen Mode

**Status:** Proposed; production requires jurisdiction-specific approval  
**Owner:** Employer product and responsible-hiring governance  
**Last updated:** 2026-08-23

## Purpose

Screen mode enables an employer to invite a candidate to a structured voice interview and gives authorized human reviewers evidence for a hiring process. Prepeet does not decide the outcome.

## Non-negotiable boundaries

- No autonomous advance or rejection.
- No practice-history access by an employer.
- No coaching or redo in the screening experience by default.
- No personality, emotion, honesty, culture-fit, health, or protected-characteristic inference.
- No accent score.
- Articulation is excluded from employer scoring by default.
- Insufficient evidence is not a low candidate result.
- Every reviewer decision is named, reasoned, and auditable.

## Recruiter workflow

1. Create/select a campaign and role.
2. Supply role context or job description.
3. Select a published rubric, calibration, neutral/approved persona, plan, duration, disclosure, and accommodation policy.
4. Preview candidate communication.
5. Issue an expiring invitation.
6. Monitor delivery, acceptance, completion, and evaluation state.
7. Review evidence, coverage, warnings, transcript, and authorized audio.
8. Record `advance`, `hold`, `decline`, or `request_re_review` with rationale.
9. Supply an override reason when disagreeing with the suggested band.
10. Deliver only approved events to an ATS/webhook.

## Candidate workflow

1. Open and validate invitation.
2. Resolve account/identity without exposing another candidate.
3. Review employer, purpose, AI use, recording, data access, retention, result disclosure, and accommodation options.
4. Give required consent.
5. Complete preparation/device checks.
6. Complete the interview with documented recovery policy.
7. Receive durable submission confirmation and permitted status only.

Candidate visibility into the evaluation is an open legal/product decision by jurisdiction. Route guards and API policy must enforce the selected disclosure; the UI must not rely on hiding links.

## Recruiter review requirements

- Session configuration and artifact versions.
- Overall evidence summary, never framed as the decision.
- Competencies with anchors, coverage, sufficiency, and defensible uncertainty.
- Evidence spans and audio timestamps.
- Neutral contradictions and unverified claims.
- Job requirements as evidenced/partial/not discussed/not assessable, not a match percentage.
- Missing evidence and suggested human follow-up.
- Previous reviewer activity and audit history.

Whether the suggested band appears before evidence is reviewed remains open because of anchoring risk.

## Comparison

Candidate comparison is off until explicitly approved. If enabled, it is limited to two to four candidates for the same role and comparable rubric, shows uncertainty and coverage, states when differences are indistinguishable, and never rank-orders candidates.

## Accommodation and incidents

- Captions, push-to-talk, and extra thinking time do not change scoring.
- Response latency is excluded from evaluation.
- Alternative assessment paths exist when voice is inaccessible.
- Connection or device failure may trigger re-invitation through a human-governed policy.
- The system records interruptions and coverage rather than silently treating missing data as poor performance.

## Re-review and appeals

The system supports a frozen original evidence/configuration set, eligibility, request reason, assigned reviewer, SLA, independent review where required, outcome, permitted candidate disclosure, and append-only history.

Whether appeal is a right or feature is open and must be settled before launch.

## Launch prerequisites

- Legal/privacy review for each jurisdiction.
- Candidate disclosure and consent approved and versioned.
- Tenant isolation independently tested.
- Supported language/accent/audio-quality limits documented.
- Human review, incident, accommodation, and re-review paths operational.
- Audit and sensitive-read monitoring operational.
- Evaluation quality and responsible-hiring release criteria met.

