# Selected parallel eval sweep

- date: 2026-07-13T05:33:06Z
- sweep_start_ts: 20260712-223306
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | missing:4次(3.774~16.064ms) no_regex_match:自身·D-state 36\.757ms no_regex_match:等待对象[ =]dma_fence_default_w | 192s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260712-223306 |
| 1 | real_trace_h1_binder_true_false_attribution | FAIL | missing:∿ | 212s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260712-223306 |
| 4 | real_trace_h4_supply_thermal_witness | PASS | - | 144s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260712-223639 |
| 3 | real_trace_h3_iofam_one_seat | PASS | - | 207s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260712-223619 |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | - | 226s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260712-223904 |
| 6 | real_trace_h6_channel_mixed_display | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260712-223947 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 2**
