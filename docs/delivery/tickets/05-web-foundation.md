# Epic WEB — Design system and application shell

**Phase 1–2** · **Workstream** Web, Product/design

The prototype in `/screens` is the hypothesis. This epic turns it into production components with real
state, real focus management and real accessibility, and builds the permission-aware shell every
authenticated screen renders inside.

---

### WEB-01 · Implement the design system as production components

**Depends on** PLT-01 · **Blocks** every screen ticket

Tokens, typography, colour with light and dark themes, and the component set the prototype already
exercises — buttons, cards, tables that become cards, dialogs, drawers, banners, badges, evidence spans.

**Done when**
- [x] Tokens are the single source of colour, spacing, radius and motion; no component hardcodes a value.
- [x] Every component ships with keyboard behaviour, focus states and a documented accessible name.
- [x] Light and dark themes are both verified for contrast.

**In progress, and deliberately narrow.** Three components exist: `Field`, `Button` and `Banner`. Each
was added because the screen being ported needed it, rather than by working through
`screens/design-system.html` up front. A component library built ahead of any screen using it is wrong
in ways nobody finds until the first screen, and this one is 3,643 lines of prototype.

The stylesheets are ported whole from `screens/assets/css`, so the class names, the cascade and the
values are the prototype's. A test asserts that they reference only properties something defines, which
catches the failure that renders as an unstyled element rather than as an error, and pins the count of
hard-coded colours so that adding one is a decision rather than a side effect.

What the components add beyond styling is the part markup repeats and gets wrong: `Field` owns the id,
the label association, `aria-describedby` and `aria-invalid`, and keeps the hint visible when an error
appears, because an error usually means somebody did not follow the hint. `Button` owns the busy state,
because a form that can be submitted twice will be. Every component has an axe assertion, and the axe
matchers had never been registered, so the first `toHaveNoViolations` anyone wrote would have failed with
a Chai error rather than an accessibility result.

Keyboard behaviour and contrast are now asserted. Enter submits, arrow keys move within a radio group,
and tab order matches reading order. Contrast is computed from the tokens for body and secondary text in
both themes, following the `var()` indirection to the palette, and checked against the 4.5:1 threshold
the AA commitment requires. Nothing had rendered dark before this existed, so a token defined only in
light would have passed every other test and rendered as nothing.

The narrowest supported viewport is checked too: no fixed width wider than 320px, no unguarded minimum,
and the two-panel authentication layout collapsing. If it did not collapse, somebody on a small phone
could not sign in at all.

Each of these was verified by breaking it, including making dark secondary text unreadable and pointing
the dark background at the light one.

**A browser harness now covers what jsdom cannot.** Rendered contrast in both themes, layout including
overflow at 320px and at 200% text, and appearance against committed screenshots. See
[apps/web/e2e/README.md](../../../apps/web/e2e/README.md).

It found a real WCAG 2.2 violation on its first run: the prototype's checkbox control is 18px, under the
24px target-size minimum, and two stacked radios had their centres 19.5px apart, inside the spacing
exception. Fixed by spacing them, which is what the exception is for, rather than by resizing a control
the whole design system shares.

Three things about the harness were wrong before they were right, and each is recorded there because each
would silently make it useless: a reused server meant assertions ran against code no longer in the
repository, colour transitions meant contrast was measured mid-interpolation, and unpinned text rendering
gave a noise floor larger than the changes being looked for.

**Remaining.** The rest of the component set, and an end-to-end flow through the real API in a browser,
which is the only way the session cookie's SameSite behaviour is checked as a browser applies it rather
than as a Go test reads a header.

The standard itself is written down in [apps/web/src/README.md](../../../apps/web/src/README.md),
including an honest record of which files were genuinely test-first and which were not.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### WEB-02 · Build the capability-aware application shell

**Depends on** WEB-01, IAM-04 · **Blocks** every authenticated screen

Sidebar, topbar, mobile navigation and breadcrumbs, rendered from one navigation configuration driven by
capabilities — with the server remaining authoritative.

**Done when**
- [ ] Navigation renders from capabilities, and hiding an item is never the only thing stopping access.
- [ ] Skip link, landmarks and a logical heading order are present on every page.
- [ ] The shell works at 320px without horizontal scrolling.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### WEB-03 · Scope navigation for screening candidates to their invitation

**Depends on** WEB-02 · **Blocks** nothing

*Implemented in the prototype; carry it into production.* A screening candidate sees status, invitation
and consent, help and privacy, and account actions — not the practice application.

**Done when**
- [ ] Navigation, mobile navigation, notifications and the user menu are all invitation-scoped in screen mode.
- [ ] Every practice route additionally refuses the mode server-side.
- [ ] The workspace label makes the current context obvious.

**Spec** [information-architecture.md](../../product/information-architecture.md) · [screen-mode.md](../../product/screen-mode.md)

---

### WEB-04 · Implement the cross-journey state contract in shared components

**Depends on** WEB-01 · **Blocks** every screen ticket

Loading, empty, error, partial, forbidden, expired, delayed, insufficient evidence, unassessable,
reconnecting and degraded — as first-class shared states rather than per-page improvisation.

**Done when**
- [ ] Each state has a shared component and a documented content rule.
- [ ] Loading skeletons match the shape of the content they replace.
- [ ] Every error names what failed, what is still safe, the next action, and a reference identifier.

**Spec** [user-journeys.md](../../product/user-journeys.md)

---

### WEB-05 · Build the error, forbidden and no-workspace destinations

**Depends on** WEB-02 · **Blocks** nothing

403, 404, 500, expired authentication and authenticated-but-no-workspace, each distinguishable and each
offering a real way forward.

**Done when**
- [ ] The five destinations are distinct and never collapse into one generic page.
- [ ] 403 names the capability required and the one currently held.
- [ ] 500 carries a correlation identifier the support team can act on.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### WEB-06 · Port every prototype screen to Next.js with verified parity

**Depends on** WEB-01, WEB-04, CTR-01 · **Blocks** every screen ticket

The 56 file prototype in [`/screens`](../../../screens) is the design source. Porting carries over the
layout, the visual design, the copy and the interaction states. It is not an invitation to redesign, and
a screen is not ported until its states are reachable, not merely its default view.

**Done when**
- [ ] Design tokens are ported first and remain the single source of colour, spacing, radius and motion.
- [ ] Each ported screen is checked against the prototype for layout, copy and state coverage.
- [ ] Every state the prototype demonstrates is reachable in the port, including the failure and empty ones.
- [ ] Deviations from the prototype are recorded with a reason, and a research finding is the only thing that overrides it.
- [ ] Ported screens carry the component and accessibility tests required by PLT-10 before they are considered done.

**Spec** [information-architecture.md](../../product/information-architecture.md)
