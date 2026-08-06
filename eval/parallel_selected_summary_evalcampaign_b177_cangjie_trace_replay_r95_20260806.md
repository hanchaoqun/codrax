# Selected parallel eval sweep

- date: 2026-08-06T12:16:40Z
- sweep_start_ts: 20260806-051639
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | - | 123s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260806-051640 |
| 1 | cangjie_repomap | PASS | - | 135s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-051640 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
