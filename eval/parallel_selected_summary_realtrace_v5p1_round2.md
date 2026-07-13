# Selected parallel eval sweep

- date: 2026-07-13T08:06:18Z
- sweep_start_ts: 20260713-010618
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | no_regex_match:自身·D-state 36\.757ms | 299s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260713-010618 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 312s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260713-010618 |
| 4 | real_trace_h4_supply_thermal_witness | FAIL | no_log_regex:phase=toolcall .*tool=trace_query missing:157.248 missing:受热限压至 missing:1.53GHz missing:全窗四 | 34s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | missing_runtime_authority | eval/results/real_trace_h4_supply_thermal_witness-20260713-011130 |
| 3 | real_trace_h3_iofam_one_seat | PASS | - | 162s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260713-011117 |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | - | 242s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260713-011204 |
| 6 | real_trace_h6_channel_mixed_display | PASS | - | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260713-011400 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 2**
