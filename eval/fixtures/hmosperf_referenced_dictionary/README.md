# Referenced dictionary consumers — closed SQLite

`capture.data` is the sole product input: a real synthetic SQLite database,
with a non-database filename suffix, rollback-journal mode and no sidecars.
`capture.sql` is its reproducible source. Generate only into a new file with
`sqlite3 new-capture.data < capture.sql`.

The case uses `eval/fixtures/stub_repo`. Neither this file nor `expected.json`
nor the SQL source is attached to the model. No oracle text is stored in the
database. The question asks naturally for startup stages and system events;
it does not advertise the failure modes or dictate an answer.

The independent oracle covers four startup stages and three resolved system
events in 1.000–1.080 seconds. Two additional system-event records have unresolved
identity and remain available through SQL fidelity. Names use valid zero and
ordinary INTEGER references, NULL, and an ambiguous duplicate key. Other
dictionary entries include unused values and local malformed/duplicate rows.
These are compatibility witnesses, not a large-dictionary memory benchmark.

Manual acceptance compares the final answer with `expected.json` and checks
both identity uncertainty and retained timing. The shell case's positive
checks are smoke checks only, not a replacement for that audit. A model may
use clear business wording for missing names; it must not invent a choice
between `FirstDraw` and `CacheWarmup`.

This fixture validates a SQLite input, not raw binary decoder execution.
Raw binary conversion and resolver resource bounds are separate deterministic
public-test obligations.
