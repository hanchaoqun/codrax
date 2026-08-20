# Selected parallel eval sweep

- date: 2026-08-20T19:18:06Z
- sweep_start_ts: 20260820-121805
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 271s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-121806 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 | 1530s | 1 | 2 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-121806 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
