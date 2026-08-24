# Implementation tickets

**Status:** Proposed backlog  
**Owner:** Principal Engineer and delivery leads  
**Last updated:** 2026-08-23

Every piece of work the specification implies, broken into 165 tickets across 22 epics. This is the
delivery view of the same system [implementation-roadmap.md](../implementation-roadmap.md) describes in
phases and [dependency-map.md](../dependency-map.md) describes as a critical path.

## How to read a ticket

Each ticket has an identifier, a title that states the work rather than naming a component, the tickets
it depends on and unblocks, a short statement of what it is and why it exists, a **Done when** checklist,
and a link to the specification it implements.

A ticket is done when its checklist is satisfied and the linked specification is still true — if the work
proved the specification wrong, updating the specification is part of the ticket.

## Definition of done

These apply to every ticket in this backlog, in addition to its own **Done when** list. A ticket that
satisfies its checklist but not these is not finished.

- **Test first.** The failing test is written before the implementation and ships in the same change.
  No implementation is merged without a test covering it.
- **Full coverage on both sides.** The Next.js frontend is held to the same standard as the Go and
  Python services. Coverage thresholds are enforced in CI for every deployable, and a drop fails the
  build. See [PLT-10](02-platform-foundation.md#plt-10--build-the-test-harness-and-coverage-gates-for-all-three-deployables).
- **Comprehensive, not happy path.** Failure modes, edge cases and the invariants the specification
  names each need a test. For this product that includes cross tenant attempts, insufficient evidence,
  unassessable input, reconnection and duplicate delivery.
- **Documented.** Every exported type, function, endpoint, workflow and component carries documentation
  that says which rule it enforces and which invariant it protects. Documentation completeness is a
  build gate. See [PLT-11](02-platform-foundation.md#plt-11--establish-and-enforce-code-documentation-standards).
- **Ported, not redesigned.** UI work carries the design across from the HTML prototype in
  [`/screens`](../../../screens) rather than reinterpreting it, including its copy and its states. See
  [WEB-06](05-web-foundation.md#web-06--port-every-prototype-screen-to-nextjs-with-verified-parity).
- **Accessible.** A candidate facing surface is not done until it is operable by keyboard and screen
  reader, because an inaccessible path excludes someone from a hiring process.

## Conventions

- **Identifiers** are `EPIC-NN`, stable once issued. A cancelled ticket keeps its number.
- **Done when** items are acceptance criteria, not a task list. They describe observable behaviour.
- **Depends on** names tickets, never people or teams.
- Tickets marked *Gap found against the prototype* close something the specification requires that no
  screen implemented. Tickets marked *Implemented in the prototype* have a design to build from in
  [`/screens`](../../../screens), not a finished implementation.
- Nothing in epic SCR or REV ships to a real candidate before **DEC-11** is answered.

## Epics

| Epic | Title | Phase | Tickets |
|---|---|---|---|
| [DEC](01-decisions-and-adrs.md) | Decisions and ADRs | 0 | 18 |
| [PLT](02-platform-foundation.md) | Platform foundation | 1 | 11 |
| [CTR](03-contracts-and-codegen.md) | Contracts and code generation | 1 | 4 |
| [IAM](04-identity-and-authorization.md) | Identity, tenancy and authorization | 1–2 | 7 |
| [WEB](05-web-foundation.md) | Design system and application shell | 1–2 | 6 |
| [PRO](06-candidate-profile.md) | Candidate profile and documents | 3 | 5 |
| [CAT](07-catalog-and-composition.md) | Catalogue, artifacts and session composition | 2–3 | 6 |
| [SES](08-session-lifecycle.md) | Session lifecycle and orchestration | 2–3 | 8 |
| [RTC](09-realtime-and-media.md) | Realtime, media and transcript integrity | 2–4 | 7 |
| [EVL](10-evaluation-system.md) | Evaluation and evidence | 3 | 7 |
| [ART](11-articulation.md) | Delivery and articulation | 3–4 | 7 |
| [PRC](12-practice-experience.md) | Practice results, review and coaching | 3 | 6 |
| [PRG](13-progression-and-goals.md) | Skills, progression, goals and readiness | 3 | 5 |
| [SCR](14-screening-and-invitations.md) | Screening, campaigns and invitations | 5 | 9 |
| [REV](15-recruiter-review-and-appeals.md) | Recruiter review, decisions and appeals | 5–6 | 8 |
| [TEN](16-tenant-administration.md) | Tenant administration | 5 | 8 |
| [INT](17-integrations.md) | Email, webhooks and ATS integration | 3 and 5 | 6 |
| [OPS](18-platform-operations.md) | Platform operations and internal consoles | 3–6 | 7 |
| [SEC](19-security-and-privacy.md) | Security, privacy and data rights | 0–5, continuous | 10 |
| [A11Y](20-accessibility.md) | Accessibility and inclusive content | 1–4, continuous | 7 |
| [QUA](21-ai-quality.md) | AI quality, datasets and monitoring | 2–6, continuous | 6 |
| [REL](22-release-readiness.md) | Release readiness and operational proof | 3, 5 and 6 gates | 7 |

**165 tickets** in total.

## Suggested order

Epic DEC runs first and never entirely stops. PLT, CTR, IAM and WEB are the foundation everything else
waits on. PRO through PRG deliver practice, which ships before screening. SCR and REV deliver screening
and are gated on legal approval. SEC, A11Y and QUA run continuously beside the feature epics rather than
after them, and REL turns each gate into evidence.

## Full index

### DEC — Decisions and ADRs

- **[DEC-01](01-decisions-and-adrs.md#dec-01--choose-the-hosting-platform-and-regional-topology)** · Choose the hosting platform and regional topology
- **[DEC-02](01-decisions-and-adrs.md#dec-02--decide-whether-identity-is-built-or-bought)** · Decide whether identity is built or bought
- **[DEC-03](01-decisions-and-adrs.md#dec-03--fix-the-go-modular-monolith-boundary-and-extraction-criteria)** · Fix the Go modular-monolith boundary and extraction criteria
- **[DEC-04](01-decisions-and-adrs.md#dec-04--decide-temporal-hosting-and-workflow-ownership)** · Decide Temporal hosting and workflow ownership
- **[DEC-05](01-decisions-and-adrs.md#dec-05--decide-postgresql-schema-layout-rls-strategy-and-connection-roles)** · Decide PostgreSQL schema layout, RLS strategy and connection roles
- **[DEC-06](01-decisions-and-adrs.md#dec-06--choose-the-realtime-provider-media-topology-and-outage-fallback)** · Choose the realtime provider, media topology and outage fallback
- **[DEC-07](01-decisions-and-adrs.md#dec-07--decide-the-recording-source-format-alignment-and-retention)** · Decide the recording source, format, alignment and retention
- **[DEC-08](01-decisions-and-adrs.md#dec-08--fix-rest-rpc-event-and-generated-contract-conventions)** · Fix REST, RPC, event and generated-contract conventions
- **[DEC-09](01-decisions-and-adrs.md#dec-09--decide-the-artifact-registry-review-publication-and-rollback-model)** · Decide the artifact registry, review, publication and rollback model
- **[DEC-10](01-decisions-and-adrs.md#dec-10--decide-model-providers-routing-fallback-and-budgets)** · Decide model providers, routing, fallback and budgets
- **[DEC-11](01-decisions-and-adrs.md#dec-11--settle-screening-disclosure-candidate-access-and-appeal-rights-per-jurisdiction)** · Settle screening disclosure, candidate access and appeal rights per jurisdiction
- **[DEC-12](01-decisions-and-adrs.md#dec-12--define-what-confidence-and-coverage-mean-and-what-they-may-not-imply)** · Define what confidence and coverage mean, and what they may not imply
- **[DEC-13](01-decisions-and-adrs.md#dec-13--publish-the-supported-language-accent-and-audio-quality-matrix)** · Publish the supported language, accent and audio-quality matrix
- **[DEC-14](01-decisions-and-adrs.md#dec-14--decide-reconnect-pause-restart-and-re-invitation-policy-for-screening)** · Decide reconnect, pause, restart and re-invitation policy for screening
- **[DEC-15](01-decisions-and-adrs.md#dec-15--decide-retention-schedules-legal-hold-precedence-and-deletion-exceptions)** · Decide retention schedules, legal hold precedence and deletion exceptions
- **[DEC-16](01-decisions-and-adrs.md#dec-16--decide-the-billing-unit-quota-behaviour-and-overage-messaging)** · Decide the billing unit, quota behaviour and overage messaging
- **[DEC-17](01-decisions-and-adrs.md#dec-17--decide-whether-candidate-comparison-ships-and-under-what-constraints)** · Decide whether candidate comparison ships, and under what constraints
- **[DEC-18](01-decisions-and-adrs.md#dec-18--decide-the-shared-brand-question-for-practice-and-screening)** · Decide the shared-brand question for practice and screening

### PLT — Platform foundation

- **[PLT-01](02-platform-foundation.md#plt-01--stand-up-the-monorepo-with-nextjs-go-and-python-deployables)** · Stand up the monorepo with Next.js, Go and Python deployables
- **[PLT-02](02-platform-foundation.md#plt-02--build-the-ci-pipeline-with-contract-boundary-and-security-gates)** · Build the CI pipeline with contract, boundary and security gates
- **[PLT-03](02-platform-foundation.md#plt-03--provision-postgresql-with-row-level-security-and-least-privilege-roles)** · Provision PostgreSQL with row-level security and least-privilege roles
- **[PLT-04](02-platform-foundation.md#plt-04--enforce-module-boundaries-and-forbidden-imports-in-the-go-control-plane)** · Enforce module boundaries and forbidden imports in the Go control plane
- **[PLT-05](02-platform-foundation.md#plt-05--provision-object-storage-with-scoped-upload-and-playback-authorization)** · Provision object storage with scoped upload and playback authorization
- **[PLT-06](02-platform-foundation.md#plt-06--stand-up-temporal-with-restart-safe-workers-and-a-deployment-story)** · Stand up Temporal with restart-safe workers and a deployment story
- **[PLT-07](02-platform-foundation.md#plt-07--establish-secret-management-and-workload-identity)** · Establish secret management and workload identity
- **[PLT-08](02-platform-foundation.md#plt-08--instrument-distributed-tracing-metrics-and-structured-logging)** · Instrument distributed tracing, metrics and structured logging
- **[PLT-09](02-platform-foundation.md#plt-09--build-environment-provisioning-and-immutable-deploy-with-rollback)** · Build environment provisioning and immutable deploy with rollback
- **[PLT-10](02-platform-foundation.md#plt-10--build-the-test-harness-and-coverage-gates-for-all-three-deployables)** · Build the test harness and coverage gates for all three deployables
- **[PLT-11](02-platform-foundation.md#plt-11--establish-and-enforce-code-documentation-standards)** · Establish and enforce code documentation standards

### CTR — Contracts and code generation

- **[CTR-01](03-contracts-and-codegen.md#ctr-01--define-the-public-rest-contract-and-generate-the-typescript-client)** · Define the public REST contract and generate the TypeScript client
- **[CTR-02](03-contracts-and-codegen.md#ctr-02--define-the-gopython-rpc-contract-and-generate-both-sides)** · Define the Go↔Python RPC contract and generate both sides
- **[CTR-03](03-contracts-and-codegen.md#ctr-03--define-the-durable-event-catalogue-and-envelope)** · Define the durable event catalogue and envelope
- **[CTR-04](03-contracts-and-codegen.md#ctr-04--add-contract-drift-compatibility-and-consumer-tests-to-ci)** · Add contract drift, compatibility and consumer tests to CI

### IAM — Identity, tenancy and authorization

- **[IAM-01](04-identity-and-authorization.md#iam-01--implement-registration-login-logout-and-session-refresh)** · Implement registration, login, logout and session refresh
- **[IAM-02](04-identity-and-authorization.md#iam-02--implement-email-verification-password-recovery-magic-link-and-otp)** · Implement email verification, password recovery, magic link and OTP
- **[IAM-03](04-identity-and-authorization.md#iam-03--implement-tenant-membership-and-explicit-active-tenant-context)** · Implement tenant membership and explicit active-tenant context
- **[IAM-04](04-identity-and-authorization.md#iam-04--build-the-capability-catalogue-and-policy-evaluation-service)** · Build the capability catalogue and policy evaluation service
- **[IAM-05](04-identity-and-authorization.md#iam-05--implement-the-tenant-and-workspace-switcher-in-the-web-application)** · Implement the tenant and workspace switcher in the web application
- **[IAM-06](04-identity-and-authorization.md#iam-06--enforce-practice-and-screening-authority-separation)** · Enforce practice and screening authority separation
- **[IAM-07](04-identity-and-authorization.md#iam-07--implement-time-bound-platform-elevation-with-reason-and-audit)** · Implement time-bound platform elevation with reason and audit

### WEB — Design system and application shell

- **[WEB-01](05-web-foundation.md#web-01--implement-the-design-system-as-production-components)** · Implement the design system as production components
- **[WEB-02](05-web-foundation.md#web-02--build-the-capability-aware-application-shell)** · Build the capability-aware application shell
- **[WEB-03](05-web-foundation.md#web-03--scope-navigation-for-screening-candidates-to-their-invitation)** · Scope navigation for screening candidates to their invitation
- **[WEB-04](05-web-foundation.md#web-04--implement-the-cross-journey-state-contract-in-shared-components)** · Implement the cross-journey state contract in shared components
- **[WEB-05](05-web-foundation.md#web-05--build-the-error-forbidden-and-no-workspace-destinations)** · Build the error, forbidden and no-workspace destinations
- **[WEB-06](05-web-foundation.md#web-06--port-every-prototype-screen-to-nextjs-with-verified-parity)** · Port every prototype screen to Next.js with verified parity

### PRO — Candidate profile and documents

- **[PRO-01](06-candidate-profile.md#pro-01--implement-candidate-profile-career-context-and-preferences)** · Implement candidate profile, career context and preferences
- **[PRO-02](06-candidate-profile.md#pro-02--implement-cv-upload-versioning-replacement-and-deletion)** · Implement CV upload, versioning, replacement and deletion
- **[PRO-03](06-candidate-profile.md#pro-03--extract-structured-facts-from-cv-and-job-description-with-provenance)** · Extract structured facts from CV and job description with provenance
- **[PRO-04](06-candidate-profile.md#pro-04--let-candidates-inspect-and-correct-extracted-facts)** · Let candidates inspect and correct extracted facts
- **[PRO-05](06-candidate-profile.md#pro-05--build-the-private-practice-evidence-bank)** · Build the private practice evidence bank

### CAT — Catalogue, artifacts and session composition

- **[CAT-01](07-catalog-and-composition.md#cat-01--build-the-artifact-registry-with-review-publication-and-rollback)** · Build the artifact registry with review, publication and rollback
- **[CAT-02](07-catalog-and-composition.md#cat-02--implement-interview-composition-as-a-durable-workflow)** · Implement interview composition as a durable workflow
- **[CAT-03](07-catalog-and-composition.md#cat-03--serve-the-discipline-role-shape-and-persona-catalogue)** · Serve the discipline, role, shape and persona catalogue
- **[CAT-04](07-catalog-and-composition.md#cat-04--build-the-practice-interview-configuration-wizard)** · Build the practice interview configuration wizard
- **[CAT-05](07-catalog-and-composition.md#cat-05--collect-recording-preference-and-practice-consent-at-composition)** · Collect recording preference and practice consent at composition
- **[CAT-06](07-catalog-and-composition.md#cat-06--build-the-content-authoring-and-publication-approval-surface)** · Build the content authoring and publication-approval surface

### SES — Session lifecycle and orchestration

- **[SES-01](08-session-lifecycle.md#ses-01--implement-the-session-aggregate-and-its-state-machine)** · Implement the session aggregate and its state machine
- **[SES-02](08-session-lifecycle.md#ses-02--implement-session-start-with-quota-reservation-and-scoped-realtime-authorization)** · Implement session start with quota reservation and scoped realtime authorization
- **[SES-03](08-session-lifecycle.md#ses-03--build-the-prepare-screen-with-device-checks-and-a-blocking-consent-gate)** · Build the prepare screen with device checks and a blocking consent gate
- **[SES-04](08-session-lifecycle.md#ses-04--implement-idempotent-completion-and-transcript-sealing)** · Implement idempotent completion and transcript sealing
- **[SES-05](08-session-lifecycle.md#ses-05--implement-active-time-accounting-and-timing-policy)** · Implement active-time accounting and timing policy
- **[SES-06](08-session-lifecycle.md#ses-06--implement-reconnection-grace-expiry-and-interruption-recording)** · Implement reconnection, grace expiry and interruption recording
- **[SES-07](08-session-lifecycle.md#ses-07--build-session-history-with-complete-lifecycle-states)** · Build session history with complete lifecycle states
- **[SES-08](08-session-lifecycle.md#ses-08--build-the-completion-receipt-and-processing-status-screen)** · Build the completion receipt and processing status screen

### RTC — Realtime, media and transcript integrity

- **[RTC-01](09-realtime-and-media.md#rtc-01--implement-browser-to-provider-webrtc-connection-and-teardown)** · Implement browser-to-provider WebRTC connection and teardown
- **[RTC-02](09-realtime-and-media.md#rtc-02--implement-the-control-event-protocol-with-epochs-and-cursors)** · Implement the control event protocol with epochs and cursors
- **[RTC-03](09-realtime-and-media.md#rtc-03--implement-reconnection-and-recovery-in-the-browser)** · Implement reconnection and recovery in the browser
- **[RTC-04](09-realtime-and-media.md#rtc-04--implement-transcript-capture-correction-and-provenance)** · Implement transcript capture, correction and provenance
- **[RTC-05](09-realtime-and-media.md#rtc-05--implement-media-upload-finalization-and-reconciliation)** · Implement media upload, finalization and reconciliation
- **[RTC-06](09-realtime-and-media.md#rtc-06--build-the-live-interview-screen)** · Build the live interview screen
- **[RTC-07](09-realtime-and-media.md#rtc-07--handle-provider-degradation-and-outage-during-a-live-interview)** · Handle provider degradation and outage during a live interview

### EVL — Evaluation and evidence

- **[EVL-01](10-evaluation-system.md#evl-01--build-the-evidence-extraction-pipeline)** · Build the evidence extraction pipeline
- **[EVL-02](10-evaluation-system.md#evl-02--implement-rubric-based-competency-evaluation-as-a-durable-workflow)** · Implement rubric-based competency evaluation as a durable workflow
- **[EVL-03](10-evaluation-system.md#evl-03--implement-sufficiency-coverage-and-the-insufficient-evidence-outcome)** · Implement sufficiency, coverage and the insufficient-evidence outcome
- **[EVL-04](10-evaluation-system.md#evl-04--detect-unverified-claims-and-contradictions-neutrally)** · Detect unverified claims and contradictions neutrally
- **[EVL-05](10-evaluation-system.md#evl-05--implement-confidence-semantics-and-publication-validation)** · Implement confidence semantics and publication validation
- **[EVL-06](10-evaluation-system.md#evl-06--map-job-requirements-to-evidence-without-a-match-percentage)** · Map job requirements to evidence without a match percentage
- **[EVL-07](10-evaluation-system.md#evl-07--handle-evaluation-failure-partial-results-and-budget-exhaustion)** · Handle evaluation failure, partial results and budget exhaustion

### ART — Delivery and articulation

- **[ART-01](11-articulation.md#art-01--compute-deterministic-delivery-features-from-audio-and-transcript)** · Compute deterministic delivery features from audio and transcript
- **[ART-02](11-articulation.md#art-02--implement-assessability-status-and-quality-warnings)** · Implement assessability status and quality warnings
- **[ART-03](11-articulation.md#art-03--produce-the-ten-dimension-delivery-profile-with-evidence)** · Produce the ten-dimension delivery profile with evidence
- **[ART-04](11-articulation.md#art-04--generate-fact-preserving-delivery-coaching-and-suggested-structure)** · Generate fact-preserving delivery coaching and suggested structure
- **[ART-05](11-articulation.md#art-05--build-the-delivery-screen-with-timestamped-evidence-and-drills)** · Build the delivery screen with timestamped evidence and drills
- **[ART-06](11-articulation.md#art-06--implement-redo-and-the-original-versus-redo-comparison)** · Implement redo and the original-versus-redo comparison
- **[ART-07](11-articulation.md#art-07--build-personal-delivery-baselines-and-trends)** · Build personal delivery baselines and trends

### PRC — Practice results, review and coaching

- **[PRC-01](12-practice-experience.md#prc-01--build-the-outcome-and-evidence-screen)** · Build the outcome and evidence screen
- **[PRC-02](12-practice-experience.md#prc-02--build-the-coaching-review-screen)** · Build the coaching review screen
- **[PRC-03](12-practice-experience.md#prc-03--implement-answer-redo-with-preserved-history)** · Implement answer redo with preserved history
- **[PRC-04](12-practice-experience.md#prc-04--implement-drills-and-drill-to-goal-linkage)** · Implement drills and drill-to-goal linkage
- **[PRC-05](12-practice-experience.md#prc-05--implement-practice-notifications-for-ready-results)** · Implement practice notifications for ready results
- **[PRC-06](12-practice-experience.md#prc-06--validate-the-results-review-and-delivery-split-with-candidates)** · Validate the results, review and delivery split with candidates

### PRG — Skills, progression, goals and readiness

- **[PRG-01](13-progression-and-goals.md#prg-01--store-append-only-competency-observations-with-rubric-provenance)** · Store append-only competency observations with rubric provenance
- **[PRG-02](13-progression-and-goals.md#prg-02--compute-readiness-against-a-pinned-role-standard)** · Compute readiness against a pinned role standard
- **[PRG-03](13-progression-and-goals.md#prg-03--build-goals-milestones-and-practice-cadence)** · Build goals, milestones and practice cadence
- **[PRG-04](13-progression-and-goals.md#prg-04--build-the-skills-and-progression-screens-with-evidence-freshness)** · Build the skills and progression screens with evidence freshness
- **[PRG-05](13-progression-and-goals.md#prg-05--use-prior-gaps-to-inform-future-session-composition)** · Use prior gaps to inform future session composition

### SCR — Screening, campaigns and invitations

- **[SCR-01](14-screening-and-invitations.md#scr-01--implement-campaigns-with-immutably-pinned-configuration)** · Implement campaigns with immutably pinned configuration
- **[SCR-02](14-screening-and-invitations.md#scr-02--implement-versioned-candidate-disclosure-and-consent)** · Implement versioned candidate disclosure and consent
- **[SCR-03](14-screening-and-invitations.md#scr-03--implement-job-context-capture-for-a-campaign)** · Implement job context capture for a campaign
- **[SCR-04](14-screening-and-invitations.md#scr-04--implement-invitation-issue-delivery-expiry-and-revocation)** · Implement invitation issue, delivery, expiry and revocation
- **[SCR-05](14-screening-and-invitations.md#scr-05--build-invitation-acceptance-and-identity-resolution)** · Build invitation acceptance and identity resolution
- **[SCR-06](14-screening-and-invitations.md#scr-06--implement-the-accommodation-request-and-fulfilment-path)** · Implement the accommodation request and fulfilment path
- **[SCR-07](14-screening-and-invitations.md#scr-07--enforce-screening-candidate-result-disclosure-policy-server-side)** · Enforce screening candidate result disclosure policy server-side
- **[SCR-08](14-screening-and-invitations.md#scr-08--implement-screening-interruption-and-re-invitation-governance)** · Implement screening interruption and re-invitation governance
- **[SCR-09](14-screening-and-invitations.md#scr-09--enforce-the-supported-language-accent-and-audio-quality-boundary)** · Enforce the supported language, accent and audio-quality boundary

### REV — Recruiter review, decisions and appeals

- **[REV-01](15-recruiter-review-and-appeals.md#rev-01--build-the-candidate-roster-with-campaign-scoped-access)** · Build the candidate roster with campaign-scoped access
- **[REV-02](15-recruiter-review-and-appeals.md#rev-02--build-the-evidence-first-candidate-review-screen)** · Build the evidence-first candidate review screen
- **[REV-03](15-recruiter-review-and-appeals.md#rev-03--implement-human-decisions-with-override-rationale-and-append-only-history)** · Implement human decisions with override rationale and append-only history
- **[REV-04](15-recruiter-review-and-appeals.md#rev-04--audit-sensitive-transcript-audio-and-evaluation-reads)** · Audit sensitive transcript, audio and evaluation reads
- **[REV-05](15-recruiter-review-and-appeals.md#rev-05--implement-constrained-candidate-comparison)** · Implement constrained candidate comparison
- **[REV-06](15-recruiter-review-and-appeals.md#rev-06--build-the-appeals-and-re-review-workflow)** · Build the appeals and re-review workflow
- **[REV-07](15-recruiter-review-and-appeals.md#rev-07--build-the-candidate-facing-appeal-request-and-status)** · Build the candidate-facing appeal request and status
- **[REV-08](15-recruiter-review-and-appeals.md#rev-08--implement-system-flagged-low-confidence-review)** · Implement system-flagged low-confidence review

### TEN — Tenant administration

- **[TEN-01](16-tenant-administration.md#ten-01--implement-tenant-settings-and-branding)** · Implement tenant settings and branding
- **[TEN-02](16-tenant-administration.md#ten-02--implement-members-scoped-roles-and-the-permission-matrix)** · Implement members, scoped roles and the permission matrix
- **[TEN-03](16-tenant-administration.md#ten-03--implement-periodic-access-review)** · Implement periodic access review
- **[TEN-04](16-tenant-administration.md#ten-04--build-the-rubric-library-with-immutable-version-history)** · Build the rubric library with immutable version history
- **[TEN-05](16-tenant-administration.md#ten-05--build-calibration-authoring-impact-preview-and-publication)** · Build calibration authoring, impact preview and publication
- **[TEN-06](16-tenant-administration.md#ten-06--implement-tenant-disclosure-and-accommodation-policy-management)** · Implement tenant disclosure and accommodation policy management
- **[TEN-07](16-tenant-administration.md#ten-07--implement-retention-policy-configuration-and-legal-hold)** · Implement retention policy configuration and legal hold
- **[TEN-08](16-tenant-administration.md#ten-08--implement-usage-quota-and-billing-visibility)** · Implement usage, quota and billing visibility

### INT — Email, webhooks and ATS integration

- **[INT-01](17-integrations.md#int-01--implement-transactional-email-delivery)** · Implement transactional email delivery
- **[INT-02](17-integrations.md#int-02--build-the-transactional-outbox-and-delivery-workflow)** · Build the transactional outbox and delivery workflow
- **[INT-03](17-integrations.md#int-03--implement-signed-webhooks-with-replay-and-ssrf-defence)** · Implement signed webhooks with replay and SSRF defence
- **[INT-04](17-integrations.md#int-04--build-webhook-delivery-history-test-and-replay)** · Build webhook delivery history, test and replay
- **[INT-05](17-integrations.md#int-05--implement-tenant-api-keys-with-scoped-capabilities)** · Implement tenant API keys with scoped capabilities
- **[INT-06](17-integrations.md#int-06--build-the-first-ats-adapter-as-a-pilot)** · Build the first ATS adapter as a pilot

### OPS — Platform operations and internal consoles

- **[OPS-01](18-platform-operations.md#ops-01--build-privacy-controlled-aggregate-analytics)** · Build privacy-controlled aggregate analytics
- **[OPS-02](18-platform-operations.md#ops-02--build-session-and-realtime-health-monitoring)** · Build session and realtime health monitoring
- **[OPS-03](18-platform-operations.md#ops-03--build-workflow-backlog-and-failed-work-recovery)** · Build workflow backlog and failed-work recovery
- **[OPS-04](18-platform-operations.md#ops-04--build-evaluation-quality-and-artifact-version-monitoring)** · Build evaluation quality and artifact version monitoring
- **[OPS-05](18-platform-operations.md#ops-05--build-provider-usage-cost-and-quota-control)** · Build provider usage, cost and quota control
- **[OPS-06](18-platform-operations.md#ops-06--build-the-privileged-append-only-audit-viewer)** · Build the privileged append-only audit viewer
- **[OPS-07](18-platform-operations.md#ops-07--build-the-restricted-super-administrator-console)** · Build the restricted super-administrator console

### SEC — Security, privacy and data rights

- **[SEC-01](19-security-and-privacy.md#sec-01--produce-and-maintain-the-threat-model)** · Produce and maintain the threat model
- **[SEC-02](19-security-and-privacy.md#sec-02--prove-tenant-isolation-adversarially)** · Prove tenant isolation adversarially
- **[SEC-03](19-security-and-privacy.md#sec-03--implement-data-classification-controls-across-storage-and-transport)** · Implement data classification controls across storage and transport
- **[SEC-04](19-security-and-privacy.md#sec-04--implement-consent-lifecycle-including-withdrawal)** · Implement consent lifecycle including withdrawal
- **[SEC-05](19-security-and-privacy.md#sec-05--implement-candidate-data-export-and-correction)** · Implement candidate data export and correction
- **[SEC-06](19-security-and-privacy.md#sec-06--build-the-durable-deletion-workflow-with-reconciliation)** · Build the durable deletion workflow with reconciliation
- **[SEC-07](19-security-and-privacy.md#sec-07--build-the-candidate-data-request-status-surface)** · Build the candidate data-request status surface
- **[SEC-10](19-security-and-privacy.md#sec-10--rate-limit-authentication-and-every-other-abusable-endpoint)** · Rate limit authentication and every other abusable endpoint
- **[SEC-08](19-security-and-privacy.md#sec-08--run-restricted-content-scanning-across-telemetry)** · Run restricted-content scanning across telemetry
- **[SEC-09](19-security-and-privacy.md#sec-09--commission-independent-penetration-and-isolation-testing)** · Commission independent penetration and isolation testing

### A11Y — Accessibility and inclusive content

- **[A11Y-01](20-accessibility.md#a11y-01--establish-the-accessibility-baseline-in-the-design-system)** · Establish the accessibility baseline in the design system
- **[A11Y-02](20-accessibility.md#a11y-02--make-the-live-interview-fully-operable-without-a-mouse-or-sight)** · Make the live interview fully operable without a mouse or sight
- **[A11Y-03](20-accessibility.md#a11y-03--provide-text-and-table-alternatives-for-every-chart)** · Provide text and table alternatives for every chart
- **[A11Y-04](20-accessibility.md#a11y-04--test-with-real-assistive-technology-across-the-candidate-journey)** · Test with real assistive technology across the candidate journey
- **[A11Y-05](20-accessibility.md#a11y-05--run-usability-testing-with-disabled-candidates)** · Run usability testing with disabled candidates
- **[A11Y-06](20-accessibility.md#a11y-06--implement-and-enforce-the-content-rules)** · Implement and enforce the content rules
- **[A11Y-07](20-accessibility.md#a11y-07--verify-the-candidate-journey-on-real-mobile-devices-and-networks)** · Verify the candidate journey on real mobile devices and networks

### QUA — AI quality, datasets and monitoring

- **[QUA-01](21-ai-quality.md#qua-01--build-representative-evaluation-datasets-across-professions)** · Build representative evaluation datasets across professions
- **[QUA-02](21-ai-quality.md#qua-02--build-the-automated-evaluation-harness)** · Build the automated evaluation harness
- **[QUA-03](21-ai-quality.md#qua-03--calibrate-confidence-and-quality-thresholds-against-human-benchmarks)** · Calibrate confidence and quality thresholds against human benchmarks
- **[QUA-04](21-ai-quality.md#qua-04--gate-artifact-and-model-publication-on-an-evaluation-report)** · Gate artifact and model publication on an evaluation report
- **[QUA-05](21-ai-quality.md#qua-05--build-fairness-and-assessability-monitoring)** · Build fairness and assessability monitoring
- **[QUA-06](21-ai-quality.md#qua-06--monitor-ai-quality-in-production-with-alerting-and-rollback)** · Monitor AI quality in production with alerting and rollback

### REL — Release readiness and operational proof

- **[REL-01](22-release-readiness.md#rel-01--pass-the-foundation-release-gate)** · Pass the foundation release gate
- **[REL-02](22-release-readiness.md#rel-02--pass-the-practice-release-gate)** · Pass the practice release gate
- **[REL-03](22-release-readiness.md#rel-03--pass-the-screening-release-gate-for-a-named-pilot)** · Pass the screening release gate for a named pilot
- **[REL-04](22-release-readiness.md#rel-04--establish-slos-error-budgets-dashboards-and-alerts)** · Establish SLOs, error budgets, dashboards and alerts
- **[REL-05](22-release-readiness.md#rel-05--write-and-exercise-the-operational-runbooks)** · Write and exercise the operational runbooks
- **[REL-06](22-release-readiness.md#rel-06--exercise-the-integrity-freeze-and-re-review-procedure)** · Exercise the integrity freeze and re-review procedure
- **[REL-07](22-release-readiness.md#rel-07--produce-the-release-record-for-each-gated-milestone)** · Produce the release record for each gated milestone

