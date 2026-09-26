# HiSys semantic-preservation fixture

`capture.data` is a self-contained rollback-journal SQLite capture generated from
`capture.sql`. Only that database is attached to the model; this README, SQL and
`expected.json` are independent test oracles, not prompt context.

The natural query asks about 2.000 to 2.080 seconds: six system-event rows, including
a NULL domain reference, an ambiguous event-name reference, valid dictionary ID
zero, names outside legacy print syntax, a newline and content beyond the raw-line
preview. Rows at 1.999 and 2.081 seconds are outside even the existing lookup-only
boundary tolerance; exact half-open boundary semantics are tested separately.
The two outside rows must not enter the selected census. Unknown domain
or name does not remove the row or turn a display fallback into a real identity.
Long content may be clearly summarized, but its retained suffix must remain
available to the finalizer; omissions must never be presented as a complete raw
record. No event implies thread waiting or a performance root cause.

Rebuild: `sqlite3 capture.data < capture.sql` in an empty output path.
