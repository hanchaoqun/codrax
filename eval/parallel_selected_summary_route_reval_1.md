# Selected parallel eval sweep

- date: 2026-07-04T19:33:50Z
- sweep_start_ts: 20260705-033350
- total cases: 3
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 37s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-033350 |
| 3 | data_json_strict_ids | PASS | - | 46s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-033428 |
| 1 | data_basic_sum_with_rules | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:\[repl/data\] data task result | 126s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260705-033350 |

**Pass: 2 / 3 — Fail/Timeout/LaunchFail: 1**
