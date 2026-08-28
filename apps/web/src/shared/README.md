# shared — the ported design system and what every screen uses

## What this owns

- `styles/tokens.css` — the ported token file. The prototype's names, values and
  its light and dark blocks, unchanged. `/screens` is the design source and this
  is the port of it.
- `styles/theme.css` — Tailwind, configured from those tokens. The whole build
  configuration; there is no `tailwind.config`.
- `components/` — the components every feature uses, styled with utilities over
  those tokens.
- `themePreference.ts` — which theme renders, and why it defaults to dark.

The three faces the tokens name are fetched at build time and served from this
origin, wired up in `app/layout.tsx` with `next/font`. They are declared there
rather than here because the variables have to land on the document element,
which only the root layout renders. `tokens.css` names both the variable and the
family, so a document rendered without that layout still asks for the right font
before falling back.

It was called `design-system` while it was a stylesheet port with a few
components around it. Since the styling moved to Tailwind, most of what is here
is the shared component set and the theme it is built on, so it is named for
that.

## How the port and Tailwind fit together

The architecture brief names Tailwind and `/screens` is the design source, and
both are satisfied rather than one at the other's expense. `tokens.css` is still
the ported file. `theme.css` maps those tokens into Tailwind's namespace, so
`bg-surface` resolves through `--surface` and a change to the palette arrives
everywhere without a component being touched.

`@theme inline` is what makes that work, and it is not optional. A plain `@theme`
copies the token's value into every utility at build time, so `bg-surface` would
carry the light colour and stay light when `data-theme` changes. `inline` emits a
reference instead.

## What this must never do

**It never invents a value.** A colour, spacing step or radius not in
`tokens.css` does not exist, which is now enforced by there being no utility for
it. Tailwind's arbitrary-value syntax is the way round that, so a test refuses a
raw colour in any component.

**It never encodes a product rule.** A component knows how something looks and
behaves, never who may see it.

**It never imports a feature or a route.** Something here that needed one would
be a component only one screen can use.

**Semantic tokens first.** The palette is exposed because a few surfaces are
deliberately theme-independent, and the authentication side panel is the case
that proved it: painted with `bg-fg` it inverted with the theme and became
light-on-light in dark mode, which the browser suite's contrast check caught and
nothing else could.

## What a test here can and cannot see

Tailwind does not complain about a utility it does not recognise. `bg-surfce`
generates no CSS, no warning, and renders unstyled. Nothing in the jsdom suite
catches that; the browser suite does, because an unstyled element fails contrast
or the screenshot or both.
