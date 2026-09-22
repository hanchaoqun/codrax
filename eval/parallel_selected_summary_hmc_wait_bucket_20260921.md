# Selected parallel eval sweep

- date: 2026-09-22T01:31:03Z
- sweep_start_ts: 20260921-183101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_wait_bucket_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | no_regex_match:(typed )?内核调用点[ =]dma_fence_default_w | 110s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_wait_bucket_20260921/real_trace_h2_dstate_dma_fence_triform-20260921-183103 |
| 1 | real_trace_g1_english_dstate | PASS | - | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_wait_bucket_20260921/real_trace_g1_english_dstate-20260921-183103 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
