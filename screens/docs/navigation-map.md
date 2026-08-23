# Navigation and user-flow map

---

## 1. Navigation structure

All navigation is generated from a single config in `assets/js/shell.js`, so it is identical on every
screen and cannot drift. Groups and items declare the minimum role required, and items the current
role cannot reach are **not rendered at all** — they are not disabled, greyed or hidden with CSS.

### Candidate sidebar

```
Prepare
  ├─ Dashboard                candidate-dashboard.html
  └─ Start an interview       candidate-start-interview.html
Your practice
  ├─ Sessions            [12] candidate-sessions.html
  ├─ Skills                   candidate-skills.html
  ├─ Progression              candidate-progression.html      ← added; see inferred-screens.md
  └─ Goals                    candidate-goals.html
Account
  ├─ Profile & résumé         candidate-profile.html
  └─ Settings                 candidate-settings.html
Footer
  ├─ Help & accessibility     candidate-settings.html#help
  └─ [user menu] Profile · Settings · Theme · Sign out
```

### Candidate mobile bottom navigation (below 768px)

```
Home        Sessions      Practise      Progress      You
dashboard   sessions      start         progression   profile
```

### Admin sidebar — role-gated

```
Hiring                                       (recruiter and above)
  ├─ Overview                admin-dashboard.html
  ├─ Candidates          [7] admin-recruiter.html
  ├─ Compare                 admin-recruiter-compare.html
  ├─ Invitations             admin-invitations.html
  └─ Re-review queue     [2] admin-appeals.html
Tenant configuration                         (tenant_admin and above)
  ├─ Calibration             admin-recruiter-calibration.html
  ├─ Rubrics                 admin-rubrics.html
  ├─ Members & roles         admin-members.html
  ├─ Integrations            admin-integrations.html
  └─ Tenant settings         admin-tenant-settings.html
Platform                                     (platform_admin and above)
  ├─ Analytics               admin-platform-analytics.html
  ├─ Sessions                admin-platform-analytics-sessions.html
  ├─ Evaluation quality      admin-platform-analytics-evaluations.html
  ├─ System health           admin-platform-system.html
  ├─ AI usage & cost         admin-platform-system-llm-usage.html
  ├─ Quotas                  admin-platform-quotas.html
  └─ Grafana ↗               external, styled consistently, marked "opens in a new tab"
Restricted                                   (god only)
  ├─ Super-admin console     admin-god.html
  └─ Audit log               admin-god-audit.html
```

**Role hierarchy:** `recruiter (1) < tenant_admin (2) < platform_admin (3) < god (4)`.
The "Viewing as" control in the admin topbar switches role and reloads, so the permission model can be
verified directly. Pages also guard themselves: opening a platform URL as a recruiter renders a
permission-denied state rather than the content.

### External links

Grafana is the only external destination in the application navigation. It is styled exactly like the
other platform items, carries the `.ext` indicator, `target="_blank" rel="noopener noreferrer"` and a
screen-reader-only "(opens in a new tab)".

---

## 2. Candidate interview lifecycle

This is the core flow the brief specifies, implemented end to end.

```
                          candidate.html  (/candidate — routing state)
                                   │
             ┌─────────────────────┼─────────────────────┐
             │                     │                     │
      first run              returning              pending invitation
             │                     │                     │
             ▼                     ▼                     ▼
  candidate-start-      candidate-dashboard    invitation-accept.html
     interview.html            .html            (consent + disclosure)
             │                     │                     │
             └──────────┬──────────┘                     │
                        ▼                                │
      candidate-start-interview.html                     │
      5-step wizard: role & focus → shape →              │
      persona → length & difficulty → review             │
                        │                                │
                        ▼                                ▼
              candidate-session-prepare.html?id=…&mode=practice|screen
              brief · persona preview · device checks · saved progress
                        │
                        ▼
              candidate-session-live.html?id=…
              phase · persona · connection · mic · waveform · speaking
              indicators · timer · progress · reconnection · captions ·
              push-to-talk · end confirmation
                        │
                        ▼
              candidate-session-complete.html?id=…
              upload → transcribe → extract evidence → evaluate →
              prepare feedback → complete       (+ failed / delayed)
                        │
        ┌───────────────┴────────────────┐
        │ mode = practice                │ mode = screen
        ▼                                ▼
  candidate-session-results.html   "Submitted to Northwind Health"
  (outcome & evidence)             confirmation ONLY — no score,
        ⇅  shared sub-nav          no band, no coaching, no chart
  candidate-session-review.html          │
  (coaching review)                      ▼
        │                          candidate-dashboard.html
        ├─→ redo a question → candidate-start-interview.html?focus=…
        ├─→ add to drills   → candidate-goals.html
        └─→ patterns        → candidate-progression.html
```

**Mode boundary enforcement points**

| Screen | Behaviour in screen mode |
| --- | --- |
| `candidate-start-interview` | Wizard replaced — screening interviews are configured by the employer |
| `candidate-session-prepare` | Shows employer, deadline and "your answers go to the hiring team"; no coaching or redo options |
| `candidate-session-live` | Different persona and phases; end-interview dialog states there is no restart; no feedback anywhere |
| `candidate-session-complete` | Confirmation only; "Preparing feedback" stage becomes "Delivering to the hiring team" |
| `candidate-session-review` / `-results` | Refuse to render; explain that screening evaluations are written for the employer |
| `candidate-skills` / `-goals` / `-progression` | Practice-only; explain and link back to the dashboard |
| `candidate-sessions` | Screen rows show "Not shown — screening" in place of a score |

