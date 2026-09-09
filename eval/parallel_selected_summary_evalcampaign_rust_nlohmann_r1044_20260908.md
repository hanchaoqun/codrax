# Selected parallel eval sweep

- date: 2026-09-09T03:36:10Z
- sweep_start_ts: 20260908-203610
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | - | 119s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260908-203610 |
| 1 | sr_rust_cross_module_chain | PASS | - | 146s | 1 | 2 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260908-203610 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
