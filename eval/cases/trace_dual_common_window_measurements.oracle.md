# Independent oracle: common-window measurement records

This file is not attached to the model. The natural question supplies two original paths and one common interval, without prescribing JSON, tools, query order, missing-value rules or recovery behavior. This is a new shared-window admission case, not a rerun of the separate-window or failed-side questions.

## Sources and requested scope

- Baseline: `eval/fixtures/hmosperf_dual_measurements/baseline.data`, whose SQL source and row oracle are `eval/fixtures/hmosperf_measurements/capture.sql` and `README.md`.
- Current: `eval/fixtures/hmosperf_common_window_measurements/current.data`, generated from the adjacent `current.sql` into a new database. Both declared capture ranges are [0, 3) seconds.
- The same requested [1, 2) second window applies independently to both original captures. Neither file supplies the other's rows, source generation, filter definitions or successful-read status.
- A shared numeric selection is not clock synchronization, a shared device, matched workload or permission to pool records. No numeric difference or performance verdict is requested.
- Both inputs are healthy. Actual successful reads must expose both sides as available; an unchanged model-declared status or tool-call attempt is not sufficient. Keep each original path distinct through preparation and the native result.

## Exact baseline population

15 raw records; 13 window-selected records; 7 independently retained groups: five uniquely resolved filters plus two unresolved original records. Physical row 5 starts at the excluded right boundary; physical row 14 has NULL time and is not part of the selected window. The complete 13-row oracle is the existing measurement README linked above.

In particular, preserve 334200000.5, 9007199254740993, NULL, numeric zero, TEXT "1", BLOB `0031`, missing filter 999, and ambiguous filter 40. Row 1 retains original [0.9, 1.2) and selected [1, 1.2). Rows with NULL or negative duration do not acquire an invented endpoint.

## Exact current population

10 raw records; 8 window-selected records; 4 uniquely resolved series. Physical row 8 starts at the excluded right boundary. Row 9 has value 55 and a 50,000,000 ns duration but NULL start; it is unpositioned, not a ninth selected row. A disclosed unpositioned count or separately labeled supplementary raw record is permissible; it must not gain a timestamp from the common query window.

| Physical row | Filter / source name | Original value / storage | Original interval (seconds) | Selected interval |
|---|---|---|---|---|
| 1 | 10 / gpufreq | 0 / INTEGER | [0.9, 1.1) | [1, 1.1) |
| 2 | 10 / gpufreq | NULL | [1.1, 1.2) | unchanged; value unknown |
| 3 | 10 / gpufreq | 250000000.25 / REAL | starts 1.2; duration NULL | end unknown |
| 4 | 11 / gpu_state | "0" / TEXT | [1.25, 1.3) | unchanged |
| 5 | 11 / gpu_state | 1 / INTEGER | [1.05, 1.15) | unchanged |
| 6 | 20 / gpufreq | 750000000 / INTEGER | [1.1, 1.3) | unchanged; distinct same-name series |
| 7 | 30 / vendor.sample | 9007199254740995 / INTEGER | point at 1.4; duration 0 | original point |
| 10 | 10 / gpufreq | 500000000 / INTEGER | starts 1.95; duration -1 ns | end unknown |

Original row order is intentionally not chronological. Source-local reordering is fine, but each row must appear once in the chosen complete raw view with its original storage type, scalar precision and duration. Equivalent exact scientific notation is acceptable. Filters 10 and 20 must not merge by the shared name `gpufreq`, within or across captures. Filter names and `source_arg_set_id` do not establish units, hardware ownership, state encoding or frequency/utilization semantics.

## Independent checks and acceptance

The SQL files are the independent source of truth, not tool output or model prose. A read-only SQLite overlap query must yield baseline row IDs `1,2,3,4,6,7,8,9,10,11,12,13,15` and current IDs `1,2,3,4,5,6,7,10`; each source has exactly one NULL timestamp. The tables have no scheduler or dependency evidence.

Audit the accepted analyzer scope, actual preparation and query parameters, original-path/status receipts, finalizer native table catalog, final accepted structured document, and rendered answer. Successful native tables must retain all 13 + 8 selected records. Narrative explanation and optional diagrams must not contradict the original values or invent physical meanings. An accurate system appendix cannot excuse a false principal explanation.

Common-window parser, explicit-side override rejection, forged quote, partial endpoint, multi-window ambiguity, same physical source, source mutation and stale recovery remain deterministic boundary tests; the user's question must not teach these rules. A machine PASS only proves the runner's minimum successful-query check. Manual audit decides this case; prior manually failed cases remain open independently.
