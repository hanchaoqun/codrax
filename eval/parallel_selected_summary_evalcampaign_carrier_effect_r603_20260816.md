# Selected parallel eval sweep

- date: 2026-08-17T02:18:55Z
- sweep_start_ts: 20260816-191853
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,160}(不能|不足以| | 172s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-191855 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 481s | 1 | 2 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260816-191855 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
