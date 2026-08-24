# app — routes

## What this owns

Next.js routes, grouped by whether a session is needed.

`(auth)` holds the screens somebody reaches without one, and its layout brings in
the ported stylesheets. `(app)` holds everything behind a session, and its layout
is where the session is resolved and the shell is rendered, so no page under it
repeats either.

## What this must never do

A route never holds business rules, and never renders a surface the server has
not authorised. What it renders being filtered by capability is a courtesy so
that nobody is offered a control that will refuse them; the server decides.

A route composes and does not implement. A page holding a form's logic is a page
that cannot be tested without a router, which is how route files end up untested
and then excluded from coverage for being untestable. That happened here once
already, and the exclusion is gone.

## What routes are allowed that features are not

Seeing everything. `app/` is the composition root, in the way `cmd/` is on the
server: it may import any feature and hand one feature's state to another, which
is precisely what a feature may not do for itself.
