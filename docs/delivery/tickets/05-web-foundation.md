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
- [ ] Tokens are the single source of colour, spacing, radius and motion; no component hardcodes a value.
- [ ] Every component ships with keyboard behaviour, focus states and a documented accessible name.
- [ ] Light and dark themes are both verified for contrast.

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
