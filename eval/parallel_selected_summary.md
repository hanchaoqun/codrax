# Selected parallel eval sweep

- date: 2026-07-15T13:18:06Z
- sweep_start_ts: 20260715-061806
- total cases: 6
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 3 | real_trace_h3_iofam_one_seat | PASS | - | 138s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260715-061806 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260715-061806 |
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260715-061806 |
| 4 | real_trace_h4_supply_thermal_witness | PASS | - | 209s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260715-062025 |
| 6 | real_trace_h6_channel_mixed_display | FAIL | missing:根因排序#1 missing:❶ missing:树读法: missing:各列口径: no_regex_match:(⊚|⊘) | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260715-062137 |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | - | 223s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260715-062110 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**
