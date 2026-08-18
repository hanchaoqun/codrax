# Selected parallel eval sweep

- date: 2026-08-18T05:23:32Z
- sweep_start_ts: 20260817-222332
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 170s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-222332 |
| 1 | mr_poly_binding_chain | PASS | - | 356s | 1 | 1 | 0 | 1 | 0 | 7 | 8 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260817-222332 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
