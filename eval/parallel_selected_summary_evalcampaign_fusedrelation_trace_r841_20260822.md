# Selected parallel eval sweep

- date: 2026-08-22T04:53:22Z
- sweep_start_ts: 20260821-215321
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 234s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260821-215322 |
| 1 | sr_ts_workspace_chain | PASS | - | 238s | 1 | 2 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260821-215322 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
