# Selected parallel eval sweep

- date: 2026-07-31T23:43:36Z
- sweep_start_ts: 20260731-164334
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | missing:4次(3.774~16.064ms) no_regex_match:自身·D-state(\(对端未解析\))? 36\.757ms | 131s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-164336 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 167s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260731-164336 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
