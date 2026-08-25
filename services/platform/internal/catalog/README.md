# catalog — the interview catalogue

## What this owns

Disciplines, roles, interview shapes and personas, and which combinations of
them a session may be built from. Implements CAT-03.

## What this must never do

**It never knows a profession's name.** The catalogue is a registry artifact
(ADR-0011), authored in git, published by `contentctl`, resolved per tenant
at serve time. Adding a profession - or a whole discipline - is publishing a
new version of the document; nothing in this package or any binary changes.
A hardcoded list here would quietly restrict the product to whatever the
deploy knew about, which is exactly what the ticket forbids.

**It never serves an incoherent document.** Every cross-reference is checked
at parse - a role in no discipline, a ghost shape, a duplicated identifier, a
shape with no runnable length - and the loader runs this same parse as the
artifact's validating step, so an incoherent catalogue never publishes at
all. Parsed documents are cached by digest, which is safe to never
invalidate: a new version has a new digest by construction.

**It never trusts the browser's filtering.** The wizard may hide options the
catalogue does not combine; `Validate` refuses them anyway, field by field
with stable codes, because hiding is a courtesy and the server is the rule.
Its enforcement endpoint is POST /interviews, which arrives with CAT-04.

**A persona is a style, never a judgement.** Pacing, follow-up pressure,
silence. The same rubric and the same evidence standard apply whichever one
is chosen, and nothing here may vary by candidate.
