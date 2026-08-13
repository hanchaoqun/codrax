# Selected parallel eval sweep

- date: 2026-08-13T14:34:43Z
- sweep_start_ts: 20260813-073442
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | no_regex_match:等待对象[ =]dma_fence_default_w | 179s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-073444 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 257s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-073443 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
