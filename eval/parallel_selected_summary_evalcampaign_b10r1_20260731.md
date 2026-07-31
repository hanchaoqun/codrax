# Selected parallel eval sweep

- date: 2026-07-31T19:01:21Z
- sweep_start_ts: 20260731-120119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260731-120121 |
| 1 | read_combo_trace_current_code_boundary | PASS | - | 184s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_boundary-20260731-120121 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
