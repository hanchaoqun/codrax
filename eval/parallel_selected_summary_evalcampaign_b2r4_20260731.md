# Selected parallel eval sweep

- date: 2026-07-31T09:25:37Z
- sweep_start_ts: 20260731-022537
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,160}(不能|不足以| | 133s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260731-022537 |
| 2 | data_multifile_reference_projection | PASS | - | 208s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260731-022537 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
