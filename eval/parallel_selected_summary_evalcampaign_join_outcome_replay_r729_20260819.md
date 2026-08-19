# Selected parallel eval sweep

- date: 2026-08-19T11:24:41Z
- sweep_start_ts: 20260819-042439
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | - | 174s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260819-042441 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 429s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-042441 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
