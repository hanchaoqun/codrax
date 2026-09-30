# Selected parallel eval sweep

- date: 2026-09-30T01:31:24Z
- sweep_start_ts: 20260929-183122
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_command_current_source_explanation | FAIL | answer_surface_receipt_missing dynamic_scalar_binding_missing:tool_non_test_go_recursive:390 | 96s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260929-183124 |
| 1 | trace_sqlite_closed_wal_records | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_sqlite_closed_wal_records-20260929-183124 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
