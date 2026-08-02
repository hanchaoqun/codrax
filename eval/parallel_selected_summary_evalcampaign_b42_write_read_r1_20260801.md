# Selected parallel eval sweep

- date: 2026-08-02T12:57:14Z
- sweep_start_ts: 20260802-055713
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_log_current_source_explanation | PASS | - | 197s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260802-055714 |
| 1 | github_issue_napi_force_wasi_env_symptom | PASS | - | 300s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-055714 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
