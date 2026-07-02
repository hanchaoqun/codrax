# Selected parallel eval sweep

- date: 2026-07-02T00:29:49Z
- sweep_start_ts: 20260702-082949
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | read_combo_trace_current_source_explanation | PASS | - | 144s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260702-082949 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 178s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260702-082949 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
