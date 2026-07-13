# Selected parallel eval sweep

- date: 2026-07-13T18:53:17Z
- sweep_start_ts: 20260713-115317
- total cases: 6
- parallel: 3
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | missing:4次(3.774~16.064ms) no_regex_match:自身·D-state(\(对端未解析\))? 36\.757ms | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260713-115317 |
| 3 | real_trace_h3_iofam_one_seat | FAIL | missing:IO延迟 missing:块设备层 missing:块设备IO(inode) missing:综合评分,非墙钟 | 191s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260713-115317 |
| 4 | real_trace_h4_supply_thermal_witness | PASS | - | 144s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260713-115439 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 308s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260713-115317 |
| 6 | real_trace_h6_channel_mixed_display | PASS | - | 150s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260713-115703 |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | - | 193s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260713-115629 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 2**
