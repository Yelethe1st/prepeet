# Browser tests

These exist because jsdom cannot see three things, and all three are
requirements rather than nice to have.

Run `make test-browser` from the repository root.

## Why a browser at all

jsdom has no layout engine and does not apply stylesheets. Both are easy to
demonstrate: a 900px box measures zero there, and a colour set by a class comes
back as the browser default rather than the ported value. So:

**Contrast as rendered.** `src/test/theme.test.ts` computes ratios from
token values for the pairs somebody chose to check. It cannot see a component
putting quiet text on a raised surface, which is a pair nobody chose.

**Layout.** Nothing outside this suite measures anything. `tailwind.test.ts`
refuses an arbitrary colour, which is a different question; a fixed width, a
`nowrap`, a long email address with no break opportunity or a flex child that
will not shrink all pass every static check and are how a page actually ends up
with a horizontal scrollbar. Adding `whitespace-nowrap` to the inputs leaves the
jsdom suite green and fails this one.

**Appearance.** Nothing else compares what a screen looks like to what it looked
like.

## What it found immediately

The `target-size` rule, which is WCAG 2.2 AA. The prototype's checkbox and radio
control is 18px, under the 24px minimum, and two stacked radios had their centres
19.5px apart, which is inside the 24px spacing exception. Fixed by spacing them,
which is what the exception is for, rather than by resizing a control the whole
design system shares.

## Three things about the harness, each of which was wrong first

**The server must never be reused.** `reuseExistingServer` was on locally, and a
server left running from an earlier build was reused, so every assertion was made
against code no longer in the repository. A deliberately broken stylesheet passed
at zero tolerance twice, and both times the conclusion drawn was about the
tolerance rather than the server. It is off everywhere now, and a run costs a
build.

**Motion has to be off.** The components transition colour. Switching theme and
asking axe about contrast measured colours mid-interpolation: white text halfway
to the primary background reads as `#97a2a2`, fails 4.5:1, and is not a colour
the product renders. Three of the first four failures were that.

**Text rendering has to be pinned.** Without `--disable-lcd-text` and
`--font-render-hinting=none`, an unchanged page differs from its own baseline by
about 130 pixels between runs, purely from subpixel antialiasing. That noise
floor is larger than many real changes: squaring the corners off a button alters
roughly thirty. With the flags the difference between runs is zero, so the
tolerance is zero and any difference is a real one.

## Baselines

Screenshots differ between operating systems, so Playwright names them per
platform and a machine without a baseline for its own platform fails rather than
comparing against somebody else's rendering.

- `make test-browser-update` accepts the current appearance on your machine.
  Review the image diff, not the test result: an accepted baseline is a decision
  about how the product looks.
- `make test-browser-baselines` regenerates the Linux ones in the same container
  CI runs, which is the only way they stay usable there.

## What is still not covered

The tests are per route, and a route is added to the list in each spec as it is
ported. Nothing here checks a screen that does not exist yet, and nothing here
runs against the real API: these serve the built application with the network
stubbed where a response is needed. An end-to-end flow through the Go service
belongs with the first flow worth testing that way.
