# Selected parallel eval sweep

- date: 2026-07-05T17:12:33Z
- sweep_start_ts: 20260706-011233
- total cases: 18
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_a3_whole_trace_overview | PASS | - | 82s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a3_whole_trace_overview-20260706-011233 |
| 1 | real_trace_a2_wide_window_ratio | PASS | - | 141s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a2_wide_window_ratio-20260706-011233 |
| 3 | real_trace_a4_out_of_range_window | PASS | - | 127s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a4_out_of_range_window-20260706-011356 |
| 4 | real_trace_a5_excerpt_degenerate_window | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a5_excerpt_degenerate_window-20260706-011455 |
| 5 | real_trace_b2_tid_only_waker | PASS | - | 162s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b2_tid_only_waker-20260706-011604 |
| 7 | real_trace_b4_missing_thread_miss | PASS | - | 60s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b4_missing_thread_miss-20260706-011846 |
| 6 | real_trace_b3_process_level_rollup | FAIL | missing:NetworkService | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b3_process_level_rollup-20260706-011737 |
| 8 | real_trace_b5_multi_subject_render | PASS | - | 91s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b5_multi_subject_render-20260706-011947 |
| 10 | real_trace_c3_vsync_periodic | PASS | - | 99s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c3_vsync_periodic-20260706-012119 |
| 9 | real_trace_c2_dstate_iowait | PASS | - | 176s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260706-012005 |
| 12 | real_trace_d2_chain_via_networkservice | PASS | - | 87s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d2_chain_via_networkservice-20260706-012301 |
| 11 | real_trace_c4_freq_supply_evidence | FAIL | no_regex_match:(807000|807 ?MHz|0\.807 ?GHz) | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c4_freq_supply_evidence-20260706-012258 |
| 13 | real_trace_d4_demand_vs_supply | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260706-012429 |
| 14 | real_trace_e1_dual_window_normalized | PASS | - | 206s | 1 | 1 | 0 | 3 | 2 | 0 | 3 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260706-012443 |
| 15 | real_trace_e2_cross_trace_asymmetry | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260706-012641 |
| 16 | real_trace_f1_exclude_no_code | PASS | - | 59s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260706-012810 |
| 17 | real_trace_g1_english_dstate | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_g1_english_dstate-20260706-012834 |
| 18 | real_trace_g2_relative_path_inrepo | PASS | - | 171s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_g2_relative_path_inrepo-20260706-012909 |

**Pass: 16 / 18 — Fail/Timeout/LaunchFail: 2**
