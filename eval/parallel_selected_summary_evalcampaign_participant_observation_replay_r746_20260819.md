# Selected parallel eval sweep

- date: 2026-08-19T22:58:59Z
- sweep_start_ts: 20260819-155858
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:missing_terminal_verify_verdict | 520s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-155859 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 536s | 1 | 2 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-155859 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
