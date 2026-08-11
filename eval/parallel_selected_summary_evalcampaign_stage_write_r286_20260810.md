# Selected parallel eval sweep

- date: 2026-08-11T02:00:54Z
- sweep_start_ts: 20260810-190052
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 154s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260810-190054 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 283s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-190054 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
