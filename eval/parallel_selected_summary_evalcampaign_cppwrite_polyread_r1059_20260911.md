# Selected parallel eval sweep

- date: 2026-09-11T06:54:21Z
- sweep_start_ts: 20260910-235420
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 481s | 1 | 2 | 0 | 1 | 0 | 5 | 6 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260910-235421 |
| 1 | github_issue_nlohmann_long_double_symptom | TIMEOUT | exceeded 1200s wall-time | 1200s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260910-235421 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
