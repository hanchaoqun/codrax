# Selected parallel eval sweep

- date: 2026-07-04T21:10:10Z
- sweep_start_ts: 20260705-051010
- total cases: 3
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 3 | data_text_filter_count | PASS | - | 45s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-051010 |
| 2 | data_json_strict_ids | PASS | - | 46s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-051010 |
| 1 | data_basic_sum_with_rules | FAIL | read_exit:1 data_terminal_status:failed | 62s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260705-051010 |

**Pass: 2 / 3 — Fail/Timeout/LaunchFail: 1**
