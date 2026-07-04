# Selected parallel eval sweep

- date: 2026-07-04T19:55:04Z
- sweep_start_ts: 20260705-035504
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 41s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-035504 |
| 1 | data_json_strict_ids | FAIL | no_regex_match:"ids" | 244s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-035504 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
