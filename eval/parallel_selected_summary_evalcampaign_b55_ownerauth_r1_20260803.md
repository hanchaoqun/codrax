# Selected parallel eval sweep

- date: 2026-08-04T01:13:28Z
- sweep_start_ts: 20260803-181327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260803-181328 |
| 2 | read_combo_trace_current_source_explanation | PASS | - | 180s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260803-181328 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
