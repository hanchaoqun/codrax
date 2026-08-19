# Selected parallel eval sweep

- date: 2026-08-19T04:15:54Z
- sweep_start_ts: 20260818-211551
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,160}(不能|不足以| | 127s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260818-211554 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 511s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260818-211554 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
