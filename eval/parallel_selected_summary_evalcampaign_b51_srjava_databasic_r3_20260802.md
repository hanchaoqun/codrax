# Selected parallel eval sweep

- date: 2026-08-03T03:25:28Z
- sweep_start_ts: 20260802-202526
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_call_chain | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260802-202528 |
| 2 | data_basic_sum_with_rules | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:(^|[^0-9])17([^0-9]|$) | 350s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260802-202528 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
