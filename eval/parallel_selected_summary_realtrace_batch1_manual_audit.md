# Selected Eval Manual Audit Scaffold

- date: 2026-07-05T10:47:04Z
- sweep_start_ts: 20260705-184704
- total cases: 18
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_a3_whole_trace_overview | PASS | eval/results/real_trace_a3_whole_trace_overview-20260705-184704 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 101s | 24 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 1 | real_trace_a2_wide_window_ratio | PASS | eval/results/real_trace_a2_wide_window_ratio-20260705-184704 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 37 | read=2,repo_map=0,list=0,trace=13,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | real_trace_a4_out_of_range_window | PASS | eval/results/real_trace_a4_out_of_range_window-20260705-184846 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 29 | read=2,repo_map=0,list=0,trace=10,source_lens=0 | midloop=2,inv=8/4,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | real_trace_b2_tid_only_waker | PASS | eval/results/real_trace_b2_tid_only_waker-20260705-185205 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 30 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 4 | real_trace_a5_excerpt_degenerate_window | PASS | eval/results/real_trace_a5_excerpt_degenerate_window-20260705-185033 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 324s | 33 | read=3,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=4/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 7 | real_trace_b4_missing_thread_miss | PASS | eval/results/real_trace_b4_missing_thread_miss-20260705-185557 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 73s | 22 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | real_trace_b3_process_level_rollup | PASS | eval/results/real_trace_b3_process_level_rollup-20260705-185502 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 33 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 8 | real_trace_b5_multi_subject_render | PASS | eval/results/real_trace_b5_multi_subject_render-20260705-185711 | log_regex,trace_attachment | perf_triage+trace_query | 106s | 33 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 9 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260705-185744 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 32 | read=1,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 10 | real_trace_c3_vsync_periodic | PASS | eval/results/real_trace_c3_vsync_periodic-20260705-185859 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 91s | 24 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 11 | real_trace_c4_freq_supply_evidence | PASS | eval/results/real_trace_c4_freq_supply_evidence-20260705-190013 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 81s | 26 | read=1,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 12 | real_trace_d2_chain_via_networkservice | PASS | eval/results/real_trace_d2_chain_via_networkservice-20260705-190031 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 119s | 31 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 13 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260705-190135 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 142s | 34 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 14 | real_trace_e1_dual_window_normalized | FAIL | eval/results/real_trace_e1_dual_window_normalized-20260705-190231 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 277s | 42 | read=1,repo_map=0,list=0,trace=15,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 15 | real_trace_e2_cross_trace_asymmetry | FAIL | eval/results/real_trace_e2_cross_trace_asymmetry-20260705-190358 | log_regex,answer_regex,answer_contains | none | 223s | 36 | read=2,repo_map=0,list=0,trace=12,source_lens=0 | midloop=2,inv=5/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 16 | real_trace_f1_exclude_no_code | FAIL | eval/results/real_trace_f1_exclude_no_code-20260705-190709 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 132s | 29 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 17 | real_trace_g1_english_dstate | PASS | eval/results/real_trace_g1_english_dstate-20260705-190742 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 37 | read=4,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 18 | real_trace_g2_relative_path_inrepo | PASS | eval/results/real_trace_g2_relative_path_inrepo-20260705-190921 | log_regex,answer_regex,answer_contains | none | 132s | 35 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
