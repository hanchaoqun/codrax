# Selected parallel eval sweep

- date: 2026-08-14T02:50:48Z
- sweep_start_ts: 20260813-195047
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | banned:132.041 no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,160}(� | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-195048 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 252s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-195048 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
