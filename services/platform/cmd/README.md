# cmd — deployable entry points

## What this owns

Three binaries: api serves the browser, worker runs Temporal workflow and activity workers, migrate applies database migrations.

## What this must never do

An entry point wires dependencies and starts a process. It never holds domain logic.
