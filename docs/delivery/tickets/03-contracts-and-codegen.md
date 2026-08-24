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
- [ ] OpenAPI covers every route in [public-api.md](../../contracts/public-api.md) that the current phase ships. Health and authentication are done.

A limitation of the Go generator is recorded here rather than left to be rediscovered. A response header
is generated as a single field written with `Header().Set`, so an operation setting two cookies cannot be
served by the generated response type. IAM-01 writes those three responses by hand against the generated
interfaces. The contract is not adjusted to suit the generator: it describes the wire, and the wire has
two cookies.
- [ ] `oasdiff` runs against the previous release once there is a previous release.
- [ ] Every operation declares its required capability, which needs the capability catalogue published as a contract in IAM-04.
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

**Spec** [public-api.md](../../contracts/public-api.md)

---

### CTR-02 · Define the Go↔Python RPC contract and generate both sides

**Depends on** DEC-08 · **Blocks** CAT-02, EVL-01, ART-01

Typed envelopes with explicit versions, deadlines, retry semantics and failure taxonomy. Python never
returns free-form text where a schema is possible.

**Done when**
- [ ] Protobuf definitions compile to Go and Python stubs in CI.
- [ ] Every RPC declares timeout, retryability, idempotency and failure codes.
- [ ] A failure taxonomy exists that distinguishes invalid input, provider failure and budget exhaustion.

**Spec** [internal-rpc.md](../../contracts/internal-rpc.md)

---

### CTR-03 · Define the durable event catalogue and envelope

**Depends on** DEC-08 · **Blocks** INT-02, OPS-06

Events are the record of what happened, consumed by integrations, analytics and audit. The envelope
carries tenant, actor, correlation, version and occurrence time.

**Done when**
- [ ] Every event in [event-catalog.md](../../contracts/event-catalog.md) has a schema and an owner.
- [ ] Envelope fields are mandatory and validated at publication.
- [ ] Compatibility rules prevent a consumer being broken by an additive change.

**Spec** [event-catalog.md](../../contracts/event-catalog.md)

---

### CTR-04 · Add contract drift, compatibility and consumer tests to CI

**Depends on** CTR-01, CTR-02, CTR-03, PLT-02 · **Blocks** REL-01

A contract that can silently diverge from the implementation is documentation, not a contract.

**Done when**
- [ ] Generated clients are regenerated in CI and a diff fails the build.
- [ ] Backwards-incompatible changes fail unless an explicit version bump accompanies them.
- [ ] Consumer contract tests run for the web client and the Python service.

**Spec** [public-api.md](../../contracts/public-api.md)
