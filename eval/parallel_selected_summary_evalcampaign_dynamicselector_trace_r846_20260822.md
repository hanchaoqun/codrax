# Selected parallel eval sweep

- date: 2026-08-22T07:14:09Z
- sweep_start_ts: 20260822-001409
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 207s | 1 | 1 | 0 | 1 | 0 | 3 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260822-001409 |
| 1 | sr_py_registry_dispatch | PASS | - | 693s | 1 | 1 | 0 | 1 | 0 | 11 | 11 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260822-001409 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
