# Selected parallel eval sweep

- date: 2026-08-20T01:16:34Z
- sweep_start_ts: 20260819-181632
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 432s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-181634 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 1535s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-181634 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
