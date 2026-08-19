# Selected parallel eval sweep

- date: 2026-08-19T12:31:10Z
- sweep_start_ts: 20260819-053108
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_nlohmann_long_double_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 236s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260819-053110 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 689s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-053110 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
