# Selected Eval Manual Audit Scaffold

- date: 2026-07-05T17:12:33Z
- sweep_start_ts: 20260706-011233
- total cases: 18
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_a3_whole_trace_overview | PASS | eval/results/real_trace_a3_whole_trace_overview-20260706-011233 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 82s | 25 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 1 | real_trace_a2_wide_window_ratio | PASS | eval/results/real_trace_a2_wide_window_ratio-20260706-011233 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 141s | 29 | read=1,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | real_trace_a4_out_of_range_window | PASS | eval/results/real_trace_a4_out_of_range_window-20260706-011356 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 127s | 26 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | real_trace_a5_excerpt_degenerate_window | PASS | eval/results/real_trace_a5_excerpt_degenerate_window-20260706-011455 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 21 | read=3,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | real_trace_b2_tid_only_waker | PASS | eval/results/real_trace_b2_tid_only_waker-20260706-011604 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 33 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=4/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 7 | real_trace_b4_missing_thread_miss | PASS | eval/results/real_trace_b4_missing_thread_miss-20260706-011846 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 60s | 22 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=3/2,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | real_trace_b3_process_level_rollup | FAIL | eval/results/real_trace_b3_process_level_rollup-20260706-011737 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 34 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 8 | real_trace_b5_multi_subject_render | PASS | eval/results/real_trace_b5_multi_subject_render-20260706-011947 | log_regex,trace_attachment | perf_triage+trace_query | 91s | 28 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 10 | real_trace_c3_vsync_periodic | PASS | eval/results/real_trace_c3_vsync_periodic-20260706-012119 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 99s | 24 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 9 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260706-012005 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 38 | read=1,repo_map=0,list=0,trace=11,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 12 | real_trace_d2_chain_via_networkservice | PASS | eval/results/real_trace_d2_chain_via_networkservice-20260706-012301 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 87s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 11 | real_trace_c4_freq_supply_evidence | FAIL | eval/results/real_trace_c4_freq_supply_evidence-20260706-012258 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 104s | 26 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 13 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260706-012429 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 132s | 32 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 14 | real_trace_e1_dual_window_normalized | PASS | eval/results/real_trace_e1_dual_window_normalized-20260706-012443 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 206s | 31 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 15 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260706-012641 | log_regex,answer_regex,answer_contains | none | 113s | 36 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 16 | real_trace_f1_exclude_no_code | PASS | eval/results/real_trace_f1_exclude_no_code-20260706-012810 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 59s | 24 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 17 | real_trace_g1_english_dstate | PASS | eval/results/real_trace_g1_english_dstate-20260706-012834 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 81s | 32 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 18 | real_trace_g2_relative_path_inrepo | PASS | eval/results/real_trace_g2_relative_path_inrepo-20260706-012909 | log_regex,answer_regex,answer_contains | none | 171s | 37 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=3/2,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
