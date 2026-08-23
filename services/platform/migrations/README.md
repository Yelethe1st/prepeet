# migrations — database schema

## What this owns

Forward-only, reviewed migrations. Every tenant-scoped table is created with its row-level security policy in the same migration that creates the table.

## What this must never do

A migration never creates a tenant-scoped table without RLS, and never edits an applied migration in place.
