# Selected parallel eval sweep

- date: 2026-07-02T01:25:02Z
- sweep_start_ts: 20260702-092502
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | read_combo_log_current_source_explanation | PASS | - | 262s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260702-092502 |
| 2 | read_combo_trace_current_source_explanation | PASS | - | 268s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260702-092502 |
| 4 | trace_query_frame_semantic_span_optimization | PASS | - | 208s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260702-092930 |
| 5 | cangjie_repomap | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260702-093259 |
| 3 | trace_query_donghu_real_frame_multicausal | FAIL | banned:still_present banned:not_enough_evidence | 814s | 1 | 11 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260702-092924 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 1**
