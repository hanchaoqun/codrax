# Selected parallel eval sweep

- date: 2026-08-17T01:14:57Z
- sweep_start_ts: 20260816-181456
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | missing_principal:5.604 missing_principal:70.338 no_principal_regex_match:([Rr]unnable.{0,120}5\.604|5\.604.{0,120}[Rr]u | 187s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-181458 |
| 1 | mr_poly_binding_chain | PASS | - | 458s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-181457 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
