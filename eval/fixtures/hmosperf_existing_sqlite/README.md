# Closed SQLite intake fixture

`capture.data` is a real SQLite database with a non-database suffix. It is
synthetic, self-contained, rollback-journal mode, and has no sidecar files.
`capture.sql` is its complete reproducible source. Regenerate only into a new
file: `sqlite3 new-capture.data < capture.sql`.

Expected observations (not root-cause claims):

| Marker's native interval (ns) | Frames | App | Duration |
|---|---:|---:|---:|
| 1020000000–1040000000 | 2 | 27599 | 20 ms |
| 1100000000–1160000000 | 4 | 27599 | 60 ms |
| 2000000000–2020000000 | 1 | 27599 | 20 ms |

The [1, 1.2) second marker-emission window contains the first two rows. The
native marker intervals use the same trace clock: no timezone or clock-domain
offset. `jank_frames >= 2` selects two rows, not the third one. `OpenDocument`
and `UploadTexture` are temporal/business context only; overlap is not proof of
a cause. `zero-time-marker` at zero and `tail-business-marker` at 2.990 seconds
exercise zero preservation and full-file queries beyond a bounded preview.
The vendor table exercises text fidelity only. Neither it nor thread-state
rows authorize invented scheduler transitions or wakeup edges.

The live case attaches only the database and uses `stub_repo`; this oracle and
SQL source are not included in the analysis repository.
