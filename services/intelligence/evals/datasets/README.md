# Evaluation datasets: provenance, consent and licensing

**Status:** Active · **Owner:** AI and data quality workstream · **Review by:** 2027-02-28

The machine-readable record is [`manifest.json`](manifest.json), which carries every field
[evaluation-system.md](../../../../docs/architecture/evaluation-system.md) requires of a dataset
manifest: source and synthetic status, consent and legal basis, de-identification, splits, expected
behaviour, known limitations, access, retention and owners. This file is the same record in prose,
because a reviewer reads sentences and a test reads JSON, and both need to be true.

## These fixtures are synthetic, and that is the right choice here

Every transcript in this directory was written for this repository. No sentence is transcribed from a
real interview. No person, employer, ward, school, account or portfolio named or implied in these files
exists. Nothing was paraphrased from a real session, so there is no source to re-identify.

Synthetic is not the convenient choice, it is the correct one for this particular job. The dataset's
purpose is to cover deliberate edges: a candidate who contradicts themselves, a candidate who says
three words, a candidate whose transcript came back unreliable, and a candidate who speaks an
instruction at the pipeline to see what it does. Collecting those from real interviews would mean
recording people at their least flattering moments and keeping the recording indefinitely so a
regression test can replay it. That is a bad trade. It is also a fragile one: a report artifact
committed to git alongside the fixtures would carry excerpts of those transcripts into every clone of
the repository, forever. Synthetic fixtures cannot leak somebody's worst interview because there is
nobody behind them.

What synthetic fixtures cannot do is establish that the product works. The specification says so and
this document repeats it: **synthetic examples are necessary for edge coverage but cannot be the only
evidence for production validity.** A green report means a known behaviour has not regressed. It does
not mean the extractor reads real speech well, and nobody should quote it as if it did.

## Consent

There is no data subject, so there was nobody to ask. The manifest records consent as
`not_applicable` rather than `obtained`, because `obtained` would describe a consent process that never
happened.

**No real candidate transcript may be added to this directory.** A dataset drawn from real sessions
needs an approved lawful basis, its own retention schedule, its own access control and its own
deletion path, and it does not belong in a git repository under any of them. If such a dataset is ever
built it gets its own manifest, its own storage and its own review; it does not inherit this one.

## Licensing

Authored for this repository and carried under the repository's own licence. No third party content is
included in any form: nothing here is lifted from a corpus, a job board, a competency framework, a
textbook or an interview guide. Recording that explicitly is what makes these fixtures safe to quote
inside a release report that leaves the engineering team.

## What is in the set

Six professions, twenty six cases, five stability probes. Each profession carries the four case classes
below, and engineering and finance additionally carry a known contradiction false positive.

| Class | What it is for |
|---|---|
| `well_evidenced` | The happy path the edges are measured against. |
| `insufficient_evidence` | Evidence that exists and is below the rubric's floor, alongside an admitted gap and a competency never reached. Three different reasons for saying nothing, and they are not interchangeable. |
| `contradiction` | A genuine numeric conflict, which is a prompt for clarification and never an inference about the person. |
| `contradiction_false_positive` | A pair the extractor makes that is not a conflict at all. Declared as a false positive so the harness counts it against the extractor rather than scoring it as a success. |
| `unassessable` | All four causes appear across the set: no candidate speech, no word timing, a transcript below the confidence floor, and too little speech to measure. Each has a different remedy, which is why they are four states and not one. |

Each case declares what it expects: sufficiency per competency, the evidence spans that must appear,
the competencies that must stay silent, the contradictions that must and must not be raised, and the
delivery assessability. The harness checks all of it, so a fixture whose behaviour drifts fails a test
rather than quietly becoming the new normal.

## How the transcripts are stored

A turn carries its text, a speaking rate, a transcript confidence and the positions of any long pauses.
Word timings are materialised from those by the loader rather than stored. Three thousand generated
word objects would make these files unreadable, and a reviewer cannot check what they cannot read; the
rate and the pause positions are the part a reviewer actually needs to see. The layout is deterministic,
so the same case yields the same milliseconds on every run.

## Known limitations, stated rather than discovered later

- Word timings are evenly spaced inside a turn, so pace and pausing are exercised as arithmetic rather
  than as real speech rhythm.
- Audio is not represented at all. Clipping, noise, volume and device differences have no fixtures here,
  so the fairness monitoring QUA-05 needs cannot be built on this dataset alone.
- No accent, dialect or second language speech is represented. Writing such fixtures by imitation would
  encode a stereotype rather than test one, so the set abstains and the gap is recorded instead of being
  quietly filled. Doing this properly needs real speakers, consent and a lawful basis.
- Every case is British English. Supported languages are an open decision in the specification, and
  inventing coverage before that decision is made would fake an answer to it.
- No partially assessable session, where one answer is measurable and another is not. Each case is
  deliberately assessable or deliberately not, so a failure names one cause.
- The transcripts are short by interview standards. Latency and cost figures from this dataset describe
  the harness, not a production session.
- Accommodations, document prompt injection and mode leakage are named by the specification's dataset
  list and are not covered. They belong with the tickets that build those surfaces.
- Only `rubric/practice-default` at 1.1.0 is exercised. Sufficiency thresholds from other rubrics are
  untested.

## Changing a fixture

`manifest.json` records a SHA-256 for every file. Editing a fixture without regenerating the manifest
fails `test_every_dataset_file_is_listed_and_its_digest_matches`, which exists so that provenance and
data cannot drift apart quietly. Regenerate both the manifest and the report with:

```
cd services/intelligence && uv run python -m prepeet_ai.evals
```

The prose in the manifest is never written by that tool. Provenance, consent, licensing and the known
limitations are claims somebody has to mean.
