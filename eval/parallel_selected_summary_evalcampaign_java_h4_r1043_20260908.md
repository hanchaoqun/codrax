# Selected parallel eval sweep

- date: 2026-09-09T02:11:57Z
- sweep_start_ts: 20260908-191157
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_handler_impls | FAIL | missing_inventory_row:handler_route:EchoHandler_/echo_EchoHandler.java missing_inventory_row:handler_route:UpperHandler_ | 73s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_java_handler_impls-20260908-191157 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:(([Rr]unning|实际([[:space:]]*CPU[[:space:]]*)?运行|运行占|运行时间).{0,120}157\.248 | 122s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260908-191157 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
