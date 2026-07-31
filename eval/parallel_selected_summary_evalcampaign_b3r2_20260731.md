# Selected parallel eval sweep

- date: 2026-07-31T09:50:51Z
- sweep_start_ts: 20260731-025050
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_log_current_code_boundary | PASS | - | 146s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260731-025051 |
| 2 | logtri_oversized | PASS | - | 147s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_oversized-20260731-025051 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
