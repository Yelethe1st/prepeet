# Release Criteria

**Status:** Required evidence framework  
**Owner:** Cross-functional release authority  
**Last updated:** 2026-08-23

## Foundation

- [ ] Reproducible Next.js/Go/Python builds and immutable deploys.
- [ ] OpenAPI/Protobuf lint, generation, drift, and compatibility gates.
- [ ] Module boundaries and forbidden imports enforced.
- [ ] Empty/prior-version migrations pass; RLS blocks cross-tenant reads/writes/lists.
- [ ] Workload identities/secrets least-privileged.
- [ ] Temporal restart/replay safe.
- [ ] Distributed trace spans complete journey without restricted content.
- [ ] Rollback and database restore demonstrated.
- [ ] Artifact schema/digest/publication/rollback flow demonstrated.
- [ ] Restricted-content telemetry scanner passes.
- [ ] Object upload/finalization authorization and reconciliation pass.

## Practice

- [ ] Full candidate journey and documented states pass.
- [ ] Coaching cites evidence and preserves facts.
- [ ] Articulation distinguishes assessability and provides relevant drills.
- [ ] Redo preserves history; progression keeps unknown distinct.
- [ ] Realtime duplicate/order/gap/refresh/reconnect/device/tab/provider tests pass.
- [ ] Journey SLOs pass representative/burst load.
- [ ] Evidence, insufficiency, unsupported-fact, contradiction, schema, latency, and cost AI gates pass.
- [ ] Supported professions/accents/devices/speech differences/audio conditions tested.
- [ ] WCAG 2.2 AA, keyboard, VoiceOver and NVDA/JAWS, and disabled-user testing pass.
- [ ] Consent, export, deletion, retention, and model-improvement opt-in work end to end.
- [ ] Threat/security scans/review/penetration findings meet policy.
- [ ] Loading, empty, partial, forbidden, expired, delayed, insufficient, unassessable, reconnecting, and degraded states are accessible and tested.
- [ ] Original and corrected transcript sequencing retains provenance.
- [ ] Model/prompt/artifact changes have evaluation report, approval, monitoring, and rollback.
- [ ] Cost per created/started/completed/review-ready session is measured.

## Screening

- [ ] Launch jurisdictions legally/privacy approved.
- [ ] Candidate disclosure/consent versioned and approved.
- [ ] Practice data unreachable from tenant authority.
- [ ] Hidden evaluation/coaching/evidence inaccessible to candidate unless approved disclosure grants it.
- [ ] Recruiter access campaign/tenant scoped and audited.
- [ ] Evaluation exposes evidence/coverage/insufficiency/warnings.
- [ ] Named human owns decision; no automatic outcome path.
- [ ] Override/history append-only; appeals/accommodations operational.
- [ ] Articulation excluded unless separately approved and validated.
- [ ] Supported language/accent/transcription limits published.
- [ ] Calibration immutable/versioned.
- [ ] Webhook forgery/replay/SSRF/retry/dedup tests pass.
- [ ] Independent isolation and penetration tests pass.
- [ ] Integrity freeze/impact/re-review exercise complete.
- [ ] Comparison off unless explicitly approved.
- [ ] Sensitive transcript/audio/evaluation reads are audited.
- [ ] Reviewer decision and integration delivery preserve true actor, evidence version, and idempotency.
- [ ] Candidate interruption/accommodation/re-invitation policy is exercised.
- [ ] Retention/legal hold and candidate data-rights procedures are approved.

## Operational readiness

- [ ] Owners/on-call, SLO/error budget, dashboards, alerts, and runbooks.
- [ ] Capacity/headroom and cost per completed session measured.
- [ ] Backup restore, key rotation, provider outage, workflow recovery exercised.
- [ ] Data lifecycle/deletion reconciled and monitored.
- [ ] Support elevation/audit and incident communication operational.

## Stop-ship

Cross-tenant or practice/screen leak; uncited material evaluation; model bypass of Go validation; invalid consent; insufficient/unassessable collapsed into poor; accent/protected/personality inference; inaccessible core path without equivalent; failed recovery evidence; restricted content in telemetry; or legal/security rejection.

## Release record

Record release scope/tenants/modes, image and migration digests, artifact/model/contract versions, flags, test/AI reports, approvals, SLO/capacity evidence, risks/exceptions, rollback/freeze plan, and approvers/time.
