# platform/authz

## What this owns

The capability catalogue and the single policy evaluation used by every module. Authorization is one
decision made in one place, so a handler cannot invent its own rule.

See [ADR-0002](../../../../docs/architecture/decisions/0002-postgresql-schema-rls-and-connection-roles.md)
for the layer behind this one, and
[authorization-model.md](../../../../docs/architecture/authorization-model.md) for the rules.

## What this must never do

It never reads a database, never calls another module, and never decides from a resource identifier.
Authorization is decided from the request, so identifiers stay opaque and nothing is inferred from how
one is shaped.

It is never the only enforcement. Repository predicates, row-level security, object scoping and audit
each hold on their own, and this layer failing open must not be sufficient to leak anything.

## The rules worth knowing before adding a capability

**Deny by default.** A capability not in the catalogue is denied, so a typo fails closed rather than
skipping a check.

**Membership is not scope.** A recruiter in a tenant is not authorized over every campaign in it. A
scoped capability asked without a scope is denied, because otherwise a list endpoint would return
everything by declining to name one.

**Own-data capabilities are structural.** No tenant capability can satisfy an owner requirement,
because the subject is not the owner. That is what keeps a candidate's practice history out of employer
authority: it is not filtered, it is unreachable.

**Step-up is for what cannot be undone.** Publishing a calibration changes how candidates are evaluated
from that moment. Reducing retention destroys evidence an appeal may depend on. Both need
authentication within the last fifteen minutes, not a session opened this morning.
