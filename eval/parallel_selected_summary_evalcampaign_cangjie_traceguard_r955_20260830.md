# Selected parallel eval sweep

- date: 2026-08-31T05:19:15Z
- sweep_start_ts: 20260830-221914
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 146s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-221915 |
| 1 | cangjie_repomap | PASS | - | 220s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260830-221915 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
