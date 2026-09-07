# Selected parallel eval sweep

- date: 2026-09-07T09:08:04Z
- sweep_start_ts: 20260907-020803
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | missing:12 missing:39.157 no_regex_match:(typed )?内核调用点[ =]dma_fence_default_w | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260907-020804 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | missing:仅关系凭证 missing:优化项,非根因 | 326s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260907-020804 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
