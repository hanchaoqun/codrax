# Selected parallel eval sweep

- date: 2026-08-06T09:53:48Z
- sweep_start_ts: 20260806-025346
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | inventory_count_mismatch:entry_page:got6:want4 | 114s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-025348 |
| 2 | github_issue_gson_lazy_number | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 123s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number-20260806-025348 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
