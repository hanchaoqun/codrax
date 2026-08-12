# Selected parallel eval sweep

- date: 2026-08-12T03:38:36Z
- sweep_start_ts: 20260811-203834
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_annotation_route | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_annotation_route-20260811-203836 |
| 2 | github_issue_nlohmann_long_double_symptom | PASS | - | 121s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260811-203836 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
