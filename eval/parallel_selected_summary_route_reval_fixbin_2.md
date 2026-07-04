# Selected parallel eval sweep

- date: 2026-07-04T19:59:08Z
- sweep_start_ts: 20260705-035908
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 39s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-035908 |
| 1 | data_json_strict_ids | PASS | - | 283s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-035908 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
