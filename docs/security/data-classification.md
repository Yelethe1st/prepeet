# Data Classification

**Status:** Proposed  
**Owner:** Security and privacy  
**Last updated:** 2026-08-23

## Classes

| Class | Examples | Required controls |
|---|---|---|
| Restricted | CV, transcript, audio, screening evaluation, accommodations, identity secrets | Need-to-know, encryption, purpose binding, sensitive-read audit, no ordinary telemetry |
| Confidential | Profile, invitation, decision, calibration, API key, billing | Tenant isolation, encryption, scoped access, material-action audit |
| Internal | Aggregate operations, artifact drafts, runbooks | Workforce/service authorization |
| Public | Marketing and published public policy | Integrity and publication control |

Derived data inherits the highest source classification unless a documented anonymization review approves otherwise.

## Handling rules

### Restricted

- Never place in ordinary logs, traces, metrics, crash reports, URLs, event names, or generic webhooks.
- Use private encrypted storage and short-lived scoped access.
- Access requires resource scope and purpose; sensitive reads may be audited.

Transcript reads are audited today, and the obligation is declared in the API contract rather than
decided in code: an operation carrying `x-prepeet-sensitive-read` is recorded before its response is
written, and a failure to record refuses the read. Widening it to another operation is a line in the
document that a reviewer sees.

What is recorded is an actor who read or was refused. A request carrying no resolvable session is not
recorded here: it never reached the content, the request log and the rate limiter already describe it,
and a row naming no actor could not be admitted by any policy on `audit.events` without one that
decides nothing about who is asking. PostgreSQL ORs permissive policies, so such a policy would
re-open the table however well the others are written, which `internal/isolation` refuses.

Note that the wording across this file and
[authorization-model.md](../architecture/authorization-model.md) is inconsistent, saying variously
"audited where required", "separately auditable", "independently authorized and audited" and "may be
audited"; what is built covers the one unambiguous requirement and claims nothing wider.
- Production copies do not enter lower environments.
- AI provider transfer requires approved purpose, region, retention/training policy, and minimization.

### Confidential

- Exclude secrets from all telemetry.
- Encrypt credentials separately and rotate.
- Tenant exports are authorized, auditable, and minimized.
- Analytics use controlled identifiers and avoid unnecessary person linkage.

## Data inventory fields

Every data asset records owner, class, subjects, purpose, source, region, storage/processors, access roles, retention, deletion mechanism, legal basis/consent, export/disclosure, backup behavior, and incident owner.

## AI and analytics

- Prompts and outputs containing candidate content are Restricted.
- Token/cost/latency without content may be Confidential/Internal after review.
- AI evaluation datasets require manifest, provenance, de-identification, consent/legal basis, access, expiry, and deletion.
- Synthetic data is preferred but labeled; it cannot alone establish real-world quality.

## Telemetry policy

Use safe opaque IDs only when operationally necessary; never use person/tenant/session IDs as unbounded metric labels. Debug content capture is off by default, time-limited, approved, access-controlled, and audited.

## Review

Automated scans and manual sampling validate telemetry, storage, exports, and datasets before production and after significant changes.

