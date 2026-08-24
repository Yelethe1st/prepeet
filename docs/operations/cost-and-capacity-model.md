# Cost and Capacity Model

**Status:** Model defined; forecasts require discovery and load data  
**Owner:** Platform, AI engineering, product, and finance  
**Last updated:** 2026-08-23

Infrastructure is priced against AWS `eu-west-2` London per [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md), which runs roughly 5 to 10 percent above `us-east-1` for equivalent resources. Model and realtime provider spend is expected to dominate infrastructure spend, so that premium is not the first number to optimise.

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

The first set of these is written down. [ADR-0006](../architecture/decisions/0006-postgresql-serves-cache-coordination-and-rate-limiting.md) names four triggers for introducing Redis, each with what to measure, an estimated threshold, and what to try before reaching for it. The estimates are arithmetic rather than measurement, which is what the instrumentation in PLT-08 and OPS-02 exists to replace.

## Open decisions

Billing unit (seat/interview/minute/evaluation), insufficient/failed billing, price currency, quota messaging, provider pricing, retention tier, traffic forecast, and enterprise headroom.

