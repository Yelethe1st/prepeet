# Service-Level Objectives

**Status:** Proposed targets requiring load/business validation  
**Owner:** SRE and product engineering  
**Last updated:** 2026-08-23

## Principles

Measure user journeys, not container uptime. Separate platform-controlled failure from third-party/provider dependencies while preserving the user's experienced outcome. Define error-budget policy before commitments.

## Proposed SLOs

| Indicator | Objective |
|---|---|
| Authenticated API availability | 99.9% monthly |
| Start-interview control path availability | 99.95% in supported regions |
| Session-create API latency | p95 < 750 ms excluding composition |
| Session composition ready | p95 < 15 s; p99 < 45 s |
| Browser start to realtime connection | p95 < 5 s on supported networks |
| Realtime event acknowledgment | p95 < 250 ms |
| Post-turn directive | p95 < 1.5 s with safe fallback |
| Practice completion to review | p95 < 3 min |
| Screening completion to review | p95 < 5 min |
| Media finalization | 99.5% supported browser/device sessions |
| Webhook accepted within 5 min | 99% excluding destination outage |
| Privacy deletion | Within approved policy SLA |

## SLIs

Creation-to-ready, connection success, reconnect recovery, transcript sequence gaps, directive latency, completion receipt, finalization success, evaluation stage age/failure, review-ready latency, webhook age, authorization anomalies, and deletion completion.

## Error-budget policy

- Burn-rate alerts over fast and slow windows.
- Freeze risky feature/provider/artifact rollout when journey budget is exhausted.
- Reliability work takes priority over expansion after repeated breach.
- Screening tenants may require stricter windows/support commitments than practice; do not promise before evidence.

## Measurement rules

Publish eligible population, exclusions, regional/provider segmentation, numerator/denominator, delayed-event handling, and source. Do not hide provider-caused candidate failures from experience dashboards.

## Review

Review monthly initially and after traffic/provider/topology changes. Ratification requires representative load, observed pilot data, on-call ownership, and cost assessment.

