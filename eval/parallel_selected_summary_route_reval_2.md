# Selected parallel eval sweep

- date: 2026-07-04T19:35:56Z
- sweep_start_ts: 20260705-033556
- total cases: 3
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 26s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-033556 |
| 3 | data_json_strict_ids | FAIL | data_terminal_missing no_log_regex:route=data no_log_regex:\[.*data\] data task result | 71s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-033623 |
| 1 | data_basic_sum_with_rules | FAIL | data_terminal_status:blocked no_log_regex:\[repl/data\] data task result | 213s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260705-033556 |

**Pass: 1 / 3 — Fail/Timeout/LaunchFail: 2**
