# Testing the web application

The same standard as the services: a test first, and a suite that is
comprehensive rather than a happy path. This file records what that means here,
because "exhaustive" is otherwise a word everybody agrees with and nobody can
check.

## What was and was not test-first

Recorded honestly, because a claim about process is worth less than the record.

Test-first: `lib/api/client.ts`, `design-system/components/Field.tsx`,
`features/auth/SignInForm.tsx`, `features/auth/RegisterForm.tsx`. The test was
written, run red, and then satisfied.

Written together: `Button.tsx`, `Banner.tsx`.

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

**The narrowest viewport.** 320px, checked against the stylesheets: no fixed
width wider than the viewport, no unguarded minimum, and the two-panel
authentication layout collapsing. If it did not, somebody on a small phone could
not sign in at all.

## The coverage floor

95% lines, statements and functions; 90% branches. Raised from 80 once the suite
sat near 98, because a floor far below where the suite actually is permits a
large regression without failing, which is the opposite of what it is for.

The floor is not the standard. It passed at 88% while two files sat at zero,
which is the shape of number to distrust: an aggregate hides a wholly untested
file. Read the per-file column.

## What is still missing

Stated rather than implied.

**Rendered contrast.** The theme test computes ratios from tokens, which catches
a colour edited badly and not a component that puts one token on another in a
way nobody checked. That needs a browser.

**Rendered layout.** The 320px test reads stylesheets. A component can satisfy
every rule here and still overflow because of content.

**Visual regression.** Nothing compares what a screen looks like to what it
looked like. WEB-01 owes this.