---

## 3. Recruiter flow

```
admin-dashboard.html
   ├─ review queue ─────────────► admin-recruiter-detail.html?id=…
   ├─ open roles ───────────────► admin-recruiter.html?role=…
   └─ quick actions ────────────► admin-invitation-new.html
                                  admin-recruiter-compare.html

admin-invitation-new.html ──► admin-invitations.html ──► admin-invitation-detail.html?id=…
                                                              │
                                          (candidate side)    ├─► invitation-accept.html
                                                              └─► admin-recruiter-detail.html?id=…

admin-recruiter.html  (roster: filter · search · sort · select)
   ├─ select 2–4 ───────────────► admin-recruiter-compare.html?ids=…
   ├─ row ──────────────────────► admin-recruiter-detail.html?id=…
   └─ row menu ─────────────────► decision dialog (rationale required on disagreement)

admin-recruiter-detail.html   tabs: Summary · Evidence & transcript ·
                                    JD compatibility · Decision · Activity
   ├─ evidence span ────────────► evidence drawer
   ├─ decision recorded ────────► audit entry (+ override reason when it disagrees)
   ├─ request re-review ────────► admin-appeals.html
   └─ audit history ────────────► admin-god-audit.html   (only when role = god)

admin-appeals.html ──► re-run evaluation │ offer new interview │ uphold (rationale required)
```

**Tenant configuration** (`tenant_admin`+): `admin-rubrics.html` → `admin-recruiter-calibration.html`
(anchors, thresholds, weights, sufficiency rules, versioning, publish) ·
`admin-members.html` (roles and the permission matrix) · `admin-tenant-settings.html` (retention,
residency, candidate-facing copy, billing) · `admin-integrations.html` (ATS, webhooks, API keys).

---

## 4. Platform administration flow

```
admin-platform-analytics.html          cross-tenant usage, funnel, cohorts
   ├─► admin-platform-analytics-sessions.html      session health, abandonment, realtime
   ├─► admin-platform-analytics-evaluations.html   confidence, coverage, overrides, failures
   └─► admin-platform-quotas.html?tenant=…

admin-platform-system.html             services · queues · failed jobs · latency ·
   │                                   realtime health · runtime policy versions
   ├─► admin-platform-system-llm-usage.html        model/provider cost, token + audio split
   ├─► admin-platform-quotas.html                  allowances, limits, rate limits, headroom
   └─► Grafana ↗                                   external operational dashboards

admin-god.html  (elevation-gated)      tenant ops · feature flags · policy rollback ·
   └─► admin-god-audit.html            append-only log, hash-chain verification, export
```

---

## 5. Authentication flows

```
index.html ──► register.html ──┬─ candidate ──► verify-email.html ──► candidate.html
               │               └─ organisation ► check-email.html?for=verify
               │
               └─► login.html ──┬─ password ─────────────► candidate.html | admin-dashboard.html
                                ├─ OAuth ► oauth-callback.html ─┬─ success ► ?next= destination
                                │                               └─ failure ► login.html?error=oauth_denied
                                ├─ MFA required ► otp.html ─────► destination
                                └─ forgot ► forgot-password.html ► check-email.html?for=reset
                                                                     └─► reset-password.html
                                                                           └─ expired ► auth-expired.html

Employer invitation ► magic-link.html ► invitation-accept.html ► candidate-session-prepare.html
No workspace access ► access-denied.html          In-app permission refusal ► error-403.html
```

---

## 6. Where every screen is reachable from

No screen is orphaned. Every file is reachable from at least two places: the prototype index
(`screens.html`) and at least one in-product link.

| Screen | Reached from |
| --- | --- |
| `index.html` | Footer/brand links on auth pages, `screens.html` |
| `login.html` / `register.html` | Marketing header and CTAs, each other, sign-out |
| `oauth-callback.html` | `login.html`, `register.html` OAuth buttons |
| `forgot-password.html` | `login.html` |
| `check-email.html` | `forgot-password.html`, `register.html`, `magic-link.html` |
| `reset-password.html` | `check-email.html?for=reset` |
| `verify-email.html` | `register.html`, `check-email.html?for=verify` |
| `magic-link.html` | Invitation email path, `check-email.html?for=magic` |
| `otp.html` | `login.html` when MFA is required |
| `auth-expired.html` | Expired variants of reset / verify / magic-link |
| `access-denied.html` | `login.html` when the account has no workspace |
| `invitation-accept.html` | `magic-link.html`, `admin-invitation-detail.html`, candidate notifications |
| `candidate.html` | Post-login redirect, `screens.html` |
| All `candidate-*` | Candidate sidebar, bottom nav, dashboard cards, session flow |
| `admin-dashboard.html` | Admin sidebar, post-login for recruiter roles |
| All recruiter/tenant `admin-*` | Admin sidebar (role-gated), dashboard quick actions, roster |
| `admin-recruiter-detail.html` | Roster rows, review queue, compare footer, invitation detail, appeals |
| `admin-invitation-*.html` | Invitations list, dashboard quick action |
| All platform `admin-*` | Admin sidebar (platform_admin+), cross-links between analytics and system |
| `admin-god*.html` | Admin sidebar (god only), audit link on recruiter detail |
| `error-403.html` | Permission-denied guards on every gated page |
| `error-404.html` / `error-500.html` | `screens.html`, error-state actions |
| `design-system.html` | `screens.html`, marketing footer |
| `screens.html` | Marketing footer, every error page |
