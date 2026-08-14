# Selected parallel eval sweep

- date: 2026-08-14T07:20:28Z
- sweep_start_ts: 20260814-002026
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | - | 136s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260814-002028 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 248s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260814-002028 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
