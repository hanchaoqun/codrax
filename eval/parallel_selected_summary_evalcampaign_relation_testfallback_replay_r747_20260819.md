# Selected parallel eval sweep

- date: 2026-08-19T23:37:03Z
- sweep_start_ts: 20260819-163702
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:missing_terminal_verify_verdict | 248s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-163703 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 391s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-163703 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
