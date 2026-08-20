# Selected parallel eval sweep

- date: 2026-08-20T23:05:14Z
- sweep_start_ts: 20260820-160512
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 175s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-160514 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 767s | 1 | 2 | 0 | 1 | 0 | 12 | 12 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-160514 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
