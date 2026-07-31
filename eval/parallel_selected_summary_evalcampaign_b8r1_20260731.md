# Selected parallel eval sweep

- date: 2026-07-31T17:31:37Z
- sweep_start_ts: 20260731-103136
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_d2_chain_via_networkservice | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d2_chain_via_networkservice-20260731-103137 |
| 2 | data_join_entity_reconcile | PASS | - | 171s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260731-103137 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
