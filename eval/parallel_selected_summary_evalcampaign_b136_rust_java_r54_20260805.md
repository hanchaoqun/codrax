# Selected parallel eval sweep

- date: 2026-08-05T22:01:33Z
- sweep_start_ts: 20260805-150132
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_trait_impls | PASS | - | 83s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_trait_impls-20260805-150134 |
| 2 | sr_java_handler_impls | FAIL | missing_inventory_row:handler_route:EchoHandler_/echo_EchoHandler.java missing_inventory_row:handler_route:UpperHandler_ | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_handler_impls-20260805-150134 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
