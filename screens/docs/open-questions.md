# Unresolved product questions

Questions the prototype had to answer provisionally but that need a real decision before build. Each
one states what the prototype currently does, why the question matters, and what would have to change.

Ordered roughly by how expensive the answer is to get wrong.

---

## Fairness, transparency and legal

**Q1 — Must screening candidates be told their evaluation exists, and be able to obtain it?**
*Prototype:* candidates see only a submission confirmation, with a data-request route in settings.
*Why it matters:* EU AI Act obligations for AI systems used in employment, GDPR Article 15 access
rights, Illinois AIVI, NYC Local Law 144 and equivalents point in different directions on disclosure.
The strictest reading may require proactive disclosure, which contradicts the brief's "candidates only
see confirmation".
*Blocked on:* legal counsel per operating jurisdiction. **This is the highest-risk open question.**

**Q2 — Does any tenant need a bias audit surface, and who publishes it?**
*Prototype:* evaluation-quality analytics show band distribution by role family with an explicit
caveat that it must not be read as a statement about groups of people. There is no demographic
breakdown anywhere, because collecting that data creates obligations we have not designed for.
*Why it matters:* several jurisdictions require published adverse-impact analysis for automated
employment decision tools. That requires demographic data the product currently never touches.

**Q3 — Is a re-review a right or a courtesy?**
*Prototype:* `admin-appeals.html` exists and appeals are tracked with SLAs, but a recruiter can uphold
a decision with a written rationale.
*Why it matters:* if it is a right, it needs a guaranteed independent reviewer, a deadline, and a
candidate-visible status — none of which is currently modelled candidate-side.

**Q4 — What does the candidate consent to, exactly, and can they withdraw it after the interview?**
*Prototype:* consent to recording and processing is captured at invitation acceptance. Withdrawal is
not modelled.
*Why it matters:* withdrawal after submission raises the question of whether an evaluation already
delivered to a recruiter can be recalled.

---

## Evaluation semantics

**Q5 — What is the confidence number actually measuring?**
*Prototype:* treated as a function of evidence quantity and consistency, shown as high/medium/low plus
a range on each competency bar.
*Why it matters:* recruiters will read it as reliability. If it is a heuristic rather than a
calibrated interval, the range visualisation overstates its rigour and should be replaced with a
qualitative marker.
*Needed:* a defensible definition, and a calibration study to justify drawing a range at all.

**Q6 — Are competency scores comparable across roles, rubrics and rubric versions?**
*Prototype:* the compare screen only compares candidates on the same role and rubric version, and says
so. Progression compares a candidate to themselves.
*Why it matters:* the moment a dashboard averages scores across roles, the number becomes meaningless.
Several tempting aggregate metrics were left out for this reason.

**Q7 — What happens to in-flight and historical evaluations when a rubric is republished?**
*Prototype:* in-flight evaluations keep the version they started with; history retains its own
version; calibration shows a version-impact comparison marked observational.
*Why it matters:* whether a recruiter can re-run an old evaluation against a new rubric — and whether
that is fair to a candidate who has already been declined — is unresolved.

**Q8 — Should the platform ever refuse to produce an evaluation?**
*Prototype:* yes — below the evidence and coverage thresholds it returns "Insufficient evidence".
*Why it matters:* recruiters may experience this as the product failing. Needs a policy on how often
it is acceptable, and whether the tenant is charged for it.

**Q9 — Is "JD compatibility" a score or a checklist?**
*Prototype:* a requirement-by-requirement table with Evidenced / Partially evidenced / Not discussed,
deliberately with no headline percentage.
*Why it matters:* a single percentage is what buyers ask for and is the most misleading number the
product could produce.

---

## Interview experience

**Q10 — What is the correct behaviour when a candidate's connection drops in a screening?**
*Prototype:* 10-minute reconnect window with the clock stopped; beyond it the session finalises and is
flagged as low coverage.
*Why it matters:* this is the most common real-world unfairness. Should the candidate get an automatic
re-invitation? Who pays for the second session?

