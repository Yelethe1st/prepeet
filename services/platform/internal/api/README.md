# internal/api

The HTTP layer for the public API.

It holds no product rules. Every decision about whether a credential is good, whether a session is live,
or what a refused token means belongs to the context that owns it; this package translates between that
and HTTP. When a handler here starts making a decision, the decision is in the wrong place.

## Why the ports are declared here

`internal/api` cannot import `internal/identity`. [ADR-0005](../../../../docs/architecture/decisions/0005-module-boundaries-and-extraction.md)
forbids one bounded context importing another and the module boundary test enforces it, so the
alternative is not "import it and be careful" but "does not compile".

So [`identity.go`](identity.go) declares the narrow interface this package needs and `cmd/api` supplies
an adapter. The duplication that creates is real and is the price of the two being separable.

It earns that price in one specific place. The identity context distinguishes `ErrNotFound` from
`ErrCredentialsInvalid` because its own logic needs to, and this layer must not, because a response that
could tell them apart is an account-existence oracle. The collapse happens once, in the adapter, rather
than being a rule every handler is trusted to remember.

## Why some responses are hand-written

`oapi-codegen` models a response header as a single field and writes it with `Header().Set`, which
replaces rather than appends. Login and refresh set two cookies with different paths, so there is no
value of that one string that produces the right result: whichever cookie is written second is the only
one the browser receives.

[`response.go`](response.go) writes those by hand. They still satisfy the generated interfaces, so the
contract stays the source and a handler returning the wrong shape still fails to compile. Only the
writing is ours.

Changing the contract to describe one cookie was rejected. That would make the document lie about the
wire in order to suit a generator, and [ADR-0004](../../../../docs/architecture/decisions/0004-contract-conventions-and-code-generation.md)
makes the contract the source.

## Why cookies arrive through the context

The generated strict request objects carry the parsed body and parameters and nothing else, which is
usually a feature: a handler working from typed input cannot quietly depend on a header the contract does
not mention.

Session tokens are the exception it forces. They travel in cookies precisely so no script can read them,
which means the one thing every authenticated operation needs is the one thing the generated input omits.
A strict middleware reads them once into the context, and [`context.go`](context.go) is the only way a
handler gets at them.

## What is asserted, and why

The tests in this package run against a fake identity, because what is under test is HTTP behaviour:
which status, which cookies, which envelope, and what is absent from a body. The identity rules are
asserted in their own package against a real database, and duplicating them here would mean two places to
update and one that gets forgotten.

Two of these assertions exist because an earlier version of them did not fail when the behaviour was
removed:

- The rejected-login message is pinned, not just compared between the two failure cases. Comparing them
  passes for a message that leaks, because it leaks equally in both.
- The 500 message is pinned to fixed text rather than only scanned for secrets. Scanning passed when the
  handler used `err.Error()`, because the scrubber happened to redact the fixture's connection string.
  Scrubbing is the last line, not the rule.
