# services/platform — Go control plane

## What this owns

The authoritative product state, the public API, authorization, persistence, lifecycle, audit and integration delivery. Go is the only writer of durable product state.

## What this must never do

It never returns a value produced by the Python service without validating it first, and it never lets a module reach into another module's tables.
