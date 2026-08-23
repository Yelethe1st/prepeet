# apps/web — Next.js application

## What this owns

The browser experience for candidates, recruiters and operators. Routes, feature modules, the ported design system, and the browser side of realtime.

## What this must never do

It never talks to PostgreSQL, Temporal, object storage or the Python service directly. Every read and write goes through the Go API. It never treats hidden navigation as authorization.
