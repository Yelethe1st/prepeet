# Telemetry conventions

**Status:** Accepted  
**Owner:** olabode omoyele  
**Last updated:** 2026-08-24

The concrete contract behind [observability.md](observability.md). That document says what to observe;
this one says exactly how, in terms every language in the system implements identically.

One trace crosses the browser, Next.js, Go, Temporal, Python and the provider call. A trace only holds
together if all five agree on names, and a correlation identifier is only useful if all five spell it the
same way. This document is what they agree on.

Implemented for Go in [`platform/telemetry`](../../services/platform/platform/telemetry). The web and
intelligence implementations follow the same rules and are checked by the same assertions.

## The rule that outranks the rest

**No restricted content reaches telemetry, ever.**

[data-classification.md](../security/data-classification.md) classifies transcript text, evaluation prose
and candidate contact details as Restricted. Telemetry leaves this system: it goes to a vendor, is
retained on somebody else's schedule, and is readable by anyone with dashboard access. A transcript
fragment in a span attribute is therefore a restricted disclosure that no retention policy covers and no
deletion request can reach.

This is not a review convention. A convention would not survive a hurried debugging session at two in
the morning, which is exactly when someone adds the attribute that leaks. It is enforced two ways:

1. **An allowlist.** An attribute is built from an approved key or it is not built.
2. **A scrubber.** Free text is filtered before it is attached, because error messages are written by
   whoever is debugging and carry whatever was to hand.

## Attribute keys

Every custom key is namespaced `prepeet.` so it cannot collide with a key a library sets.

| Key | Carries |
|---|---|
| `prepeet.request_id` | The correlation identifier echoed in `X-Request-ID` and every error envelope |
| `prepeet.tenant_id` | The tenant the request acts within |
| `prepeet.user_id` | The acting user |
| `prepeet.session_id` | The interview session |
| `prepeet.event_id` | An outbox event |
| `prepeet.event_type` | The event's type, from the fixed set |
| `prepeet.capability` | The capability an authorization decision was about |
| `prepeet.decision` | The outcome of that decision |
| `prepeet.mode` | Practice or screening |
| `prepeet.environment` | The deployment |
| `prepeet.outcome` | A terminal state from a fixed set, never a sentence |
| `prepeet.error_code` | A stable code from the error envelope, never a message |
| `prepeet.attempt` | Retry number |
| `prepeet.artifact_version` | The pinned rubric, persona, plan or prompt version |
| `prepeet.policy_version` | The policy version a decision was made under |
| `prepeet.duration_ms` | A measured duration |

**Every one of these identifies something rather than describing it.** That is the property doing the
work: an identifier resolves to a record for somebody already authorised to read it, while a description
carries the content itself to whoever holds the dashboard.

Adding a key is a reviewed change to the allowlist. A key whose name contains `email`, `transcript`,
`password`, `token`, `secret`, `credential`, `answer`, `content`, `payload`, `prose`, `name`, `phone` or
`address` is rejected by test, because a key named that will eventually carry one.

Standard OpenTelemetry semantic conventions are used unchanged where they exist: `http.route`,
`http.request.method`, `http.response.status_code`, `service.name`, `deployment.environment`.

## Span naming

A span name is a **route template, never a resolved path**.

```
GET /api/v1/sessions/{sessionID}      correct
GET /api/v1/sessions/01a0301d-aa10…   wrong, twice over
```

Wrong twice because a name built from the path gives every session its own name, which makes latency
unaggregatable and is the standard way to overwhelm a tracing backend; and because a path segment is
caller controlled, so a name built from one is unclassified text arriving in telemetry.

An unmatched path gets a fixed name. Otherwise anyone can mint span names by requesting paths that do not
exist. An unrecognised HTTP method becomes `OTHER` for the same reason.

Query strings are never recorded.

## Metric dimensions

Stricter than span attributes, because a metric attribute is repeated across every series: an unbounded
one multiplies storage rather than adding a row.

**Permitted:** environment, service, route template, capability, workflow and activity name, mode,
provider and model policy, outcome, retry class, status code.

**Never:** request, user, tenant, session, invitation or document identifiers. These are all unbounded,
and `prepeet.request_id` in particular creates exactly one time series per request.

Latency is a **histogram**, never an average. An average stays acceptable while the slowest one request in
a hundred times out, and that request is the incident.

## Scrubbing

Applied to every attribute value and every log message, in this order:

| Pattern | Becomes | Reaches telemetry via |
|---|---|---|
| `scheme://user:password@host` | `scheme://[redacted]@` | Driver errors, which carry their own credentials by default |
| `$argon2id$…` | `[redacted hash]` | An error about a stored credential being unusable |
| `ses_…`, `ref_…`, `vrf_…`, `rst_…`, `mgc_…`, `inv_…` | `[redacted token]` | A rejected token logged so somebody can check which one it was |
| An email address | `[redacted address]` | A lookup failure naming the account it could not find |

