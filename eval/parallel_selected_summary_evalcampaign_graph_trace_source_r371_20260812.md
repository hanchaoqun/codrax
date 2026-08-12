# Selected parallel eval sweep

- date: 2026-08-12T07:22:06Z
- sweep_start_ts: 20260812-002204
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 291s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260812-002206 |
| 2 | read_combo_trace_current_source_explanation | PASS | - | 306s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260812-002206 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
