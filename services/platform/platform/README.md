# platform — infrastructure adapters

## What this owns

Shared adapters for PostgreSQL, Temporal, observability, object storage, email and cryptography.

## What this must never do

An adapter never contains a product rule, and never assumes which bounded context is calling it.
