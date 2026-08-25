# The cross-journey state contract

user-journeys.md names the states every data surface must support: loading,
empty, error, partial, forbidden, expired/revoked, delayed, insufficient
evidence, unassessable input, reconnecting/recovered, and degraded incident.
This directory is that contract as components, so the states are one
recognisable shape across the product rather than per-page improvisation.

The content rules below are the reason each component's props are required
rather than optional. A screen that cannot fill them has not yet decided what
it is telling the person, and that is the conversation to have before the
screen ships. The shared principle, in order: what is true, what is still
safe, what to do next - and claim nothing that is not so.

## The rules

**Loading** - a skeleton composed to the shape of the content it replaces,
never a bare spinner. `LoadingSurface` announces once, by name ("Loading your
sessions"), and hides the shapes from assistive technology; the shapes are
each surface's own composition of the prototype's skeleton vocabulary.

**Empty** - names the surface, explains what will appear here, and offers the
action that creates the first item. Absence is not a problem and is never
announced as one.

**Error** - the journey spec's rule verbatim, all four required: what failed,
what remains safe, the permitted next action, and a reference identifier. The
only state that interrupts (`role="alert"`).

**Partial** - the loaded content renders untouched beside a notice naming
exactly which part is missing, with a retry scoped to that part. Never
presents two-thirds of a page as the whole.

**Forbidden** - says what is closed and who can grant it. Deliberately offers
no retry: the refusal is a decision, and a button that will refuse
identically is a lie with a spinner. Full-page refusals belong to WEB-05's
destinations; this is the in-surface form.

**Expired / revoked** - says what expired, that entered work is not lost, and
offers the renewal.

**Delayed** - still running, longer than usual. Always says nothing is lost
and leaving is safe, because the spec promises no timed interaction silently
discards work - and a spinner alone teaches people to guard a page.

**Insufficient evidence** - a neutral fact with a remedy, never a score. The
prototype is explicit that an empty track must not read as "scored zero";
the component renders no number and never uses failure language.

**Unassessable input** - what could not be read, what would be readable, and
the action that provides different input.

**Reconnecting / recovered** - one component, two phases, both polite
(`role="status"`): an assertive interruption mid-interview is worse than the
blip it reports. Recovery says the connection held work safe.

**Degraded incident** - what is degraded and what still works, so a person
decides whether to continue rather than discovering the edge mid-task. Says
the incident is known; never asks the person to report it.

## What the tests hold

That each required field genuinely reaches the page; that error is the only
alert; that insufficient evidence renders no zero and no failure word; that
loading is announced once and its shapes are invisible to assistive
technology; and axe across the whole set. The browser suite owns how the
states look; these tests own what they say and to whom.
