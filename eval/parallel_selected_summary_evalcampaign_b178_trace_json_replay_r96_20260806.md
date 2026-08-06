# Selected parallel eval sweep

- date: 2026-08-06T12:37:20Z
- sweep_start_ts: 20260806-053718
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 59s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260806-053720 |
| 1 | real_trace_d4_demand_vs_supply | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260806-053720 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
