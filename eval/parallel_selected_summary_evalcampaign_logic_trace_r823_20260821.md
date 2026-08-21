# Selected parallel eval sweep

- date: 2026-08-21T19:33:39Z
- sweep_start_ts: 20260821-123337
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 188s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260821-123339 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 612s | 1 | 1 | 0 | 1 | 0 | 13 | 13 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260821-123339 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
