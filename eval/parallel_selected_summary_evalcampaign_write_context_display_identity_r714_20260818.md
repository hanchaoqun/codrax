# Selected parallel eval sweep

- date: 2026-08-19T03:47:35Z
- sweep_start_ts: 20260818-204734
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | - | 218s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-204735 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 464s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260818-204735 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
