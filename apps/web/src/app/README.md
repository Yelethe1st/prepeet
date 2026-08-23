# app — routes

## What this owns

Next.js routes grouped by audience: public, candidate, recruiter and platform. A route composes features; it does not implement them.

## What this must never do

A route never holds business rules, and never renders a surface the server has not authorized.