**Q11 — Can a screening candidate pause, or restart once?**
*Prototype:* no. One attempt, no pause.
*Why it matters:* childcare, shift work and disability all argue for an accommodation path; integrity
argues against. Currently accommodations are requested at invitation time only.

**Q12 — How much thinking time is "extra thinking time", and does it change the evaluation?**
*Prototype:* offered as an accommodation with an explicit statement that it does not affect
evaluation.
*Why it matters:* if response latency ever enters the evaluation, that statement becomes false. It
must be architecturally impossible, not merely policy.

**Q13 — Do personas need to be selectable in screening?**
*Prototype:* the employer chooses; the candidate cannot.
*Why it matters:* a candidate who finds Marcus's sceptical style destabilising is disadvantaged by an
employer's stylistic preference. Arguably screening should use one neutral persona.

**Q14 — What languages and accents are supported, and how is transcription quality communicated?**
*Prototype:* a language preference exists in settings and tenant defaults; quality is reported only as
"audio quality: good".
*Why it matters:* transcription error rates vary by accent, and an evaluation built on a poor
transcript is unfair in a way the current UI cannot surface.

---

## Recruiter workflow

**Q15 — Should a recruiter be able to read the transcript before seeing the suggested band?**
*Prototype:* both are on the same screen, with the suggested band first.
*Why it matters:* anchoring. An evidence-first ordering, or an option to hide the band until the
transcript has been read, would materially change review behaviour — and is testable.

**Q16 — Who owns the decision when reviewers disagree?**
*Prototype:* the most recent decision wins; previous activity is visible.
*Why it matters:* no concept of a required second reviewer, quorum, or escalation exists.

**Q17 — Should comparison be available at all?**
*Prototype:* yes, but constrained — 2–4 candidates, same role, confidence ranges shown, non-overlapping
differences called out and close calls named as too close to call.
*Why it matters:* comparison is the feature most likely to be misused as a leaderboard. It may be
safer to remove it than to caveat it.

**Q18 — What is the retention story for a candidate who is declined?**
*Prototype:* tenant retention policy applies uniformly.
*Why it matters:* many jurisdictions require *minimum* retention of hiring records, which conflicts
with candidate deletion requests.

---

## Platform and commercial

**Q19 — What is the unit of billing — interviews, minutes, evaluations, or seats?**
*Prototype:* seats plus a monthly interview allowance, with cost-per-completed-interview shown
internally.
*Why it matters:* it determines what quotas actually limit, and what happens to a session that fails
evaluation — the interview happened, but the deliverable did not.

**Q20 — At the hard quota limit, what does a candidate see?**
*Prototype:* in-flight interviews always complete; new ones are blocked with a message shown to the
candidate.
*Why it matters:* a candidate being blocked because their prospective employer hit a billing limit is
a bad experience the employer may not want visible.

**Q21 — Does the platform need a candidate-facing status page?**
*Prototype:* the error screen links to a status page for the operational audience only.

**Q22 — Should self-serve practice and enterprise screening share one brand and one domain?**
*Prototype:* yes — one product, one marketing site, mode disclosed everywhere.
*Why it matters:* candidates may distrust practising on the same platform an employer uses to assess
them, even though the data is separated. This is a positioning question the privacy copy currently
carries alone.

---

## Design and IA

**Q23 — Should `review` and `results` stay separate?**
*Prototype:* yes, with distinct responsibilities and shared sub-navigation
(see [inferred-screens.md](inferred-screens.md)). Worth validating with candidates — the split is a
judgement call, and usage data would settle it quickly.

**Q24 — Is the "Insufficient evidence" state understood by recruiters?**
*Prototype:* shown as a hatched bar with the words and an explanation.
*Why it matters:* if recruiters read it as "bad candidate" rather than "short conversation", the
fairness mechanism inverts into a penalty. Needs testing.

**Q25 — How much should the candidate app say about how evaluation works?**
*Prototype:* a moderate amount — enough to explain evidence and confidence, not enough to game.
*Why it matters:* transparency and gameability pull in opposite directions, and the right point on
that line is a policy decision, not a design one.
