# Selected parallel eval sweep

- date: 2026-08-19T19:25:41Z
- sweep_start_ts: 20260819-122539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260819-122541 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 610s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-122541 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
