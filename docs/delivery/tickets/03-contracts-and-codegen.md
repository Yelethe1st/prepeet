# Epic CTR — Contracts and code generation

**Phase 1** · **Workstream** Go, Python, Web

The browser talks REST to Go. Go talks Protobuf RPC to Python. Durable facts travel as versioned
events. All three are generated from checked-in definitions, and drift fails the build. This epic is
what lets the three workstreams proceed in parallel without integrating by rumour.

---

### CTR-01 · Define the public REST contract and generate the TypeScript client

**Depends on** DEC-08 · **Blocks** every web ticket · **In progress**

OpenAPI is authored, linted and used to generate the browser client. Error shape, idempotency headers,
cursor pagination and version headers are in the contract, not in each handler.

The toolchain is built against [ADR-0004](../../architecture/decisions/0004-contract-conventions-and-code-generation.md):
`packages/contracts/api/openapi.yaml` is the source, `oapi-codegen` produces a strict Go server
interface and `openapi-typescript` the browser types, Spectral lints the document with a ruleset of its
own, and `make check-generated` fails the build on drift. The health and authentication operations are
written; the rest arrive with the tickets that need them.

**Done when**
- [x] The toolchain generates Go and TypeScript from one hand-authored document, and drift fails the build.
- [x] Spectral lints the document, with rules for the error envelope and for undocumented operations.
- [x] The server's responses are checked against the generated types, so a shape change is a test failure.
- [x] OpenAPI covers every route in [public-api.md](../../contracts/public-api.md) that the current phase ships. Health and authentication are done.

A limitation of the Go generator is recorded here rather than left to be rediscovered. A response header
is generated as a single field written with `Header().Set`, so an operation setting two cookies cannot be
served by the generated response type. IAM-01 writes those three responses by hand against the generated
interfaces. The contract is not adjusted to suit the generator: it describes the wire, and the wire has
two cookies.
- [x] `oasdiff` runs against the previous release once there is a previous release.
- [x] Every operation declares its required capability, which needs the capability catalogue published as a contract in IAM-04.
- [x] Every operation declares its cacheability, per the conventions added to ADR-0004: `no-store` for anything derived from a candidate's own data, `ETag` with a short `max-age` for the catalogue, and indefinite for anything addressed by an immutable version.

Every response in the document declares `Cache-Control`, including the shared error responses, because a
response that says nothing is not neutral: an intermediary applies its own heuristics, and the ones for an
authenticated JSON endpoint are not something to leave to a CDN's defaults.

Two tests keep the declaration from being a comment. One walks the embedded contract and fails if any
response omits the header, which covers operations no handler serves yet. The other makes a real request
per served operation and compares what arrived to what the document promises.

Declaring the header also changed the generated response types, so a handler now populates
`Headers.CacheControl` rather than setting a string of its own. Removing a declaration from the contract
makes the handler fail to compile, which is a stronger gate than a test: for anything the server
implements, the document and the code cannot drift at all.

The `ETag` half of the convention lands with the catalogue endpoints in CAT-03, which are the responses
it describes. Nothing shipped so far is cacheable.

**The last three boxes, and one of them was already true.**

`oasdiff` had stopped needing this ticket. CTR-04 built the gate: `make check-api` compares the document
against the previous release, refuses a breaking change, and runs in CI on every change. Proved by
tagging a throwaway clone, removing `/me/memberships` and watching the target fail with "the path was
removed, so a client still calling it gets a 404", then watching it pass against an unchanged document.
It is inert until the first tagged release, which is what the criterion said it would be.

One discrepancy is recorded rather than left to be rediscovered. The gate is `tools/apicompat`, a
first-party checker, and ADR-0004 names `oasdiff` twice. Nothing is wrong with the checker, and its
refusals name a remedy where oasdiff's do not, but the ADR is binding and currently describes a tool the
repository does not use. Either the ADR is amended or the tool is swapped, and that decision belongs
with CTR-04, which chose the checker.

**The capability declaration was the work.** Every operation now carries `x-prepeet-capability`, naming
an entry in the catalogue IAM-04 published or one of three reserved words for the operations no catalogue
entry describes: `public`, `authenticated`, `service`. The blocker the criterion named is gone, because
`packages/contracts/authz/capabilities.yaml` exists and is generated into Go.

It is bound four ways, because a declaration nothing reads is a comment. Spectral fails the lint on an
operation without one. `NewServer` reads the contract at startup and refuses to build if an operation
omits one, so a missing declaration is a process that will not start rather than a request that is let
through. A test refuses a value the capability catalogue does not define, which covers the operations no
handler serves yet. And the two handlers decided through the policy path, member administration and the
billing reads, now take the capability they enforce from the document rather than from a string literal
of their own: the strict middleware knows which operation the router matched, and carries the
declaration into the handler, so changing the contract changes what is enforced.

