# lib — cross-cutting browser concerns

## What this owns

What every feature needs and none of them owns, in the four areas the
architecture brief names:

- `api` — the client for the Prepeet API, typed from the contract.
- `auth` — the session: who is signed in, what they may do, and the calls that
  change it.
- `observability` — browser telemetry, when it lands.
- `realtime` — the interview transport, when it lands.

## What this must never do

**Nothing here presents.** A provider is fine: it renders its children and
nothing of its own, and a session is plumbing whichever way it is expressed. But
nothing in lib may import the design system, because something reaching for a
Button is something that should have been a feature.

**Nothing here imports a feature or a route.** lib is what features are built
on, so it cannot be built on them.

The request shapes here come from the generated contract rather than from the
components that send them. An earlier version imported them from the forms,
which made the network layer describe itself in terms of a screen.

## Where the boundary is enforced

`src/architecture.test.ts`.
