# Selected parallel eval sweep

- date: 2026-08-31T04:44:06Z
- sweep_start_ts: 20260830-214404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 198s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-214406 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 | 876s | 1 | 2 | 0 | 1 | 0 | 7 | 6 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260830-214406 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
