# Selected parallel eval sweep

- date: 2026-08-12T02:58:21Z
- sweep_start_ts: 20260811-195820
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 193s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260811-195821 |
| 2 | hilog_mixed_arkts_cangjie | PASS | - | 328s | 1 | 3 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260811-195822 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
