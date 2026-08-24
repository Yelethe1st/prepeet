# lib — cross-cutting browser concerns

## What this owns

Things every feature needs and none of them owns: the API client, and later
browser observability and the realtime transport.

## What this must never do

Nothing here renders. If it renders it belongs in a feature or the design
system.

That rule is why the session provider is not here, although an earlier version
of this file claimed it was. Knowing who is signed in is the auth feature's
concern and it renders a provider, so it lives in `features/auth/session.tsx`.
Anything else that needs the session is given what it needs by the route, rather
than reaching into that feature for it.
