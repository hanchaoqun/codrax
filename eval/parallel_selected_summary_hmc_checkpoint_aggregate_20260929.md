# Selected parallel eval sweep

- date: 2026-09-29T12:20:25Z
- sweep_start_ts: 20260929-052024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_sqlite_checkpoint_records | PASS | - | 129s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_sqlite_checkpoint_records-20260929-052025 |
| 2 | read_combo_command_current_source_explanation | PASS | - | 276s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260929-052025 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
