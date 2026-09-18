# Selected parallel eval sweep

- date: 2026-09-18T08:39:02Z
- sweep_start_ts: 20260918-013901
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results_hmc_window_stats_20260918

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h2_dstate_dma_fence_triform | FAIL | no_regex_match:(typed )?内核调用点[ =]dma_fence_default_w | 149s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results_hmc_window_stats_20260918/real_trace_h2_dstate_dma_fence_triform-20260918-013903 |
| 2 | real_trace_h3_iofam_one_seat | FAIL | no_regex_match:(单次请求|设备端).*(墙钟|墙上|wall.clock) no_regex_match:41\.329.*(非墙钟|不可相加|non. | 258s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results_hmc_window_stats_20260918/real_trace_h3_iofam_one_seat-20260918-013903 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
