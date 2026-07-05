# Selected parallel eval sweep

- date: 2026-07-05T01:45:23Z
- sweep_start_ts: 20260705-094523
- total cases: 3
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 31s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-094523 |
| 3 | data_text_filter_count | PASS | - | 98s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-094523 |
| 1 | data_basic_sum_with_rules | PASS | - | 247s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260705-094523 |

**Pass: 3 / 3 — Fail/Timeout/LaunchFail: 0**
