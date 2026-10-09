# Generic measurement acceptance fixture

Synthetic, closed SQLite input; no external capture data or credentials. `capture.sql` is the complete reproducible source. To generate a new database, run `sqlite3 /path/to/new/capture.data < capture.sql` (do not append into an existing database).

The public question asks for raw observations and whether GPU interpretation is justified. It does not teach implementation constraints to the user. The native tool, typed result and final answer must carry the following boundaries themselves.

## Oracle for [1, 2) seconds

15 source measurement rows; 13 selected records; one record exactly at 2 seconds excluded; one NULL timestamp unpositioned and not assigned to the window. Missing/ambiguous filter definitions do not delete the underlying measurement or multiply it into several rows.

| Physical row | Filter | Source value / storage | Original interval in seconds | Window selection |
|---|---:|---|---|---|
| 1 | 10 | 334200000.5 / REAL | [0.9, 1.2) | [1, 1.2) |
| 2 | 10 | 480000000 / INTEGER | [1.2, 1.4) | unchanged |
| 3 | 10 | NULL | [1.45, 1.55) | unchanged; value unknown |
| 4 | 10 | 600000000 / INTEGER | starts 1.6; duration NULL | point observation; no inferred end |
| 6 | 20 | 800000000 / INTEGER | [1, 1.5) | unchanged; independent series |
| 7 | 11 | 1 / INTEGER | [1, 1.25) | unchanged; state semantics unverified |
| 8 | 11 | 0 / INTEGER | [1.25, 1.5) | unchanged; real zero retained |
| 9 | 11 | "1" / TEXT | [1.5, 1.6) | unchanged; not numeric state 1 |
| 10 | 12 | 42.5 / REAL | [1.1, 1.2) | unchanged; unit unverified |
| 11 | 30 | 9007199254740993 / INTEGER | point at 1.75 | exact integer; zero duration |
| 12 | 999 | hex 0031 / BLOB | [1.8, 1.85) | raw record; no matching filter |
| 13 | 40 | 7 / INTEGER | [1.85, 1.9) | raw record; duplicate filter identity |
| 15 | 10 | 500000000 / INTEGER | starts 1.95; duration -1 ns | point observation; duration invalid |

Filters 10 and 20 both have the exact name `gpufreq`, but remain separate sequences. `source_arg_set_id` is an unresolved argument-set reference, not a GPU instance ID; matching reference 7 does not authorize pairing `gpufreq` with `gpu_state`. The input provides no verified unit, state encoding or resource pairing. A final answer can identify suggestive names, not assert Hz/MHz, utilization percent, active/inactive semantics or actual active-frequency statistics. There is no scheduler, wakeup or frame dependency evidence, so these observations cannot establish an application root cause.

More adversarial inputs (negative timestamps, overflow, missing columns, malformed/duplicate carriers, cancellation, retention exhaustion and source mutation) belong in deterministic public-path tests; they are not packed into the natural-language question.
