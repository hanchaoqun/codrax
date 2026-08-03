# Selected parallel eval sweep

- date: 2026-08-03T14:03:44Z
- sweep_start_ts: 20260803-070342
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_trace_current_source_explanation | PASS | - | 298s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260803-070344 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | read_exit:143 | 538s | 1 | 1 | 0 | 1 | 0 | 34 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260803-070344 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
