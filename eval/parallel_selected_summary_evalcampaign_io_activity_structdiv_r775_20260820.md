# Selected parallel eval sweep

- date: 2026-08-20T14:22:45Z
- sweep_start_ts: 20260820-072243
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-072245 |
| 2 | qf_type_relation_loop_controller | PASS | - | 278s | 1 | 1 | 0 | 1 | 0 | 5 | 6 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260820-072245 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
