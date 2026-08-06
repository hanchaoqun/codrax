# Selected parallel eval sweep

- date: 2026-08-06T08:07:03Z
- sweep_start_ts: 20260806-010702
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dayjs_duration_nan | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 174s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan-20260806-010704 |
| 2 | cangjie_repomap | FAIL | inventory_count_mismatch:public_class:got9:want8 | 450s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-010704 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
