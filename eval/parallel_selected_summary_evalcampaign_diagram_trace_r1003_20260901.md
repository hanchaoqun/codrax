# Selected parallel eval sweep

- date: 2026-09-01T07:43:17Z
- sweep_start_ts: 20260901-004315
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-004317 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 827s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260901-004317 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
