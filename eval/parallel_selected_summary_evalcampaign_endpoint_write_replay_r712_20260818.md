# Selected parallel eval sweep

- date: 2026-08-19T02:21:13Z
- sweep_start_ts: 20260818-192111
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | plan_not_written apply_not_run | 903s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-192113 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 1000s | 1 | 2 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260818-192113 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
