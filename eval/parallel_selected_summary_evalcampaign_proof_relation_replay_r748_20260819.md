# Selected parallel eval sweep

- date: 2026-08-19T23:52:35Z
- sweep_start_ts: 20260819-165234
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 642s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-165235 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 963s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-165235 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
