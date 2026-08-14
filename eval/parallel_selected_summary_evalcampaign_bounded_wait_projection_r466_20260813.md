# Selected parallel eval sweep

- date: 2026-08-14T03:41:01Z
- sweep_start_ts: 20260813-204100
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:(([Dd][-_ ]state|D 状态|不可中断).{0,120}(0\.000|0 ?ms)|(^|[^0-9])(0\.000|0 ?ms).{0,120}( | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-204102 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 420s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-204102 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
