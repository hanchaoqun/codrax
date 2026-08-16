# Selected parallel eval sweep

- date: 2026-08-16T21:35:02Z
- sweep_start_ts: 20260816-143501
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260816-143502 |
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 172s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260816-143502 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
