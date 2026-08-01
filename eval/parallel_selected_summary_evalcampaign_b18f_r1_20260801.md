# Selected parallel eval sweep

- date: 2026-08-01T05:16:43Z
- sweep_start_ts: 20260731-221641
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-221643 |
| 1 | qf_type_relation_loop_controller | PASS | - | 270s | 1 | 4 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260731-221643 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
