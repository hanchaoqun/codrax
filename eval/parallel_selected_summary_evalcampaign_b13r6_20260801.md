# Selected parallel eval sweep

- date: 2026-08-01T00:46:09Z
- sweep_start_ts: 20260731-174608
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 197s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260731-174609 |
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | - | 225s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-174609 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
