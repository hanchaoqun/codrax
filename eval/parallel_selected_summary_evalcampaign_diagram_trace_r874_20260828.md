# Selected parallel eval sweep

- date: 2026-08-28T07:31:52Z
- sweep_start_ts: 20260828-003151
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 175s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260828-003153 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 546s | 1 | 2 | 0 | 1 | 0 | 9 | 9 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260828-003153 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
