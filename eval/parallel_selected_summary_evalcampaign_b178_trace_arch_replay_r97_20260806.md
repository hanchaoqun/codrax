# Selected parallel eval sweep

- date: 2026-08-06T12:44:13Z
- sweep_start_ts: 20260806-054411
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_architecture | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260806-054413 |
| 1 | real_trace_d4_demand_vs_supply | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260806-054413 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
