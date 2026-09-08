# Selected parallel eval sweep

- date: 2026-09-08T07:14:39Z
- sweep_start_ts: 20260908-001438
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap_fixture | PASS | - | 87s | 1 | 3 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260908-001439 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 203s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260908-001439 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
