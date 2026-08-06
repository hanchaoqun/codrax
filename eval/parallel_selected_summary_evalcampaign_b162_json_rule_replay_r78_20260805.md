# Selected parallel eval sweep

- date: 2026-08-06T04:30:53Z
- sweep_start_ts: 20260805-213050
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_basic_sum_with_rules | PASS | - | 46s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260805-213053 |
| 2 | hilog_arkts_panic | PASS | - | 92s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_arkts_panic-20260805-213053 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
