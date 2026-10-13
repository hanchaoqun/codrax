# Multi-application gzip SQLite intake oracle

This new fixture does not replace the original `hmosperf_gzip_sqlite` input.
Only `capture.transport` is attached to the model; the SQL, uncompressed
database and this oracle are not attached. The source repository is `stub_repo`.
The transport is one deterministic gzip member (`gzip -n`) of `capture.data`;
its suffix, gzip name and mtime do not identify a source or clock.

The natural question selects report emissions in 1.0–1.5 seconds with at least
3 reported frames. It does not select an application or declare a result count.
The independent, complete oracle contains 4 matching rows across 2 applications:

| Report emission (s) | appid | Frames | Native start (ns) | Native end (ns) | Duration (ms) |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1.041 | 27599 | 3 | 1020000000 | 1040000000 | 20 |
| 1.161 | 27599 | 5 | 1100000000 | 1160000000 | 60 |
| 1.241 | 41002 | 4 | 1180000000 | 1240000000 | 60 |
| 1.421 | 41002 | 7 | 1320000000 | 1420000000 | 100 |

Negative controls are a 2-frame record for appid 27599 emitted at 1.301 seconds
and a 6-frame record for appid 41002 emitted at 2.081 seconds. Thus threshold
3, result count 4, application count 2, and total marker count 6 are distinct.
This catches preview observations becoming user declarations or hidden filters.

Human acceptance inspects actual preparation, analyzer and query arguments,
native payload, finalizer context, final prose/table and every other visible
surface. The answer must enumerate the 4 source rows with exact native fields
and independently calculated durations. A narrow application query alone does
not establish all-application coverage; a complete partitioned search is valid
if the partition population is itself established. A preview count is not a
user-declared count. Keep these checks out of the user-facing question.

The marker's emission timestamp is distinct from its declared jank interval.
Native start/end fields share the trace clock and need no timezone adjustment.
Nearby business spans and the synthetic running inventory do not establish a
cause, scheduler transition or wakeup dependency. Machine answer regex checks
are only smoke tests and cannot replace membership or full-process audit.

To regenerate, create a new SQLite database from `capture.sql`, run
`gzip -n -k capture.data`, and rename the resulting `capture.data.gz` to
`capture.transport`. Verify decompression equals the original database and
record SHA-256 for the case, SQL, database and compressed input before the run.
