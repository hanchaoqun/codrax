# Selected parallel eval sweep

- date: 2026-08-31T16:41:04Z
- sweep_start_ts: 20260831-094103
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-094104 |
| 2 | cangjie_repomap | PASS | - | 270s | 1 | 3 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260831-094104 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
