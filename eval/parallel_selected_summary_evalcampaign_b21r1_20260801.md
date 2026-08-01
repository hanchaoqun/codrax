# Selected parallel eval sweep

- date: 2026-08-01T12:34:46Z
- sweep_start_ts: 20260801-053444
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_log_current_source_bucketed_units | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_bucketed_units-20260801-053446 |
| 2 | read_combo_git_current_source_explanation | PASS | - | 219s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_git_current_source_explanation-20260801-053446 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
