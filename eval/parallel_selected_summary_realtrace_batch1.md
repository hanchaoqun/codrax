# Selected parallel eval sweep

- date: 2026-07-05T10:47:04Z
- sweep_start_ts: 20260705-184704
- total cases: 18
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | real_trace_a3_whole_trace_overview | PASS | - | 101s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a3_whole_trace_overview-20260705-184704 |
| 1 | real_trace_a2_wide_window_ratio | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a2_wide_window_ratio-20260705-184704 |
| 3 | real_trace_a4_out_of_range_window | PASS | - | 199s | 1 | 4 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a4_out_of_range_window-20260705-184846 |
| 5 | real_trace_b2_tid_only_waker | PASS | - | 177s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b2_tid_only_waker-20260705-185205 |
| 4 | real_trace_a5_excerpt_degenerate_window | PASS | - | 324s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a5_excerpt_degenerate_window-20260705-185033 |
| 7 | real_trace_b4_missing_thread_miss | PASS | - | 73s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b4_missing_thread_miss-20260705-185557 |
| 6 | real_trace_b3_process_level_rollup | PASS | - | 161s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b3_process_level_rollup-20260705-185502 |
| 8 | real_trace_b5_multi_subject_render | PASS | - | 106s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b5_multi_subject_render-20260705-185711 |
| 9 | real_trace_c2_dstate_iowait | PASS | - | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260705-185744 |
| 10 | real_trace_c3_vsync_periodic | PASS | - | 91s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c3_vsync_periodic-20260705-185859 |
| 11 | real_trace_c4_freq_supply_evidence | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c4_freq_supply_evidence-20260705-190013 |
| 12 | real_trace_d2_chain_via_networkservice | PASS | - | 119s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d2_chain_via_networkservice-20260705-190031 |
| 13 | real_trace_d4_demand_vs_supply | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260705-190135 |
| 14 | real_trace_e1_dual_window_normalized | FAIL | no_text_regex_match:(3\.[0-9]+ *(ms|毫秒)|1[01](\.[0-9]+)? *%) | 277s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260705-190231 |
| 15 | real_trace_e2_cross_trace_asymmetry | FAIL | no_text_regex_match:(时基|时间基准|时间轴|基准|时钟|clock|timebase).{0,60}(不同|不一致|不能直接|� | 223s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260705-190358 |
| 16 | real_trace_f1_exclude_no_code | FAIL | banned:.codrax/blob | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260705-190709 |
| 17 | real_trace_g1_english_dstate | PASS | - | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_g1_english_dstate-20260705-190742 |
| 18 | real_trace_g2_relative_path_inrepo | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_g2_relative_path_inrepo-20260705-190921 |

**Pass: 15 / 18 — Fail/Timeout/LaunchFail: 3**
