# Epic CTR — Contracts and code generation

**Phase 1** · **Workstream** Go, Python, Web

The browser talks REST to Go. Go talks Protobuf RPC to Python. Durable facts travel as versioned
events. All three are generated from checked-in definitions, and drift fails the build. This epic is
what lets the three workstreams proceed in parallel without integrating by rumour.

---

### CTR-01 · Define the public REST contract and generate the TypeScript client

**Depends on** DEC-08 · **Blocks** every web ticket

OpenAPI is authored, linted and used to generate the browser client. Error shape, idempotency headers,
cursor pagination and version headers are in the contract, not in each handler.

**Done when**
- [ ] OpenAPI covers every route in [public-api.md](../../contracts/public-api.md) that the current phase ships.
- [ ] The TypeScript client is generated, not hand-written, and the build fails if it is stale.
- [ ] Every operation declares its error codes, idempotency behaviour and required capability.

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
