# Retention and Deletion

**Status:** Architecture defined; schedules require legal/product approval  
**Owner:** Privacy and data governance  
**Last updated:** 2026-08-23

## Principles

- Retention is purpose-, mode-, tenant-, jurisdiction-, and data-class-specific.
- Practice and screening schedules may differ.
- Policy is versioned and snapshotted where it affects consent/session data.
- Deletion is a durable, observable workflow.
- Legal hold/required hiring-record retention overrides only under approved policy.

No universal duration is approved here. Prototype values such as 18 months are assumptions, not policy.

## Asset coverage

Retention/deletion covers identity/profile, documents/extracted facts, invitations, sessions/bundles, transcripts, audio, evaluations/evidence/articulation, progression, reviews/appeals, exports, integrations/outbox, caches, search/vector, analytics, AI datasets, provider copies, and backups.

## Deletion workflow

```text
request → authenticate/authorize → discover scope
→ evaluate holds/exceptions → freeze new processing
→ delete/anonymize primary data → objects → indexes/caches
→ analytics/derived artifacts → provider/downstream requests
→ reconcile → complete with evidence/exception report
```

Requirements:

- idempotent workflow and per-system status;
- requester-visible status and approved SLA;
- no new derivatives after freeze;
- manifest/object reconciliation;
- provider deletion evidence where available;
- backup expiry documented rather than unsafe selective editing;
- minimized audit record retained only where justified.

## Withdrawal

Consent withdrawal stops future consent-dependent processing. Whether already delivered screening evidence can or must be recalled is a jurisdiction-specific legal decision and must be reflected in tenant/candidate communication.

## Legal hold

Record authority, scope, reason, start, reviewer, expiry/review, data affected, and allowed operations. Hold data is access-controlled and excluded from ordinary deletion until release. Candidate response explains exceptions where permitted.

## Retention policy model

```text
policy_version
purpose/mode
jurisdiction/tenant override
data category
duration/start trigger
archive/delete/anonymize action
legal basis
processor/downstream behavior
owner/review date
```

## Operational controls

Monitor approaching expiry, lifecycle job failures, deletion SLA, orphaned objects, provider acknowledgments, backup expiry, and holds past review. Periodically sample completed deletions end to end.

## Open decisions

Exact schedules, controller/processor responsibilities, declined-candidate hiring-record requirements, candidate screening access/withdrawal, audit survival, backup duration, analytics anonymization, and tenant override limits.

