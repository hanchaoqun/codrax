# Selected parallel eval sweep

- date: 2026-09-07T12:40:21Z
- sweep_start_ts: 20260907-054019
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_cross_module_chain | PASS | - | 115s | 1 | 1 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260907-054021 |
| 2 | github_issue_nlohmann_long_double_symptom | PASS | - | 143s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260907-054021 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
