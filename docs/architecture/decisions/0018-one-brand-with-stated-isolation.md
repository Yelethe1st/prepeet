# ADR-0018: One brand, with the isolation guarantee stated where the modes meet

**Status:** Accepted; validated with candidates in PRC-06  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-18. The ticket asks for candidate research rather than internal
preference; this ADR records the decision, the structural facts that
support it, and the deliberate deviation that the research lands in PRC-06
(the validate-with-candidates ticket) instead of preceding the decision.
If that research contradicts this decision, the research wins and this ADR
is revised before screening launches.

## Context

Practice and screening could ship as one brand or two. One brand is
operationally simpler and compounds trust in a single name; the risk is a
candidate who practises on Prepeet reasonably fearing their practice
history reaches an employer who screens on Prepeet. Splitting brands
treats the fear; it does not treat the fact, and it doubles every surface,
policy page and support channel while quietly implying the isolation is
too weak to say out loud.

## Decision

**One brand.** The trust problem is answered with the guarantee itself,
which is structural, not editorial:

- A practice session cannot acquire a tenant: the schema CHECK on
  `interview.sessions` makes practice-with-tenant an unrepresentable row,
  and every interview-family table repeats it.
- Row-level security gives tenants no path to candidate-owned practice
  rows; the policies are FORCEd and the build fails if a table forgets.
- The billing, evidence, results and contradiction stores all carry the
  same dual-scope shape, proven by cross-candidate invisibility tests.

### The copy rule

Wherever the two modes meet on a candidate surface (the practice
dashboard when an invitation exists, the invitation landing page, the
screening consent screen), the isolation guarantee is stated in candidate
words: **practice history, recordings, transcripts and results are never
visible to any employer, and screening evaluations never mention that
practice happened.** This is a content rule handed to WEB-02 and the SCR
disclosure surfaces, with the same server-supplied-copy pattern EVL-04
established so no screen can omit it.

### The validation

PRC-06 adds the brand-trust question to its candidate sessions: shown the
isolation copy, does a candidate believe it, and does the shared brand
change what they say in practice? A negative result reopens this
decision before screening launches, while the two-brand option is still
cheap.

## Consequences

- WEB-02 and PRC-01 unblock now.
- The isolation guarantee becomes load-bearing marketing copy, which
  means the structural tests behind it are load-bearing too; weakening
  them is henceforth a product decision, not a refactor.
