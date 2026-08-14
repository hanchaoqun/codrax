# Selected parallel eval sweep

- date: 2026-08-14T06:13:06Z
- sweep_start_ts: 20260813-231305
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | missing_principal:70.338 no_principal_regex_match:(([Ss]_[Ss]leep|[Ss]leep|睡眠).{0,120}70\.338|70\.338.{0,120}([Ss]_[ | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-231306 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 369s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-231306 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
