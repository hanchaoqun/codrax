# Selected parallel eval sweep

- date: 2026-08-02T03:47:22Z
- sweep_start_ts: 20260801-204720
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 146s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260801-204722 |
| 2 | read_combo_git_diff_hunk_current_code | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_diff_hunk_current_code-20260801-204722 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
