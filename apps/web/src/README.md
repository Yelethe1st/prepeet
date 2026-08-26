# Testing the web application

The same standard as the services: a test first, and a suite that is
comprehensive rather than a happy path. This file records what that means here,
because "exhaustive" is otherwise a word everybody agrees with and nobody can
check.

## What was and was not test-first

Recorded honestly, because a claim about process is worth less than the record.

Test-first: `lib/api/client.ts`, `shared/components/Field.tsx`,
`features/auth/SignInForm.tsx`, `features/auth/RegisterForm.tsx`, the whole of
`shared/states/`, `features/profile/facts.ts` and `CvSection.tsx`,
`features/interview/rules.ts` and `Wizard.tsx`, `features/prepare/gate.ts`
and `PrepareScreen.tsx`, and `features/members/matrix.ts` and
`MembersScreen.tsx`. The test was written, run red, and then satisfied.

Written together: `Button.tsx`, `Banner.tsx`, `features/profile/api.ts`,
`features/interview/api.ts`, `features/prepare/api.ts` and `checks.ts` - the
check runners' hardware behaviour is untestable in jsdom and is faked in the
screen's suite; the browser harness is where it gets proven.

Test-after: `features/auth/api.ts`, `features/auth/AuthShell.tsx`, the stylesheet
and theme tests, and both route pages. The pages are the worst of these: they
were untested and excluded from coverage as "composition points with no logic",
which stopped being true the moment one decided where to navigate after signing
in. The exclusion is gone.

## What every piece of UI is expected to assert

Not a checklist to satisfy mechanically. Each of these exists because leaving it
out has a specific consequence.

**Behaviour, from the outside.** Query by role and label, as somebody using the
thing would. A test that reaches for a class name passes when the component is
broken and fails when it is restyled, which is backwards.

**The failure paths, not only the success.** Every form asserts what happens
when the server refuses, when the network fails, and when something throws that
is not either. The last one is the path that shows the exception's own text to a
person if nobody checks.

**What must not be shown.** A rejected sign-in asserts the absence of words that
would say whether the account exists. A form asserts the password is not in the
serialised DOM. Absence is not covered by coverage: a line that renders nothing
is still a line that ran.

**Double submission.** A form that can be submitted twice will be, on a slow
connection. Harmless for a login and a second billed session for an interview.

**Accessibility, per component and per page.** `toHaveNoViolations` from
vitest-axe, plus the things axe cannot see: that a label names its input, that an
error is announced when it appears, that a hint survives an error, and that
tabbing reaches every control in reading order.

**Keyboard.** Enter submits. Arrow keys move within a radio group. Tab order
matches reading order. Most people who type fast never touch the mouse.

**Both themes.** Contrast is computed from the tokens for body and secondary
text, in light and dark. Nothing rendered dark until this existed, so a token
defined only in light would have passed every other test and rendered as nothing.

Since the move to Tailwind that check is necessary and no longer sufficient: it
sees the tokens, not what a component puts on what. A panel painted with a
semantic token that inverts with the theme passes it and is unreadable, which
happened. The browser suite is what catches that.

Dark is the default, which is the one place the product departs from the
prototype: `/screens` declares light on 53 of its 57 pages. An interview is a
long, concentrated session and many people take one in the evening. An explicit
choice wins and persists; the operating system's `prefers-color-scheme` is
deliberately not consulted, because most systems report light and following it
would make the default light in practice while claiming otherwise.

**The narrowest viewport.** 320px, checked against the stylesheets: no fixed
width wider than the viewport, no unguarded minimum, and the two-panel
authentication layout collapsing. If it did not, somebody on a small phone could
not sign in at all.

## One thing about the harness

`localStorage` is supplied by `vitest.setup.ts` rather than by the environment.
jsdom provides one, and Node 26 ships an experimental global that is unavailable
unless the process was started with `--localstorage-file`; Node's wins, so
`window.localStorage` is undefined and any test touching a stored preference
fails with "Cannot read properties of undefined" rather than with anything about
storage.

What happens when storage is genuinely unavailable, which is a private window or
blocked site data, is asserted separately by stubbing one that throws.

## The coverage floor

95% lines, statements and functions; 90% branches. Raised from 80 once the suite
sat near 98, because a floor far below where the suite actually is permits a
large regression without failing, which is the opposite of what it is for.

The floor is not the standard. It passed at 88% while two files sat at zero,
which is the shape of number to distrust: an aggregate hides a wholly untested
file. Read the per-file column.

## The browser tests

The three things this file used to list as missing are covered now, in
[`e2e/`](../e2e/README.md), because none of them can be done in jsdom: it has no
layout engine and does not apply stylesheets, so a 900px box measures zero and a
colour set by a class comes back as the browser default.

Rendered contrast, layout including overflow at 320px and at 200% text, and
appearance compared against committed screenshots. Run them with
`make test-browser`.

They are separate from this suite rather than folded into it because they cost
seconds and need a built server. Keep asserting behaviour here, where it is
milliseconds; put anything that depends on layout or colour there, because here
it will pass whatever the answer is.

## What is still missing

An end-to-end flow through the real API. The browser tests serve the built
application and stub the network where a response is needed, which is right for
what they assert. Nothing yet drives sign-in through the Go service in a real
browser, which is the only way the session cookie's `SameSite` and `HttpOnly`
behaviour is checked as a browser applies it rather than as a Go test reads a
header. That belongs with the first flow worth testing that way.
