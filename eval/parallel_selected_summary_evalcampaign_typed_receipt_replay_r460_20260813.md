# Selected parallel eval sweep

- date: 2026-08-14T00:27:18Z
- sweep_start_ts: 20260813-172717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 218s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-172718 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:((D-state|D state|不可中断).{0,120}(0\.000|0 ?ms)|(^|[^0-9])(0\.000|0 ?ms).{0,120}(D-state| | 292s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-172718 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
