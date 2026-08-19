# Selected parallel eval sweep

- date: 2026-08-19T15:21:18Z
- sweep_start_ts: 20260819-082117
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_regex_match:(([Rr]unning|实际运行|运行占|运行时间).{0,120}157\.248|157\.248.{0,120}([Rr]unning|� | 101s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260819-082119 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 508s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-082118 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
