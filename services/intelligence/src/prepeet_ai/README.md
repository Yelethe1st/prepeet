# prepeet_ai — capability packages

## What this owns

One package per capability, each with a typed input and output contract.

## What this must never do

A capability never reaches for a provider directly; it goes through the providers package so routing, fallback and budgets stay in one place.
