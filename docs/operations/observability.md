# Observability

**Status:** Proposed  
**Owner:** Platform/SRE  
**Last updated:** 2026-08-23

## Standard

Use OpenTelemetry across browser, Next.js, Go, Python, Temporal, PostgreSQL, object storage, and approved provider calls. Preserve correlation across the complete interview journey without exporting restricted content.

## Safe dimensions

Environment, region, service/version, route template/capability, workflow/activity, mode, provider/model policy, outcome, retry class, and controlled tenant tier/reference where justified.

Do not use person, tenant, session, invitation, or document IDs as unbounded metric labels. Never log raw CV, transcript, audio, prompt, model output, access/invitation token, or secret.

## Dashboards

- Candidate creation/start/completion funnel and failure reason.
- Realtime connection, reconnect, event gaps, and provider health.
- Workflow backlog/age/retry/terminal failure.
- Evaluation stage, invalid output, insufficiency, latency, quality version.
- Articulation assessability and feature failures without candidate content.
- Media upload/finalization/playback authorization.
- Webhook delivery and endpoint health.
- Tenant usage/quota and cost.
- Authentication, authorization denials, and privileged access.
- SLO/error budgets.

## Alerts

Each alert identifies user impact, owner, runbook, urgency, and safe diagnostic context. Page for journey burn, regional provider failure, workflow age, database/PITR risk, isolation anomaly, finalization collapse, webhook backlog, AI unsupported-fact/invalid-output regression, cost abuse, and deletion SLA breach.

## Logs/traces

Structured logs use standardized codes and low-cardinality fields. Trace spans record capability, version, timings, retry/failure, and safe object references. Debug content capture is exceptional, approved, time-limited, encrypted, audited, and off by default.

## Quality telemetry

AI quality metrics are versioned by artifact/model/calculator and include evidence validation, unsupported facts, insufficiency, assessability, human override, and re-review. They are not combined with protected demographic analysis without an approved data-governance plan.

## Validation

Automated telemetry scanning, failure-injection trace review, dashboard/runbook drills, cardinality budgets, retention/access review, and incident sampling.

