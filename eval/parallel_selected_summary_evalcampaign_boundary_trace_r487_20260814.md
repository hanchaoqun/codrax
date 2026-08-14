# Selected parallel eval sweep

- date: 2026-08-14T12:57:34Z
- sweep_start_ts: 20260814-055733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:gate\.Run([^A-Za-z0-9_]|$).*RunWith.*gate\.go:[0-9]+ | 136s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-055735 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:(([Dd][-_ ]state|D 状态|不可中断).{0,120}(0\.000|0 ?ms)|(^|[^0-9])(0\.000|0 ?ms).{0,120}( | 259s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260814-055734 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
