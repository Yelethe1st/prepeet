# packages/contracts — hand-authored contracts

## What this owns

The OpenAPI description of the public API, the Protobuf definitions for Go to Python RPC, and the event schemas.

## What this must never do

These are the source. Nothing here is generated, and nothing generated is edited.

[ADR-0004](../../docs/architecture/decisions/0004-contract-conventions-and-code-generation.md) settles
the direction: contracts are written here first, and the Go server interface, the RPC stubs and the
TypeScript client are generated from them. A handler that does not satisfy the generated interface does
not compile, which is what keeps the document and the implementation from drifting apart.

The prose in `docs/contracts/` is the capability inventory and the reasoning behind it. This directory
is the contract.
