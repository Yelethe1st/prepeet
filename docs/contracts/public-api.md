# Public API

**Status:** Proposed capability surface; generated OpenAPI is authoritative after implementation  
**Owner:** Go API team  
**Last updated:** 2026-08-23

## Conventions

- REST/JSON over HTTPS under `/api/v1`.
- Opaque UUIDv7 resource IDs; tenant is never inferred from ID.
- `Idempotency-Key` for retryable mutations and `If-Match`/version for optimistic updates.
- Cursor pagination, stable sorting, bounded sizes, explicit filtering.
- RFC 3339 UTC times and integer millisecond durations.
- `X-Request-ID` and trace propagation without restricted content.
- Long work returns resource/workflow status rather than holding the request.
- Sensitive existence may return `404`.

```json
{
  "error": {
    "code": "SESSION_INVALID_STATE",
    "message": "This interview has already been finalized.",
    "retryable": false,
    "field_errors": [],
    "request_id": "req_uuidv7"
  }
}
```

Error codes are stable; localized messages are not machine logic.

## Capability surface

### Authentication/tenant

Registration, login/logout/refresh, recovery/reset, verification, magic link, OTP, OAuth providers, current user, memberships, and active tenant.

### Candidate

Profile, documents/uploads, extracted-fact correction, preferences, consents, data requests, goals, competencies, progression, readiness, and session history.

### Catalog/session

Disciplines, roles, interview shapes, personas, create/get/cancel interview, bundle summary, processing status, start/resume/complete, event timeline, transcript, turns, results, review, and redo.

### Realtime/media

Start/resume returns attempt, expiry, scoped provider authorization, and control connection. SSE exposes progress; WebSocket is used only if bidirectional control is required. Media supports upload initiation/parts/finalization and authorized track playback.

### Internal (service credential, never a person's session)
POST /api/v1/internal/interviews/{session_id}/events

The voice agent is the transcript's source of truth (ADR-0019). This
operation accepts the platform's own service token as a bearer credential,
compared in constant time; a session cookie never opens it, and a
deployment with no token configured answers 401 to everything. The server
stamps the current connection epoch and assigns sequences itself, so the
agent and the browser never share a numbering.

# Invitations/recruiting

Resolve/accept invitation; campaigns; invitation list/create/detail/resend/revoke; candidate roster/detail; review decisions/history; comparison request; appeals assign/resolve.

### Tenant administration

Settings, members/roles, rubric/calibration draft/publish, usage/quota, integrations, webhooks, delivery history/test/replay.

### Platform

Analytics, sessions, evaluations, health, workflows/retry, AI usage, quotas, elevation, and audit.

## Representative routes

