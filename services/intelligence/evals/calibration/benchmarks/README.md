# Benchmark sets

Empty, and that is the finding rather than an oversight. There are no human benchmark ratings in this
repository, so no threshold in this product is calibrated. See [`../README.md`](../README.md) for what
would have to be collected and [`../plan.json`](../plan.json) for the floors a set would have to clear.

A file dropped in here is not a calibration. It has to declare `"rater_provenance": "human"` with a
collection record naming its raters and their lawful basis, and the plan's owner has to name its
`set_id` in `approved_benchmark_sets`. Until both have happened, `calibrate()` refuses and says which
of the two is missing.

`test_no_rating_set_in_the_repository_claims_a_human_rater` fails the moment a file here claims human
provenance. That is deliberate: whoever collects real ratings updates that test, this directory, the
plan and the QUA-03 ticket together, so the claim cannot be made by accident.
