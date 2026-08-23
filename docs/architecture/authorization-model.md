# Authorization Model

**Status:** Proposed  
**Owner:** Identity/security and Go platform teams  
**Last updated:** 2026-08-23

## Principles

- Deny by default.
- Authorization evaluates identity, capability, tenant, resource scope, purpose, and state.
- Backend policy is authoritative; hidden navigation is not authorization.
- Tenant authority and platform authority are separate.
- Candidate practice ownership is never inherited by an employer.
- Sensitive reads are audited where required.
- Support access is time-, reason-, ticket-, and scope-bound.

## Authorization context

```text
subject_id · subject_type · active_tenant_id · membership_id
capabilities[] · resource_scopes[] · purpose
authentication_strength · platform_grant_id? · issued/expires
```

Roles are capability bundles, not hardcoded checks.

## Proposed roles

- Candidate.
- Recruiter with campaign/role scope.
- Tenant administrator.
- Platform support.
- Platform administrator.
- Elevated super administrator.
- Content author and separate publication approver.
- Narrow workload/service identities.

## Capability groups

Candidate profile/practice, session create/participate/read, campaign/invitation management, screening evidence review, human decision, comparison, appeal, rubric draft/publish, tenant members/settings/retention/billing/integrations, platform analytics/operations/quota/audit/elevation.

The machine-readable catalog is a versioned contract and avoids page-specific permission names.

Representative capability values:

```text
candidate.profile.read_own
candidate.profile.write_own
candidate.practice.read_own
candidate.practice.delete_own
session.create_practice
session.accept_invitation
session.participate
session.read_own_practice
session.read_screen_confirmation
campaign.read
campaign.manage
invitation.read
invitation.manage
evaluation.read_screen
evaluation.review
evaluation.compare
appeal.manage
rubric.read
rubric.draft
rubric.publish
tenant.member_manage
tenant.settings_manage
tenant.retention_manage
tenant.billing_read
tenant.integration_manage
platform.analytics_read
platform.operations_read
platform.operations_execute
platform.quota_manage
platform.audit_read
platform.privileged_elevate
```

## Resource policy

- Candidate reads/manages own practice data.
- Recruiter/admin cannot read practice data merely because the candidate screened for the tenant.
- Screening candidate sees invitation, consent, participation, and policy-approved disclosure/status.
- Recruiter evidence access requires tenant plus campaign/role scope.
- Transcript/audio reads are separately auditable.
- Comparison requires common authorized and comparable scope.
- Publishing calibration and destructive retention/integration changes require stronger capability and recent authentication.

## Matrix

| Action | Candidate | Recruiter | Tenant admin | Platform support/admin | Elevated super admin |
|---|---|---|---|---|---|
| Own practice/profile | Own | No | No | Exceptional purpose | Exceptional |
| Participate in invitation | Own token/session | No | No | Recovery only | Exceptional |
| Screening evidence | Disclosure policy | Scoped | Capability + scope | Approved grant | Active elevation |
| Invitation management | No | Scoped | Yes | No | Exceptional |
| Human decision | No | Scoped | Capability + scope | No | Correction only |
| Tenant members/settings | No | No | Yes | No | Exceptional |
| Platform health/quota | No | Tenant view | Tenant view | Scoped capability | Yes |
| Privileged audit | Own access record | Tenant scope | Tenant scope | Scoped | Yes |

`Own` means candidate-owned practice resource. `Scoped` means an explicit tenant/campaign/role assignment, not merely membership.

### Session participation

The subject must match the authorized candidate/invitation, the session must permit participation, and realtime credentials bind to one attempt. Screening restart is not authorized by possessing the original invitation after it has been consumed.

### Review and comparison

Reading transcript/audio is independently authorized and audited. Recording a decision requires review capability and evidence scope. Comparison additionally requires common role/rubric comparability and feature approval.

### Tenant configuration

Retention reduction, secret/key changes, integration deletion, artifact publication, and membership privilege changes require recent authentication. Enterprise policy may require a second approver.

## Platform elevation

Requires strong recent authentication, ticket, reason, tenant/resource scope, approval where needed, short expiry, visible countdown, and audit of grant, reads, actions, and expiry. Avoid impersonation; if unavoidable, preserve and display the true operator identity.

Elevation never grants unrestricted candidate-practice access by default. Revocation/expiry is enforced immediately for new actions and visible in the operator UI. Tenant-visible support history is retained where policy permits.

## Service identities

- Next.js calls Go on behalf of the user and has no database/Python authority.
- Go API performs product actions under policy.
- Go workers perform named workflow/outbox activities.
- Python reads immutable scoped inputs and returns typed results.
- Media processors access named objects only.
- Integration dispatcher accesses one delivery payload and destination credential.

Use workload identity and short-lived credentials, not shared superusers.

## Enforcement layers

Authentication → Go capability check → domain mode/state/ownership policy → repository tenant predicates → PostgreSQL RLS → object/workflow scope → sensitive-action audit.

## Token rules

Secure HTTP-only same-site browser sessions or equivalent, CSRF defense, rotating refresh with reuse detection, explicit active tenant, hashed invitation tokens, short-lived realtime/media tokens, and step-up for sensitive administration.

## Test requirements

Cross-tenant IDs and lists, practice/screen linkage, route/API/object/SSE/WebSocket/workflow/export, stale membership, invitation states, tenant switching, elevation expiry/scope escape, service misuse, and missing tenant context.

The test matrix covers each capability against own, same-tenant scoped, same-tenant unscoped, different-tenant, platform aggregate, elevated, expired/revoked, and nonexistent resources. Batch/list endpoints must be tested because correct single-resource authorization does not prevent projection leakage.

## Open decisions

Identity provider, enterprise federation, non-hierarchical scopes, publication separation of duties, support impersonation, screening disclosure rights, and step-up method.
