# features — journey modules

## What this owns

A feature owns one journey: its components, its state, and the calls it makes.
`auth` owns signing in, registering and knowing who is signed in. `shell` owns
navigation, the workspace switcher and signing out.

## What this must never do

**A feature never imports another feature.** Not its components, not its state,
not even its types. Two features that import each other are one feature with a
directory between them, and the cost is paid later, when one has to change and
the other breaks.

What a feature needs from elsewhere it declares itself, as `AppShell` declares
`ShellUser`, and the route supplies it. That is the same pattern the Go services
use for the same reason: the consumer names the narrow thing it needs, and the
composition root wires the two together.

It is also less coupling than it looks. `ShellUser` is a subset of the session,
so anything added to a session does not become something the shell can quietly
start depending on.

## Where the boundary is enforced

`src/architecture.test.ts`, which fails naming the file and the import. It was
written because this rule was already broken while being stated here, and prose
does not fail a build.
