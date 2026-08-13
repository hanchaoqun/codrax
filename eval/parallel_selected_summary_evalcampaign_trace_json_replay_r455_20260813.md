# Selected parallel eval sweep

- date: 2026-08-13T20:36:39Z
- sweep_start_ts: 20260813-133638
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 44s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260813-133639 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | missing:Trace missing:因果投影 banned:132.041 missing_principal:157.248 no_principal_regex_match:([Rr]unning.{0,120} | 225s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-133639 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