```text
# Authentication and tenant context
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/auth/refresh
POST /api/v1/auth/password/forgot
POST /api/v1/auth/password/reset
POST /api/v1/auth/email/verify
POST /api/v1/auth/magic-links
POST /api/v1/auth/magic-links/consume
POST /api/v1/auth/otp/verify
GET  /api/v1/auth/providers
GET  /api/v1/me
GET  /api/v1/me/memberships
PUT  /api/v1/me/active-tenant

# Candidate profile/preferences
GET    /api/v1/me/candidate-profile
PATCH  /api/v1/me/candidate-profile
POST   /api/v1/me/documents
GET    /api/v1/me/documents
DELETE /api/v1/me/documents/{document_id}
GET    /api/v1/me/extracted-facts
PATCH  /api/v1/me/extracted-facts/{fact_id}
GET    /api/v1/me/preferences
PATCH  /api/v1/me/preferences
GET    /api/v1/me/consents
POST   /api/v1/me/consents
POST   /api/v1/me/data-requests

# Catalog/session
GET  /api/v1/interviews/practice-consent
GET  /api/v1/catalog/disciplines
GET  /api/v1/catalog/roles
GET  /api/v1/catalog/interview-shapes
GET  /api/v1/catalog/personas
POST /api/v1/interviews
GET  /api/v1/interviews/{session_id}
GET  /api/v1/interviews/{session_id}/bundle-summary
GET  /api/v1/interviews/{session_id}/processing
POST /api/v1/interviews/{session_id}/cancel
POST /api/v1/interviews/{session_id}/start
POST /api/v1/interviews/{session_id}/resume
POST /api/v1/interviews/{session_id}/complete
GET  /api/v1/interviews/{session_id}/events
POST /api/v1/interviews/{session_id}/events
GET  /api/v1/interviews/{session_id}/stream
POST /api/v1/interviews/{session_id}/media/uploads
POST /api/v1/interviews/{session_id}/media/parts
POST /api/v1/interviews/{session_id}/media/finalize
GET  /api/v1/interviews/{session_id}/media/{track}
GET  /api/v1/interviews/{session_id}/results
GET  /api/v1/interviews/{session_id}/review
GET  /api/v1/interviews/{session_id}/transcript
GET  /api/v1/interviews/{session_id}/turns
POST /api/v1/interviews/{session_id}/turns/{turn_id}/redos
GET  /api/v1/me/sessions
GET  /api/v1/me/competencies
GET  /api/v1/me/progression
GET  /api/v1/me/readiness
GET  /api/v1/me/goals
POST /api/v1/me/goals
PATCH /api/v1/me/goals/{goal_id}

# Invitations/recruiting
GET  /api/v1/invitations/{token}
POST /api/v1/invitations/{token}/accept
GET  /api/v1/tenant/campaigns
POST /api/v1/tenant/campaigns
GET  /api/v1/tenant/invitations
POST /api/v1/tenant/invitations
GET  /api/v1/tenant/invitations/{invitation_id}
POST /api/v1/tenant/invitations/{invitation_id}/resend
POST /api/v1/tenant/invitations/{invitation_id}/revoke
GET  /api/v1/tenant/candidates
GET  /api/v1/tenant/candidates/{candidate_id}/reviews
POST /api/v1/tenant/review-cases/{case_id}/decisions
GET  /api/v1/tenant/review-cases/{case_id}/history
POST /api/v1/tenant/comparisons
GET  /api/v1/tenant/appeals
POST /api/v1/tenant/appeals/{appeal_id}/assign
POST /api/v1/tenant/appeals/{appeal_id}/resolve

# Tenant configuration
GET/PATCH /api/v1/tenant/settings
GET/POST  /api/v1/tenant/members
PATCH     /api/v1/tenant/members/{membership_id}
DELETE    /api/v1/tenant/members/{membership_id}
GET       /api/v1/tenant/rubrics
POST      /api/v1/tenant/rubrics/{rubric_id}/calibrations
POST      /api/v1/tenant/calibrations/{id}/publish
GET       /api/v1/tenant/usage
GET       /api/v1/tenant/quota
GET/POST  /api/v1/tenant/integrations
GET/POST  /api/v1/tenant/webhooks
GET       /api/v1/tenant/webhooks/{id}/deliveries
POST      /api/v1/tenant/webhooks/{id}/test
POST      /api/v1/tenant/webhook-deliveries/{id}/replay

# Platform operations
GET  /api/v1/platform/analytics
GET  /api/v1/platform/sessions
GET  /api/v1/platform/evaluations
GET  /api/v1/platform/system/health
GET  /api/v1/platform/system/workflows
POST /api/v1/platform/system/workflows/{id}/retry
GET  /api/v1/platform/usage
GET  /api/v1/platform/quotas
PATCH /api/v1/platform/quotas/{tenant_id}
POST /api/v1/platform/elevations
DELETE /api/v1/platform/elevations/{id}
GET  /api/v1/platform/audit
```

This is a capability inventory, not permission to expose every route in the first release. Each route requires request/response/error/idempotency/authorization/pagination/audit examples in generated OpenAPI before implementation is considered complete.

## Security

Secure HTTP-only browser sessions or equivalent, CSRF defense, explicit active tenant, server authorization, bounded uploads, safe redirects, rate limits, and short-lived media/realtime authorization. Browser never calls Python or PostgreSQL directly.

## Compatibility

OpenAPI lint, generated TypeScript client, drift checks, additive changes within version, explicit deprecation, and consumer contract tests. Exact request/response and error schemas must be defined before implementation of each capability.
