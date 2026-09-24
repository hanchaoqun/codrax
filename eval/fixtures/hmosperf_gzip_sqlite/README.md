# Gzip SQLite intake fixture

`capture.transport` is a single gzip member containing the exact bytes of
`../hmosperf_existing_sqlite/capture.data`. Its suffix deliberately does not
identify a database or compressed file. Gzip Name and MTIME are unset; neither
is a trace identity, path or clock authority. Generate with gzip's deterministic
`-n` option while keeping the source database unchanged.

Only the compressed file is attached to the live model, with `stub_repo` as
the source repository. The independent oracle remains the source fixture's
`capture.sql` and README; none of these oracle files is attached.

In the requested 1.0–1.2 second record window, the complete matching population
is two rows for appid 27599:

| Marker emission (s) | Native start (ns) | Native end (ns) | Frames | Duration (ms) |
| --- | ---: | ---: | ---: | ---: |
| 1.041 | 1020000000 | 1040000000 | 2 | 20 |
| 1.161 | 1100000000 | 1160000000 | 4 | 60 |

The third row is emitted at 2.021 seconds and reports one frame; it is outside
both requested selection conditions. Marker emission is not the declared jank
interval. Native intervals use the trace clock, with no timezone offset or
refresh-rate reconstruction. Nearby business activity is context, not causal
proof. Missing scheduler edges must not be manufactured from state inventory.

Machine checks are a smoke test only. Human acceptance independently checks
complete membership, threshold, timestamps, units, actual source preparation,
raw/query/finalizer handoff, source-read obligations and causal restraint.
