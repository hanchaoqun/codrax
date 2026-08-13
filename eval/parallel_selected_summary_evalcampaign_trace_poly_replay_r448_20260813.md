# Selected parallel eval sweep

- date: 2026-08-13T18:26:20Z
- sweep_start_ts: 20260813-112618
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 139s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260813-112620 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | missing:Trace missing:因果投影 no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限� | 158s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-112620 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