Where the declaration is a statement rather than an enforcement is written down here. Own-data
capabilities are not decided through `authz` at all. `Identity.Authorize` builds its request without an
owner, so an own-data capability asked through it would be denied by construction, and those operations
are owner-scoped structurally instead, by ports that take only the session's own user. What holds them
is a test that every operation declaring anything but `public` answers 401 without a credential. Closing
that gap means changing the identity port, which is IAM's and not this ticket's.

`recordInsightFeedback` declares `session.read_own_practice` and is the one approximate declaration. It
is a write, and the catalogue has no own-practice write capability short of
`candidate.practice.delete_own`. The authority it needs really is "this practice session is mine", which
is what that capability says, but a read capability gating a write deserves a reviewer's attention and
IAM-04 may want an entry for it.

**Logout was wrong in the document and is now right.** It declared a 401 the handler cannot produce:
logout is idempotent by design and answers 204 whether or not a session was presented. The security
block now says the session cookie is optional, which is OpenAPI's way of saying accepted rather than
demanded, and the unreachable 401 is gone. The Spectral rule requiring an error response now also
accepts a 2xx-only operation, which is what its existing exemption for the health probes already meant.

**Coverage is checkable from the consumer's side now, not only asserted.** Nothing can be served that
the document does not declare: the handler implements a generated interface, so an operation without a
handler does not compile. The missing half was the browser, where the client took any string, so a call
to a route the document never declared failed as a 404 on somebody's screen and no test of the client
would have caught it. `apiFetch` now takes `ApiPath`, derived from the generated `paths`, so an unknown
route is a compile error. Proved by removing `/catalog/personas` from the document and watching two call
sites fail to build.

Nothing the current phase serves over HTTP is missing from the document. Recruiting, progression and
operations have landed as write paths and workers with no HTTP surface, so what public-api.md lists for
them arrives with the tickets that expose them.

**Spec** [public-api.md](../../contracts/public-api.md)

---

### CTR-02 · Define the Go↔Python RPC contract and generate both sides

**Depends on** DEC-08 · **Blocks** CAT-02, EVL-01, ART-01

Typed envelopes with explicit versions, deadlines, retry semantics and failure taxonomy. Python never
returns free-form text where a schema is possible.

**Done when**
- [x] Protobuf definitions compile to Go and Python stubs in CI.
- [x] Every RPC declares timeout, retryability, idempotency and failure codes.
- [x] A failure taxonomy exists that distinguishes invalid input, provider failure and budget exhaustion.

**Spec** [internal-rpc.md](../../contracts/internal-rpc.md)

---

### CTR-03 · Define the durable event catalogue and envelope

**Depends on** DEC-08 · **Blocks** INT-02, OPS-06

Events are the record of what happened, consumed by integrations, analytics and audit. The envelope
carries tenant, actor, correlation, version and occurrence time.

**Done when**
- [x] Every event in [event-catalog.md](../../contracts/event-catalog.md) has a schema and an owner.
- [x] Envelope fields are mandatory and validated at publication.
- [x] Compatibility rules prevent a consumer being broken by an additive change.

**Spec** [event-catalog.md](../../contracts/event-catalog.md)

---

### CTR-04 · Add contract drift, compatibility and consumer tests to CI

**Depends on** CTR-01, CTR-02, CTR-03, PLT-02 · **Blocks** REL-01

A contract that can silently diverge from the implementation is documentation, not a contract.

**Done when**
- [x] Generated clients are regenerated in CI and a diff fails the build.
- [x] Backwards-incompatible changes fail unless an explicit version bump accompanies them.
- [ ] Consumer contract tests run for the web client and the Python service.

**Two of three.** The drift gate was already there and unticked: `make check-generated` regenerates
everything and fails on a diff, and it runs in the contracts job on every change. That box had been
true for some time and nobody came back to it.

The compatibility gate was genuinely missing, and it was missing for the contract with the most
consumers. Events have had `check-events` and RPC has had `check-rpc` since CTR-03; the OpenAPI
document had nothing, so removing an endpoint, dropping a required response field or making a
request field mandatory passed CI and broke a client at run time instead. `tools/apicompat` closes
it, in the same shape as its siblings and against the previous release rather than the previous
commit, per ADR-0004, so a document can be revised while in progress.

It reports what a client would actually meet: a removed path or operation, a request property that
is newly required, a request enum value no longer accepted, a response property removed or no
longer guaranteed, a changed type, and a removed success response. Each carries a remedy as well as
a reason, because a gate that says only "this is breaking" is one people learn to route around.

What it deliberately does not flag is written down rather than left silent: additions of any kind,
and a new value in a *response* enum. The last can strictly break a client with an exhaustive
switch, and flagging it would fire on almost every release while the product grows.

Every break is proven by making it and every safe change by making that too, because a gate that
fires on a safe change is a gate people ignore. It was also run against the real document: the
additions made to it this week report clean, and renaming `/auth/login` reports one break naming
the path.

The third box remains. The web client is typed from the contract and a drift fails `tsc`, which is
most of a consumer test but not one: nothing yet asserts the *server* answers the shape the client
was generated against.

**Spec** [public-api.md](../../contracts/public-api.md)
