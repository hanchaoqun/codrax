# Selected parallel eval sweep

- date: 2026-08-05T11:18:12Z
- sweep_start_ts: 20260805-041811
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_web_manual_summary | FAIL | operation_coverage_ref_missing:material-coverage:v1:_0-9a-f_64_:html_text | 101s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260805-041813 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260805-041813 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
