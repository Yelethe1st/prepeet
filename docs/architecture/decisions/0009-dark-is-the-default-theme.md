# ADR-0009: Dark is the default theme, against the prototype

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Recorded because [architecture-and-implementation-brief.md](../architecture-and-implementation-brief.md)
requires that deviations from the `/screens` prototype carry a reason.

## Context

The prototype is the design source and the production interface is a port of it. It declares light
on 53 of its 57 pages; dark exists, is complete, and is not what a page renders unless asked.

The brief is deliberately asymmetric about who may override the prototype: research findings may,
developer preference may not. This decision has to be held to that line or it is exactly the kind of
change the rule exists to stop.

## Decision

**The application renders dark unless somebody has chosen otherwise.** Light remains complete,
reachable, and sticky once chosen.

The reason is the workload rather than a preference about how the product should look. A practice
interview is a long, concentrated session held on a screen the person is talking at, and a
meaningful share are taken in the evening around other commitments, which is the point of practising
alone rather than with somebody. That is the condition dark serves, and it is a property of what
this product is for rather than of this product's taste.

**The operating system's `prefers-color-scheme` is deliberately not consulted.** This is the part
worth arguing, because following the system is the usual advice. Most systems report light, most
people never change that setting, and honouring it would make the default light for nearly everyone
while appearing to be a considered choice. A default that quietly evaluates to the opposite of the
decision is worse than either option chosen outright.

**An explicit choice always wins and persists**, in `localStorage` under `prepeet.theme`. The root
layout renders the attribute and an inline script in `<head>` applies the stored choice before
paint, so somebody who chose light does not see a dark flash on every navigation.

## Consequences

- Dark is now the theme that must be right, so it is the one the contrast checks and the visual
  baselines are read in first. It has already earned this: an authentication panel painted with a
  semantic foreground token inverted with the theme and rendered light-on-light, which the browser
  contrast check caught and no token-level test could.
- The port is otherwise unaffected. Both themes come from the same ported tokens, so this changes
  which block applies rather than what any value is.
- A theme control has to exist, be reachable, and be tested, which would not have been true if the
  prototype's default had been kept and dark left as a preference nobody surfaced.

## What would change this

- Usage showing sessions concentrated in daytime hours, or a light-choice rate high enough that the
  default is working against most people.
- Accessibility feedback that dark is the worse default for the readers who most depend on the
  interface, which some low-vision conditions genuinely make true.
- Screening recruiters, whose workload is short and daytime, becoming the larger audience. That
  would argue for a default that differs by journey rather than for flipping this one.
