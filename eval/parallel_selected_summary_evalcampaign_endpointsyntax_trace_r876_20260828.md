# Selected parallel eval sweep

- date: 2026-08-28T08:36:46Z
- sweep_start_ts: 20260828-013644
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 261s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260828-013646 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 453s | 1 | 2 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260828-013646 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
