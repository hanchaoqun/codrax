# Selected parallel eval sweep

- date: 2026-08-31T20:58:12Z
- sweep_start_ts: 20260831-135811
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 198s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-135813 |
| 1 | sr_java_call_chain | PASS | - | 344s | 1 | 2 | 0 | 1 | 0 | 10 | 10 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260831-135813 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
