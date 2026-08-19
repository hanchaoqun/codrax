# Selected parallel eval sweep

- date: 2026-08-19T06:20:03Z
- sweep_start_ts: 20260818-232001
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 271s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260818-232003 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 295s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260818-232003 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
