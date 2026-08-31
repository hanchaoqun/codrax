# Selected parallel eval sweep

- date: 2026-08-31T06:24:56Z
- sweep_start_ts: 20260830-232454
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 170s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-232456 |
| 1 | cangjie_repomap | FAIL | degraded_answer_checks_skipped:1 | 1601s | 1 | 3 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260830-232456 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
