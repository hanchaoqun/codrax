# Selected parallel eval sweep

- date: 2026-08-31T09:19:54Z
- sweep_start_ts: 20260831-021952
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 278s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-021954 |
| 1 | cangjie_repomap | FAIL | inventory_count_mismatch:extend:got4:want2 inventory_count_mismatch:public_class:got11:want8 | 507s | 1 | 3 | 0 | 1 | 0 | 4 | 5 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260831-021954 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