Connection strings are handled first, because one contains something that looks like neither an address
nor a token and a little like both.

Anything over **512 characters** is truncated with a visible marker. Blunt, and correct: a dashboard
cannot display a paragraph anyway, and a paragraph arriving in telemetry is usually content that got
there by accident. The marker matters as much as the cut, because a silently shortened message reads as
complete and somebody will draw a conclusion from the half they can see.

Scrubbing must not make a message useless. `evaluation failed for session ses_7Kq2XA: provider timed out
after 30s` keeps everything an operator needs and loses only the token.

Identifiers survive scrubbing intact. Scrubbing them would make every span useless while protecting
nothing.

## Correlation

**Propagation is W3C `traceparent` and `baggage`**, in every language, in both directions. A request
arriving with a trace context continues that trace rather than starting a second one describing the same
user action.

Propagation is configured even when export is off, so local development behaves like deployment in the
one respect a distributed trace depends on.

**Every log line carries `trace_id` and `span_id`** when written inside a span, plus `service` and
`environment`. A log line and a span describing the same moment are useless separately: the log says what
happened and the trace says where it sat. Joining them is also what stops people pasting content into log
messages to compensate.

**`prepeet.request_id` on the span equals the `X-Request-ID` header on the response.** That equality is
what makes a user quoting a value from an error message lead to a single trace.

## Span status

Only a **5xx** marks a span as an error. A 4xx is the caller getting it wrong; marking those as errors
produces an error rate that tracks how many people mistyped a password, which trains everyone to ignore
it.

A panic is recorded on its span with a stack trace, marked as an error, and answered with the standard
error envelope. Its message goes to the trace and never to the client. It is scrubbed like any other free
text, because a panic message is written with no classification in mind.

Failed requests are measured too. Leaving them out of the latency distribution removes exactly the
requests that went worst, which is how a latency graph stays flat through an incident.

## Service names

`prepeet-api`, `prepeet-worker`, `prepeet-migrate`, `prepeet-web`, `prepeet-intelligence`.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PREPEET_OTLP_ENDPOINT` | unset | The collector, as `host:port`. Unset disables export |
| `PREPEET_TRACE_SAMPLE_RATIO` | `1` | Fraction of traces recorded, 0 to 1 |

Export is off by default. An engineer running the stack should not need a collector, and a default
endpoint would produce a steady stream of connection errors that teaches everyone to ignore telemetry
logs.

Sampling defaults to everything, and is `ParentBased`. A product with no traffic gains nothing from
sampling and loses the one trace it needed, and sampling each span independently produces traces with
holes, which are worse than no trace: a gap reads as an absence of work rather than an absence of
recording.

**OTLP is used rather than a vendor SDK.** The observability vendor is still open in
[deployment-topology.md](deployment-topology.md), and a vendor client would quietly make that decision by
being expensive to remove.

## What each implementation must prove

Not by reading this document, but by passing these assertions. They are the shared suite named in
[SEC-08](../delivery/tickets/19-security-and-privacy.md).

- No approved key contains a word from the forbidden list, and every key is namespaced.
- Building an attribute from an unapproved key fails.
- The scrubber removes each of the four patterns, keeps the operational remainder, truncates visibly, and
  leaves identifiers and ordinary messages untouched.
- A recorded span from a real request carries no value matching any of the four patterns.
- A log line written through the provided logger, with no explicit scrub call, carries none either.
- A log line written inside a span carries that span's trace.
- A span is named for the route, and an unmatched path does not become a name.
- An inbound `traceparent` is continued, with a valid parent.
- 4xx does not mark a span as an error; 5xx does.
- A panic ends its span, is recorded, is scrubbed, and does not reach the client.
- No metric dimension is unbounded, and two requests to one route share a series.

The suite is written once per language and run against that language's implementation, so an
implementation inherits the assertions rather than copying them.

## Related: the fan-out transport

Live progress does not travel over telemetry. It travels over
[`platform/broadcast`](../../services/platform/platform/broadcast), and the same classification rule
applies: a topic carries identifiers and small signals, never restricted content. A subscriber that needs
the content reads it from the database using the identifier, under the row-level security that decides
whether it may.

Topics are lower case letters, digits and underscores, at most 63 bytes, because that is PostgreSQL's
identifier limit and the transport is currently `LISTEN/NOTIFY`. Payloads are capped at 4000 bytes.
Delivery is best effort and nothing is replayed, so anything that must not be lost goes through the
outbox instead. See [ADR-0006](../architecture/decisions/0006-postgresql-serves-cache-coordination-and-rate-limiting.md).

## Open

- **Vendor.** Deferred with [deployment-topology.md](deployment-topology.md). OTLP is what keeps it
  deferrable.
- **Cardinality budgets.** Named in [observability.md](observability.md) and not yet enforced. They need
  a real series count to be set against, which needs traffic.
- **Web and intelligence implementations.** Neither service exists yet. PLT-08 is not complete until a
  single trace crosses all three.
