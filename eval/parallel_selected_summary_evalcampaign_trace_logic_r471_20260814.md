# Selected parallel eval sweep

- date: 2026-08-14T06:51:09Z
- sweep_start_ts: 20260813-235108
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:(([Dd][-_ ]state|D 状态|不可中断).{0,120}(0\.000|0 ?ms)|(^|[^0-9])(0\.000|0 ?ms).{0,120}( | 143s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-235109 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 488s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-235109 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
