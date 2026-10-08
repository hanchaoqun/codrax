# Static initialize source-admission oracle

Synthetic, redistributable SQLite protocol fixture, not device acceptance.
Source schema follows the official `static_initalize` uint32-IPID/int64 SQLite
projection and public TID. The optional source `depth` is process-relative
presentation metadata; it is not thread call-stack authority.

For thread 101 in [10.000, 10.050) seconds:

| Dynamic library | Interval (s) | Elapsed time | CPU evidence |
|---|---|---|---|
| libimage.so | [10.002, 10.008) | 6 ms | Both endpoints have CPU 0 Running witnesses |
| librender.so | [10.030, 10.045) | 15 ms | Unknown; no scheduler witness at either endpoint |

The two non-overlapping source intervals total 21 ms of observed initialization
wall time, not CPU execution time. Background thread 202 and liblater.so are
outside the selected owner/window. Lack of a scheduler witness does not make
the library interval disappear and must not become CPU 0, zero duration, or a
causal explanation. No claim of a complete startup call tree or hardware/device
acceptance is justified. User question deliberately carries no schema, tool, or
boundary-system instructions.

Rebuild only a new output file: `sqlite3 capture.data < capture.sql` from the
fixture directory. The source SQL is kept for deterministic audit.
