# Selected parallel eval sweep

- date: 2026-08-05T22:54:52Z
- sweep_start_ts: 20260805-155451
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 43s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260805-155452 |
| 2 | sr_cpp_sink_impls | FAIL | missing_inventory_row:sink_implementer:ConsoleSink_console_sink.hpp missing_inventory_row:sink_implementer:FileSink_file | 118s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_cpp_sink_impls-20260805-155452 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
