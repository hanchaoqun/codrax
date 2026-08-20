# Selected parallel eval sweep

- date: 2026-08-20T06:22:11Z
- sweep_start_ts: 20260819-232209
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | - | 397s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-232211 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 447s | 1 | 2 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-232211 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
