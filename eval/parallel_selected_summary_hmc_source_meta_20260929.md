# Selected parallel eval sweep

- date: 2026-09-29T11:35:36Z
- sweep_start_ts: 20260929-043535
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_sqlite_wal_records | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_sqlite_wal_records-20260929-043537 |
| 2 | read_combo_command_current_source_explanation | TIMEOUT | exceeded 1200s wall-time | 1201s | 1 | 3 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260929-043537 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
