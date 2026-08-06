# Selected parallel eval sweep

- date: 2026-08-06T04:18:37Z
- sweep_start_ts: 20260805-211834
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | hilog_arkts_panic | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_arkts_panic-20260805-211837 |
| 1 | data_basic_sum_with_rules | PASS | - | 127s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260805-211837 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
