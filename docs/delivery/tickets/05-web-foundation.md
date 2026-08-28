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
- [x] Navigation renders from capabilities, and hiding an item is never the only thing stopping access.
- [x] Skip link, landmarks and a logical heading order are present on every page.
- [x] The shell works at 320px without horizontal scrolling.

**Navigation comes from the capabilities the session holds**, which arrive on `/me` and are derived from
role bundles on the server. Capability names come from the generated catalogue, so renaming one breaks
this at compile time rather than silently hiding a menu item forever.

The second half of that box is the part worth being explicit about: filtering here is a courtesy and not
a boundary. The server authorises every request against the same capabilities, so somebody typing an
address they cannot use is refused there. The tests assert what is offered and never assert that hiding
protects anything, because it does not, and a test claiming otherwise would be the beginning of an
unguarded endpoint behind a hidden button.

**The 320px box is checked in a real browser**, which is why the harness came first. The stylesheet-level
check cannot see overflow, and the shell is where overflow actually happens: a sidebar that fails to go
off-canvas pushes the content off the right of a small screen and every page becomes unusable rather than
cramped. Verified by breaking it, along with a long workspace name and text at 200%.

**IAM-05's switcher is inside it.** A select rather than a menu, because it is a choice between mutually
exclusive options with one in force, and it comes with keyboard behaviour and a mobile picker a custom
menu would have to reimplement. It refuses a second change while one is in flight, since two overlapping
switches can settle in either order and the interface would then show one workspace's authority while the
session is in another. A refused switch returns to what the session actually is.

**Remaining.** The rest of the component set, and the screens themselves.

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
- [x] Each state has a shared component and a documented content rule.
- [x] Loading skeletons match the shape of the content they replace.
- [x] Every error names what failed, what is still safe, the next action, and a reference identifier.

**Done.** `shared/states/` holds the eleven states as components, each content rule enforced by
required props rather than requested by convention: `ErrorState` cannot be rendered without what
failed, what is still safe, the next action and a reference identifier, which is the third box held
at the type level. The rules themselves are documented in the directory's README, in the order the
copy must run: what is true, what is still safe, what to do next - claiming nothing that is not so.
The shape rules that cannot be typed are pinned by tests instead: error is the only state that
interrupts as an alert, insufficient evidence renders no zero and no failure word, empty is never
announced as a problem, delayed always says nothing is lost and leaving is safe.

Skeletons are the prototype's vocabulary - text lines in its width steps, block, circle, shimmer
ported whole with a reduced-motion stop - composed per surface into the shape of what they replace,
which is the only honest reading of the second box since the shape is each surface's own.
`LoadingSurface` owns the half every screen gets wrong alone: announced once by name, shapes hidden
from assistive technology. Icons are deliberately absent, as they are from ErrorScreen, until a
screen needs one. The colour-utility guard learned the theme's own font sizes along the way,
because `text-md` generates CSS and the guard said it did not.

**Spec** [user-journeys.md](../../product/user-journeys.md)

---

### WEB-05 · Build the error, forbidden and no-workspace destinations

**Depends on** WEB-02 · **Blocks** nothing

403, 404, 500, expired authentication and authenticated-but-no-workspace, each distinguishable and each
offering a real way forward.

**Done when**
- [x] The five destinations are distinct and never collapse into one generic page.
- [x] 403 names the capability required and the one currently held.
- [x] 500 carries a correlation identifier the support team can act on.

**Done.** 404 and 500 are the App Router's own conventions (not-found.tsx, error.tsx), so they are
destinations nothing has to remember to link; /access-denied, /session-expired and /no-workspace
are routes, on the names information-architecture.md gives them. One shared ErrorScreen frame keeps
them distinguishable by what they say, and a matrix test renders all five asserting their headlines
differ, which is the collapse the first criterion forbids. The 403 shows the required capability
from the refusal and, from the session, the capabilities held in the same area, in capability terms
rather than a role name because capabilities are what the decision was made on; the error-envelope
field that will carry the requirement server-side lands with the first capability-gated endpoint.
The 500's reference is the error digest, which appears beside the stack in the server log. Two
prototype claims did not port because nothing exists behind them: the screen-index search on the
404, and the 500's promise that an engineer was already paged - the screen claims only that the
failure is in the logs under the quoted reference.

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

**In progress.** The front page is ported, `screens/index.html` whole: ten sections in the
prototype's order, its copy, the live-interview still, the practice-versus-screen visibility table,
the worked evidence cards and the eight questions. It lives in `features/marketing`, composed by a
route thin enough to be nothing but composition, with its copy in one content module so a change to
the words is a change in one place.

The design tokens finally tell the truth. `tokens.css` had named Figtree, Fraunces and JetBrains Mono
since WEB-01 and nothing had ever fetched them, so every screen in the product rendered in system-ui
and Georgia. `next/font` now downloads them at build time and serves them from our own origin, which
is also why every visual baseline in this commit changed: the port is the reason it was noticed, not
the only screen it fixes.

Deviations, all recorded where they were made and two of them enforced by tests. No em dashes, per
the copy rule, so every one is a colon, a comma or a full stop. No link to a screen that is not
ported: the prototype links freely across 56 files, and a dead link from the front page is the worst
place in the product to have one, so an unported destination becomes an anchor to the section that
answers the same question, or is dropped. The visibility table stays a table at every width and
scrolls inside its own box, rather than being restyled into cards by `display: block`, which strips
the row and column relationships that make it navigable. A tagged evidence span explains itself in
text rather than in a tooltip a touch user never sees. The theme toggle has a name and says which
theme it switches to, rather than announcing itself as pressed and nothing else.

Three things the browser found that no unit test could. At 320px the header was eight pixels wider
than the screen, so the theme toggle moves into the menu there. The table's scroll container could
not be focused, which put its last two columns out of reach of a keyboard on a phone. And the
persona avatar in the interviewer's own colour missed 4.5:1 in the dark theme. `/` is in the
accessibility and layout sweeps now, at both widths and in both themes, which is what caught all
three.

Icons arrive with it. WEB-04 recorded that they stay out until a screen needs one, and this screen
draws thirty: `lucide-react`, the React build of the set the prototype uses, behind one `Icon`
component that fixes the sizes and hides every glyph from assistive technology, because every icon
here sits beside the words it illustrates.

No visual baseline for this page, deliberately, and for the reason the appearance suite already
gives for covering two of IAM-02's screens rather than all seven: a full-page screenshot of a
six-thousand-pixel marketing page at zero tolerance is the highest-maintenance and lowest-signal
baseline in the suite, and this page has no interactive state the other two tiers miss.

**Spec** [information-architecture.md](../../product/information-architecture.md)
