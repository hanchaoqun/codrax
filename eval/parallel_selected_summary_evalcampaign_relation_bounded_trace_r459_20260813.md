# Selected parallel eval sweep

- date: 2026-08-14T00:00:33Z
- sweep_start_ts: 20260813-170032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((CPU ?4|cpu=4).{0,240}(2\.10 ?GHz|2\.1 ?GHz|2100 ?MHz|2100000 ?kHz).{0,160}(上限|限制 | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-170033 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 286s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-170033 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
