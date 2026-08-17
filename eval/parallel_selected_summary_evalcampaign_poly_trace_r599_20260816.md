# Selected parallel eval sweep

- date: 2026-08-17T00:41:38Z
- sweep_start_ts: 20260816-174137
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | - | 203s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-174138 |
| 1 | mr_poly_binding_chain | PASS | - | 239s | 1 | 1 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-174138 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
