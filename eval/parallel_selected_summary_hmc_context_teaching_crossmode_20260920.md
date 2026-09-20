# Selected parallel eval sweep

- date: 2026-09-20T10:35:11Z
- sweep_start_ts: 20260920-033511
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_context_teaching_crossmode_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_trace_current_source_explanation | PASS | - | 383s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_context_teaching_crossmode_20260920/read_combo_trace_current_source_explanation-20260920-033511 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | write_final_run_status:blocked write_final_verdict:missing:missing | 576s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_context_teaching_crossmode_20260920/github_issue_dateutil_relativedelta_float_symptom-20260920-033511 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
