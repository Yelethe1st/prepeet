# Cost and Capacity Model

**Status:** Model defined; forecasts require discovery and load data  
**Owner:** Platform, AI engineering, product, and finance  
**Last updated:** 2026-08-23

## Demand inputs

- Registered/monthly active candidates.
- Invitations and starts per minute.
- Concurrent live sessions by region.
- Average/p95 interview minutes.
- Transcript events per minute.
- Audio bytes per minute and retention.
- Evaluations per minute and backlog tolerance.
- Model tokens/audio minutes by capability.
- Webhook endpoints/deliveries.
- Database rows, bytes, IOPS, connections, and hot indexes.

## Cost attribution

Immutable usage entries attribute tenant, session, mode, capability, provider/model, unit, price version, region, and experiment for:

- realtime audio input/output;
- transcription;
- extraction/composition;
- runtime directives;
- turn/session/articulation/coaching evaluation;
- object storage/retrieval;
- compute/workflow/database/cache;
- notification/webhook;
- observability.

## Capacity scenarios

Model normal traffic, invitation-campaign start burst, synchronized session completion/media upload, provider recovery evaluation backlog, tenant quota exhaustion, and regional failover. Load testing uses expected peak plus approved headroom.

## Controls

- Per-tenant/subject rate, concurrency, duration, upload, and AI budgets.
- Soft warning before hard limit.
- In-flight interviews complete safely after quota exhaustion; new-start messaging avoids exposing employer billing details.
- Optional coaching degrades before required evaluation when budget policy permits.
- Provider routing and fallback account for equivalence, region, latency, and cost.
- Cost anomaly alerts and abuse investigation.

## Unit economics

Track cost per created, started, completed, review-ready, and insufficient-evidence session separately for practice/screen. Include failed attempts and support/observability overhead; do not optimize by hiding unsuccessful user journeys.

## Scaling thresholds

Define measured triggers for database partition/read replica, evaluation worker pools, search/warehouse, provider diversification, regional expansion, service extraction, and Kubernetes. Each requires an ADR and operating-cost owner.

## Open decisions

Billing unit (seat/interview/minute/evaluation), insufficient/failed billing, price currency, quota messaging, provider pricing, retention tier, traffic forecast, and enterprise headroom.

