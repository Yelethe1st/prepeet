# features — journey modules

## What this owns

Feature modules organised by journey, each owning its data access, state and components.

## What this must never do

A feature never imports another feature's internals. Shared behaviour moves to lib or the design system first.
