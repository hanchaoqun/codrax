# Selected parallel eval sweep

- date: 2026-08-06T10:26:17Z
- sweep_start_ts: 20260806-032615
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 127s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260806-032617 |
| 2 | github_issue_gson_lazy_number | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 157s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number-20260806-032617 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
