# Selected parallel eval sweep

- date: 2026-08-22T18:40:30Z
- sweep_start_ts: 20260822-114028
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260822-114030 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 647s | 1 | 2 | 0 | 1 | 0 | 9 | 8 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260822-114030 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
