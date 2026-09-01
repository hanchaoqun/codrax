# Selected parallel eval sweep

- date: 2026-09-01T09:05:17Z
- sweep_start_ts: 20260901-020515
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 235s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-020517 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 | 618s | 1 | 1 | 0 | 1 | 0 | 10 | 9 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260901-020517 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
