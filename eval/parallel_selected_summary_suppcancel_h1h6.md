# Selected parallel eval sweep

- date: 2026-07-14T20:04:58Z
- sweep_start_ts: 20260714-130458
- total cases: 6
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 3 | real_trace_h3_iofam_one_seat | PASS | - | 193s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260714-130458 |
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | - | 320s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260714-130458 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 355s | 2 | 1 | 0 | 1 | 0 | 8 | 4 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260714-130458 |
| 4 | real_trace_h4_supply_thermal_witness | FAIL | banned:132.041 | 230s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260714-130812 |
| 6 | real_trace_h6_channel_mixed_display | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260714-131054 |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260714-131019 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**
