# Selected parallel eval sweep

- date: 2026-08-28T13:11:26Z
- sweep_start_ts: 20260828-061124
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 260s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260828-061126 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 374s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260828-061126 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
