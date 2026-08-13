# Selected parallel eval sweep

- date: 2026-08-13T20:15:54Z
- sweep_start_ts: 20260813-131553
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | missing:因果投影 no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,1 | 186s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-131554 |
| 2 | data_json_strict_ids | PASS | - | 335s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260813-131554 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
