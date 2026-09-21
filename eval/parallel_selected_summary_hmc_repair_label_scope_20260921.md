# Selected parallel eval sweep

- date: 2026-09-21T11:10:58Z
- sweep_start_ts: 20260921-041057
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_repair_label_scope_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | - | 170s | 1 | 0 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_repair_label_scope_20260921/real_trace_h4_supply_thermal_witness-20260921-041058 |
| 1 | trace_query_business_marker_io_chain | FAIL | no_primary_regex_match:(^|[^0-9])35([.]0+)?[[:space:]*]*(ms|毫秒) | 240s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_repair_label_scope_20260921/trace_query_business_marker_io_chain-20260921-041058 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
