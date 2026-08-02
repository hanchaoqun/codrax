# Selected parallel eval sweep

- date: 2026-08-02T15:58:05Z
- sweep_start_ts: 20260802-085803
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 179s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-085805 |
| 2 | read_combo_log_current_source_explanation | PASS | - | 228s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260802-085805 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
