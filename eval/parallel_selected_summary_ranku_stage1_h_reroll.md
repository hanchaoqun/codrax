# Selected parallel eval sweep

- date: 2026-07-13T19:02:00Z
- sweep_start_ts: 20260713-120200
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_h3_iofam_one_seat | FAIL | missing:IO延迟 missing:块设备层 missing:块设备IO(inode) missing:综合评分,非墙钟 | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260713-120200 |
| 1 | real_trace_h2_dstate_dma_fence_triform | PASS | - | 192s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260713-120200 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
