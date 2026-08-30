# ADR-0020: Screening disclosure, candidate access and appeal, parameterised by jurisdiction

**Status:** Accepted for the mechanism; the legal determination it carries is open
**Owner:** olabode omoyele
**Decision date:** 2026-08-30
**Review date:** 2027-02-28
**Supersedes:** None
**Superseded by:** None

Answers three of DEC-11's four criteria and deliberately does not answer the
first. DEC-11 asks for a legal determination per launch jurisdiction with named
approvers. That is counsel's to give and a named person's to sign, and an ADR
that invented one would be worse than no ADR: it would make an unanswered
question look answered, in the one area of this product where that is most
dangerous.

What this ADR fixes is everything that holds whichever way each determination
goes, plus the behaviour of the platform while a determination is missing. That
unblocks the whole of epic SCR and epic REV as engineering work, without
pre-empting the legal answer or letting a jurisdiction go live without one.

## Context

[screen-mode.md](../../product/screen-mode.md) and
[responsible-hiring.md](../../security/responsible-hiring.md) both name the same
three open questions and both mark themselves as pending jurisdiction-specific
approval: what a candidate may see of their own evaluation, whether appeal is a
legal right or a product feature, and who owns each answer. Both list an
unresolved legal disclosure as a stop-ship.

The specifications are also explicit about one thing that is not open: route
guards and API policy must enforce whatever disclosure is selected, and the UI
must not rely on hiding links. That is an engineering requirement independent of
the legal answer, and it is the part that has to exist before any screening
surface is written, because retrofitting server-side policy onto a UI that
assumed it was the gatekeeper is how the guarantee gets lost.

DEC-11 blocks epics SCR and REV in their entirety: 17 tickets, none started. The
cost of waiting for the determination before building anything is 17 tickets of
idle critical path. The cost of building without a decision is a codebase that
assumes one jurisdiction's answer and has to be unpicked. This ADR takes the
third route: build the machinery that every answer needs, make the answer a
recorded input rather than an assumption, and refuse to operate where the input
is absent.

## Decision

### The determination is data, not code

A jurisdiction determination is a stored, versioned record naming the
jurisdiction, the four policy positions below, the approver, and the date. It is
read at run time. No screening behaviour is compiled against a particular
jurisdiction's answer, and no default is baked into a handler.

This is the same shape as the pinned artifact model the rest of the platform
already uses: the campaign pins the determination version it was opened under,
exactly as it pins its rubric and calibration, so a determination that changes
later does not retroactively alter a running campaign or a completed interview.

### Absent a determination, screening does not run

A campaign cannot be opened in a jurisdiction with no recorded determination.
Not a warning, not a permissive default: a refusal, at the point of opening,
naming the missing jurisdiction.

This is the structural half of the decision and the reason the mechanism can
ship before the legal work. A missing determination is the normal state today,
in every jurisdiction, so the refusal is what makes it safe to build and merge
the whole of SCR while the legal question is still open. DEC-11's first
criterion stays open, and the code enforces that it is open rather than
documenting it.

### Result disclosure is server-side policy

What a candidate may see of their own evaluation is one of the determination's
four positions. It is enforced by route guards and API policy on every read
path, and the enforcement is tested by requesting the resource directly rather
than by checking that a link is absent. Hiding a link is not a control, and a
test that asserts a link is hidden proves nothing about the endpoint behind it.

The positions the platform supports are, from most to least disclosing: the
full evaluation as the reviewer sees it, the evidence summary without the
suggested band, completion status only, and nothing beyond confirmation of
submission. A determination names one. Absent a determination there is no
campaign, so there is no fifth default.

### Consent is unbundled, in every jurisdiction

Required processing and optional processing are separate records with separate
acceptances. Declining optional processing never blocks the interview and never
degrades the evaluation. This is not parameterised: it is the strictest
position, it is stricter than any launch jurisdiction is expected to require,
and making it uniform removes a class of bug where a jurisdiction's looser rule
becomes the code path everyone shares.

Model improvement is optional processing by definition and may never be a
condition of taking a screening interview.

### Disclosure is versioned and acceptance records the version

A candidate accepts a specific disclosure version, and that version is stored
with the acceptance. Changing the disclosure creates a new version and never
rewrites what someone already accepted. A disclosure version is immutable once
any acceptance references it.

This ADR fixes the mechanism and the immutability rule. It does not supply the
approved disclosure text, which is part of DEC-11's second criterion and needs
the same approver the determination does.

### Appeal defaults to a right

Appeal status is a determination position with three values, as DEC-11 frames
it: a legal right, a tenant option, or platform policy. Absent a determination
the platform treats appeal as a right, and since absent a determination there is
no campaign, this default is what a new determination is drafted against rather
than something that silently takes effect.

The default is the strictest of the three deliberately. If the determination for
a jurisdiction turns out to be looser, loosening a right that was honoured is a
product change; discovering that a right was owed and not offered is a
compliance failure with candidates already affected.

## What this hands to the tickets

- **SCR-01** pins the determination version into the campaign alongside the
  rubric and calibration, and refuses to open a campaign in a jurisdiction with
  no determination.
- **SCR-02** builds versioned disclosure with immutable versions, acceptance
  recording the version, and separate required and optional consent records.
- **SCR-07** enforces the result-disclosure position server-side on every read
  path, tested by direct request.
- **REV-06** reads the appeal position rather than assuming one, and treats the
  absence of a determination as unreachable rather than as permission.

## Consequences

- Epic SCR and epic REV can be built now. Neither can be launched in any
  jurisdiction until that jurisdiction has a recorded determination, which is
  the intended outcome and matches both specifications' stop-ship lists.
- The legal work becomes a data-entry task against a defined shape rather than a
  prerequisite for design. Counsel is asked four specific questions per
  jurisdiction instead of being asked to review a system.
- A determination change is a new version, so it never alters a running campaign
  or rescores a completed interview.
- DEC-11's first criterion remains open and is now enforced by the code. Its
  second remains open for the approved text. Its third and fourth are answered
  here.

## Revisit when

The first jurisdiction determination is recorded and the four positions meet
real legal advice; a jurisdiction requires a disclosure position this ADR does
not model; or DEC-15's retention schedules change what a candidate may be told
about how long their evaluation persists.
