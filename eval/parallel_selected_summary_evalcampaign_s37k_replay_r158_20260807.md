# Selected parallel eval sweep

- date: 2026-08-07T10:09:13Z
- sweep_start_ts: 20260807-030911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 196s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260807-030913 |
| 1 | read_combo_answer_document_tools | PASS | - | 248s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260807-030913 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
