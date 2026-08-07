# Selected parallel eval sweep

- date: 2026-08-07T22:10:26Z
- sweep_start_ts: 20260807-151024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 45s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260807-151026 |
| 1 | github_issue_nlohmann_long_double | PASS | - | 142s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double-20260807-151026 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
