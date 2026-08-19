# Selected parallel eval sweep

- date: 2026-08-19T17:24:09Z
- sweep_start_ts: 20260819-102407
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_report_failed write_final_verdict:unverified:parser_error | 602s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-102409 |
| 1 | qf_logic_view_read_pipeline | TIMEOUT | exceeded 1200s wall-time | 1200s | 1 | 3 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-102409 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
