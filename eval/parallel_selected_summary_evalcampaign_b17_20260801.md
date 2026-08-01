# Selected parallel eval sweep

- date: 2026-08-01T03:32:53Z
- sweep_start_ts: 20260731-203252
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_path_question_multi_trace_files | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_multi_trace_files-20260731-203253 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 115s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-203253 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
