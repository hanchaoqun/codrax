# Selected parallel eval sweep

- date: 2026-08-13T19:19:56Z
- sweep_start_ts: 20260813-121955
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | missing:因果投影 no_principal_text_regex_match:((CPU ?4|cpu=4).{0,240}(2\.10 ?GHz|2\.1 ?GHz|2100 ?MHz|2100000 ?kHz). | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-121956 |
| 2 | mr_poly_binding_chain | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260813-121956 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
