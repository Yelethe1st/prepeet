# test — the tests that belong to no one file

## What this owns

Tests about the codebase rather than about a component or a page. A test that
exercises one module lives beside it; these have no single subject to sit next
to.

- `architecture.test.ts` — the boundaries the READMEs in this tree describe.
  They were prose until this existed, and two of them were already broken.
- `theme.test.ts` — both themes and their contrast, computed from the tokens.
- `tailwind.test.ts` — that the Tailwind configuration maps only tokens that
  exist, and that no component hard-codes a colour.

## What belongs here and what does not

Here: anything whose subject is the tree, the configuration, or a rule that
spans files.

Not here: a test for a component, a page, a hook or a module. Those go next to
what they test, so that moving the thing moves its test and deleting the thing
leaves no orphan.

## What these cannot do

They read files. `theme.test.ts` computes contrast from token values, which
catches a colour edited badly and not a component putting quiet text on a raised
surface. `tailwind.test.ts` checks the mapping, and cannot see a mistyped
utility, because Tailwind emits nothing for one rather than complaining.

Both of those are answered in `e2e/`, against a real browser. What is here is
the part that can be known without rendering.
