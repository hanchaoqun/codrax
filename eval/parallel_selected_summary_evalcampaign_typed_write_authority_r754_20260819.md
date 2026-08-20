# Selected parallel eval sweep

- date: 2026-08-20T03:23:30Z
- sweep_start_ts: 20260819-202329
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 207s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260819-202330 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | - | 895s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-202330 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
