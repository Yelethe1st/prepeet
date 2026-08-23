# Webhook Protocol

**Status:** Proposed  
**Owner:** Integration team  
**Last updated:** 2026-08-23

## Delivery semantics

Tenant webhooks are opt-in, event-filtered, at-least-once, and potentially out of order. Generic payloads never include raw audio, full transcripts, CVs, hidden candidate coaching, or platform-only data.

## Payload

```json
{
  "event_id": "evt_uuidv7",
  "event_type": "review.decision_recorded.v1",
  "schema_version": "1.0",
  "tenant_id": "tenant_uuidv7",
  "occurred_at": "2026-08-23T12:34:56Z",
  "delivery_attempt_id": "delivery_uuidv7",
  "data": {}
}
```

## Signing

Sign timestamp plus exact raw body with HMAC-SHA256 or an approved asymmetric scheme. Send signature version, key ID, timestamp, and signature headers. Receivers verify raw body, timestamp tolerance, secret/key, and event ID. Key rotation supports overlap.

## Receiver contract

- Return 2xx only after durable acceptance.
- Deduplicate by `event_id`.
- Tolerate additive fields and unknown optional events.
- Do not assume ordering.
- Reject stale timestamp or invalid signature.

## Retry

Bounded exponential backoff with jitter. Separate retryable network/5xx/429 from terminal 4xx/configuration errors. Expose attempt time, status, latency, safe response summary, and next retry. Permit audited manual replay with same event ID and new attempt ID.

Repeated terminal failure disables/pauses the endpoint under policy and notifies tenant administrators.

## Endpoint security

- HTTPS only.
- Validate destinations and prevent SSRF through DNS/IP/redirect/egress controls.
- Encrypt secrets separately from normal configuration.
- Rate/concurrency limits per destination.
- Do not log payload bodies or secrets.
- Test delivery uses synthetic non-candidate data.

## Lifecycle

Create → verify/test → active → degraded/paused → rotated → disabled/deleted. Configuration changes and secret access/rotation are audited.

## Compatibility

Publish event schemas, signature examples, retry behavior, IP/egress information where applicable, and deprecation windows. Maintain fixtures and a local verification example without exposing production secrets.

