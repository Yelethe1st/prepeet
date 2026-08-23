# Implementation Roadmap

**Status:** Proposed  
**Owner:** Principal Engineer and product leadership  
**Last updated:** 2026-08-23

The phase-by-phase plan below is broken into 161 tickets in [tickets/](tickets/README.md).

## Principles

Ship vertical journeys, retire high-risk boundaries early, deliver practice before screening, include security/accessibility/observability/AI evaluation with each feature, and treat mockups as hypotheses until validated.

## Phase 0: Validate and decide

Approve scope/mode boundary; validate mockups with candidates/recruiters/disabled users/operators; assign open decisions; create domain map, quality scenarios, capacity assumptions, classification, threat model, and initial ADRs.

Exit criteria:

- product, engineering, security, privacy, and responsible-hiring owners approve boundaries;
- high-risk assumptions have owner, decision date, and fallback scope;
- first region/provider/operating ownership is known;
- screening work is explicitly gated by legal/product decisions.

Exit: no silent unresolved decision can change foundational topology or screening legality.

## Phase 1: Foundation

Monorepo, Next.js/Go/Python, contracts/codegen, CI, local/cloud environments, PostgreSQL/RLS, S3, Temporal, secrets, telemetry, identity/tenant context, service identity, and artifact publication skeleton.

Exit: traced authenticated request and durable Go→Python workflow deploy/rollback/recovery.

Additional exit evidence: contract compatibility and module boundaries enforced; production-like RLS roles tested; backup restore and secret rotation demonstrated outside production; one artifact can be reviewed, published, pinned, and rolled back.

## Phase 2: Walking skeleton

```text
register → personal tenant → short practice configuration
→ pinned bundle → device check → realtime connection
→ one answer/transcript → complete/finalize
→ evidence-linked evaluation → review
```

Include duplicate/restart/reconnect and insufficient-evidence fixture.

Exit criteria:

- one trace crosses browser, Go, workflow, Python, provider, PostgreSQL, and object storage;
- duplicate requests and worker restarts do not duplicate state/usage/notification;
- completed review reconstructs pinned inputs and evidence;
- unauthorized practice/screen and cross-tenant reads fail.

## Phase 3: Practice MVP

Profile/CV/JD/correction/consent; complete composition/runtime; captions/push-to-talk/reconnect/media; evaluation; transcript articulation; results/review/redo; sessions/skills/goals/progression/readiness; privacy/delete/export; responsive/assistive testing.

Exit criteria: representative multi-discipline AI quality, latency, and cost thresholds pass; live/review SLOs are measured; candidate privacy/deletion and accessibility gates pass; production support/runbooks are ready.

## Phase 4: Practice hardening

Audio articulation, personal baselines, diverse AI datasets, SLO/load/cost/provider degradation, and research-driven IA/coaching changes.

Exit criteria: deterministic audio fixtures, diverse-user quality analysis, provider degradation, burst/load, restore, and candidate research support expansion.

## Phase 5: Screening foundation

Campaigns/invitations/disclosure/accommodations; scoped tenant roles; calibration publication; recruiter evidence/human decisions; appeals/audit; retention/usage/quota; signed webhooks. Restricted flags only.

Exit criteria: named jurisdiction, tenant-isolation, security, privacy, accessibility, responsible-hiring, human-review, appeal, retention, and integration gates approve a limited pilot.

## Phase 6: Screening pilot

Named tenants/roles/jurisdictions, legal/security/responsible-AI approval, quality/insufficiency/transcription/override/appeal monitoring, ATS pilot, incident support, independent isolation/penetration test. Comparison remains off absent approval.

Exit criteria: measured pilot evidence supports expansion, integrity incidents can be frozen/reviewed, and candidate/tenant support plus operational budgets are sustainable.

## Phase 7: Enterprise/scale

SSO/SCIM, advanced scopes, residency/legal hold, provider redundancy, analytics/search specialization, additional languages/mobile, and justified service/Kubernetes extraction.

## First 90 days

- Days 1–30: validate, ADRs/threat/domain/capacity, repo/contracts/CI/environments, observable deployables.
- Days 31–60: identity/tenant/auth harness, session/bundle/artifacts/composition, realtime/device state, recovery/uploads.
- Days 61–90: short practice journey, transcript/media, evidence review, transcript articulation, duplicate/restart/reconnect/delete/restore demonstrations.

## Workstreams

Product/design, web, Go control plane, Python intelligence, platform, security/privacy, AI/data quality, and integrations.

| Workstream | Core responsibility |
|---|---|
| Product/design | Requirements, research, prototype, content, accessibility, disclosures |
| Web | Next.js shell, journeys, design system, realtime browser/media |
| Go | Modules, APIs, lifecycle, authorization, persistence, audit/integrations |
| Python | Composition, runtime policy, evaluation, articulation, providers, AI evals |
| Platform | Environments, PostgreSQL, Temporal, storage, telemetry, CI/CD |
| Security/privacy | Threats, identity, isolation, consent, retention/deletion, incidents |
| AI/data quality | Datasets, human benchmarks, calibration, quality monitoring |
| Integrations | Email, webhooks, ATS, usage/billing adapters |

## Required initial ADRs

Hosting/regions, identity, modular-monolith boundary, Temporal, PostgreSQL/RLS, realtime/media provider, recording, contracts/events, artifact registry, model routing/fallback/budgets, screening disclosure/appeals, and Next.js/Go boundary.

## Principal risks

| Risk | Early retirement |
|---|---|
| Realtime/network unreliability | Walking skeleton and reconnect/device tests |
| Untrustworthy evaluation | Evidence contract, insufficiency, human datasets |
| Accent/disability disadvantage | Assessability, supported matrix, accommodations, diverse testing |
| Practice/screen leakage | Separate purpose projections and adversarial authorization tests |
| Regulatory mismatch | Jurisdiction decision before screening build/launch |
| AI cost/backlog | Durable workflows, budgets, capacity/cost model |
| Premature distributed complexity | Three deployables and extraction criteria |
| Mockups treated as requirements | Status labels, research, acceptance evidence |
