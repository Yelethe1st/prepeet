# Prepeet Greenfield Documentation

**Status:** Proposed implementation baseline  
**Audience:** Principal Engineer, product, design, engineering, security, legal, operations, and delivery teams  
**Last updated:** 2026-08-23

This is the standalone specification set for building Prepeet from scratch with Go, Python, and Next.js/React. It assumes no knowledge of any existing codebase.

The high-fidelity mockups in `/screens` informed the route map, interaction states, accessibility requirements, and product assumptions. Mockup behavior remains proposed until promoted into an approved requirement.

The [coverage manifest](COVERAGE.md) maps the earlier consolidated subjects to their canonical split files and distinguishes real remaining implementation artifacts from omissions.

## Status vocabulary

| Label | Meaning |
|---|---|
| Required | Invariant unless explicitly changed through governance |
| Proposed | Recommended starting decision requiring validation |
| Open | Material decision requiring an owner before affected release |
| Deferred | Intentionally outside the initial release |

## Reading order

1. [Product requirements](product/product-requirements.md)
2. [Practice mode](product/practice-mode.md) and [screen mode](product/screen-mode.md)
3. [User journeys](product/user-journeys.md) and [information architecture](product/information-architecture.md)
4. [Architecture and implementation brief](architecture/architecture-and-implementation-brief.md)
5. Domain architecture documents under `architecture/`
6. Interface contracts under `contracts/`
7. Security and responsible-hiring controls under `security/`
8. Operational requirements under `operations/`
9. Delivery sequencing and gates under `delivery/`

## Source-of-truth rules

- User-visible behavior belongs in `product/`.
- Ownership, invariants, and system behavior belong in `architecture/`.
- Wire protocols belong in `contracts/` and generated OpenAPI/Protobuf.
- Security, privacy, and hiring safeguards belong in `security/`.
- SLOs, deployment, recovery, and cost belong in `operations/`.
- Sequencing and release evidence belong in `delivery/`.
- Approved ADRs override proposals in broader documents.

## Document map

```text
docs-new/
├── product/
├── architecture/
│   └── decisions/
├── contracts/
├── security/
├── operations/
└── delivery/
```

The set intentionally distinguishes architecture baselines from final generated contracts, legal determinations, calibrated AI quality thresholds, and measured production SLOs. Those must be produced and approved during implementation.
