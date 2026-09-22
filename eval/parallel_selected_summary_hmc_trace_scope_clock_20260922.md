# Selected parallel eval sweep

- date: 2026-09-22T14:01:55Z
- sweep_start_ts: 20260922-070154
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_trace_scope_clock_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 181s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_trace_scope_clock_20260922/trace_query_frame_semantic_span_optimization-20260922-070155 |
| 1 | trace_query_jank_field_inventory | FAIL | no_primary_text_regex_match:((单位|量纲).{0,48}(换算|转换)|(纳秒|ns).{0,80}(换算|转换).{0,80}(秒|seconds) | 202s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_trace_scope_clock_20260922/trace_query_jank_field_inventory-20260922-070155 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
