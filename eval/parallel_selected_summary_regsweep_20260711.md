# Selected parallel eval sweep

- date: 2026-07-11T06:04:01Z
- sweep_start_ts: 20260711-140401
- total cases: 81
- parallel: 4
- timeout: 600s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 3 | hitrace_gc_present_no_long | PASS | - | 84s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/hitrace_gc_present_no_long-20260711-140401 |
| 1 | hitrace_artifact_line_anchor | PASS | - | 90s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/hitrace_artifact_line_anchor-20260711-140401 |
| 2 | hitrace_binary_converted_manual | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/hitrace_binary_converted_manual-20260711-140401 |
| 4 | hitrace_jank | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hitrace_jank-20260711-140401 |
| 5 | hitrace_no_long_gc | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/hitrace_no_long_gc-20260711-140526 |
| 8 | read_combo_trace_current_code_dimensions | FAIL | no_regex_match:internal/(analysis|tool|agent|orchestrator|types)/[^[:space:]]+\.go:[0-9]+ | 158s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_dimensions-20260711-140556 |
| 7 | read_combo_trace_current_code_boundary | PASS | - | 244s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_boundary-20260711-140534 |
| 10 | real_trace_a2_wide_window_ratio | PASS | - | 71s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a2_wide_window_ratio-20260711-140834 |
| 9 | read_combo_trace_current_source_explanation | PASS | - | 201s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260711-140647 |
| 6 | trace_query_openharmony_bytrace_thread | PASS | - | 304s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_openharmony_bytrace_thread-20260711-140531 |
| 11 | real_trace_a3_whole_trace_overview | PASS | - | 138s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a3_whole_trace_overview-20260711-140939 |
| 12 | real_trace_a4_out_of_range_window | PASS | - | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a4_out_of_range_window-20260711-140946 |
| 14 | real_trace_b2_tid_only_waker | PASS | - | 127s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b2_tid_only_waker-20260711-141036 |
| 16 | real_trace_b4_missing_thread_miss | PASS | - | 90s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b4_missing_thread_miss-20260711-141212 |
| 13 | real_trace_a5_excerpt_degenerate_window | PASS | - | 216s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a5_excerpt_degenerate_window-20260711-141009 |
| 17 | real_trace_b5_multi_subject_render | PASS | - | 99s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b5_multi_subject_render-20260711-141244 |
| 15 | real_trace_b3_process_level_rollup | FAIL | missing:NetworkService | 187s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b3_process_level_rollup-20260711-141157 |
| 20 | real_trace_c4_freq_supply_evidence | PASS | - | 84s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c4_freq_supply_evidence-20260711-141424 |
| 21 | real_trace_d2_chain_via_networkservice | PASS | - | 77s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d2_chain_via_networkservice-20260711-141505 |
| 22 | real_trace_d4_demand_vs_supply | PASS | - | 97s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260711-141549 |
| 18 | real_trace_c2_dstate_iowait | PASS | - | 260s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260711-141342 |
| 19 | real_trace_c3_vsync_periodic | PASS | - | 286s | 1 | 6 | 0 | 2 | 1 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c3_vsync_periodic-20260711-141344 |
| 25 | real_trace_f1_exclude_no_code | PASS | - | 60s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260711-141802 |
| 24 | real_trace_e2_cross_trace_asymmetry | PASS | - | 110s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260711-141727 |
| 23 | real_trace_e1_dual_window_normalized | PASS | - | 192s | 2 | 1 | 0 | 3 | 2 | 0 | 2 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260711-141623 |
| 26 | real_trace_g1_english_dstate | PASS | - | 93s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_g1_english_dstate-20260711-141831 |
| 27 | real_trace_g2_relative_path_inrepo | PASS | - | 64s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_g2_relative_path_inrepo-20260711-141902 |
| 28 | trace_query_android_perfetto_cpu_frequency_limits | PASS | - | 99s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_android_perfetto_cpu_frequency_limits-20260711-141918 |
| 29 | trace_query_android_perfetto_sched_blocked | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_android_perfetto_sched_blocked-20260711-141936 |
| 31 | trace_query_binder_ipc_peer | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_binder_ipc_peer-20260711-142007 |
| 30 | trace_query_binder_aux_wait | FAIL | no_regex_match:(alloc|alloc_buf|分配).*(128)|128.*(alloc|alloc_buf|data_size) | 138s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_binder_aux_wait-20260711-142005 |
| 32 | trace_query_blocked_reason_chain | PASS | - | 165s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_blocked_reason_chain-20260711-142058 |
| 33 | trace_query_converted_inode_io_pressure | FAIL | no_regex_match:(io_pressure|IO pressure|io pressure|top inode|top_inode|主因 inode).*(0xb9b8e) | 155s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_converted_inode_io_pressure-20260711-142141 |
| 35 | trace_query_donghu_mixed_platform | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_mixed_platform-20260711-142224 |
| 34 | trace_query_core_topology_supply | PASS | - | 250s | 1 | 6 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_core_topology_supply-20260711-142141 |
| 37 | trace_query_donghu_real_short_runnable | PASS | - | 109s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_short_runnable-20260711-142417 |
| 36 | trace_query_donghu_real_frame_multicausal | PASS | - | 201s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260711-142342 |
| 38 | trace_query_frame_semantic_span_optimization | PASS | - | 205s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260711-142418 |
| 39 | trace_query_frame_timeline_flow | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_timeline_flow-20260711-142552 |
| 40 | trace_query_inode_event_search | PASS | - | 127s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_inode_event_search-20260711-142607 |
| 41 | trace_query_inode_io_pressure | PASS | - | 150s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_inode_io_pressure-20260711-142703 |
| 42 | trace_query_path_question_absolute_donghu_short | PASS | - | 109s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_absolute_donghu_short-20260711-142744 |
| 44 | trace_query_path_question_relative_donghu_short | PASS | - | 90s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_relative_donghu_short-20260711-142814 |
| 43 | trace_query_path_question_multi_trace_files | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_multi_trace_files-20260711-142809 |
| 45 | trace_query_path_question_relative_perftrace | PASS | - | 94s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | none | eval/results/trace_query_path_question_relative_perftrace-20260711-142933 |
| 46 | trace_query_path_question_suffixless_trace | PASS | - | 105s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | none | eval/results/trace_query_path_question_suffixless_trace-20260711-142934 |
| 47 | trace_query_perf_quality_harmony_cpu_unknown | PASS | - | 187s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_harmony_cpu_unknown-20260711-142944 |
| 48 | trace_query_perf_quality_raw_fallback | PASS | - | 175s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_raw_fallback-20260711-143003 |
| 50 | trace_query_perf_quality_simpleperf_symbolized | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_simpleperf_symbolized-20260711-143120 |
| 49 | trace_query_perf_quality_simpleperf_proto_offcpu | PASS | - | 233s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_simpleperf_proto_offcpu-20260711-143107 |
| 51 | trace_query_runnable_context_thread_load | PASS | - | 151s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_runnable_context_thread_load-20260711-143251 |
| 53 | trace_query_smartperf_resources | PASS | - | 127s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_smartperf_resources-20260711-143326 |
| 52 | trace_query_running_perf_context | PASS | - | 226s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_running_perf_context-20260711-143257 |
| 55 | trace_query_state_churn_window_stats | FAIL | no_regex_match:(fragments|片段).*(switches|切换)|(switches|切换).*(fragments|片段) no_regex_match:(max_segment|m | 134s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_window_stats-20260711-143522 |
| 54 | trace_query_state_churn_root_cause_rank | PASS | - | 187s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260711-143501 |
| 56 | trace_query_wakeup_background_demotion | PASS | - | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260711-143534 |
| 57 | trace_query_wakeup_causal_io_chain | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260711-143644 |
| 61 | github_issue_dateutil_relativedelta_float | PASS | - | 109s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float-20260711-143914 |
| 58 | trace_query_wakeup_causal_runnable | PASS | - | 245s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260711-143737 |
| 59 | github_issue_chrono_duration_min | FAIL | write_report_failed | 328s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min-20260711-143808 |
| 64 | github_issue_dayjs_duration_nan_symptom | FAIL | plan_not_written apply_not_run | 97s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan_symptom-20260711-144337 |
| 60 | github_issue_chrono_duration_min_symptom | TIMEOUT | exceeded 600s wall-time | 600s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260711-143818 |
| 62 | github_issue_dateutil_relativedelta_float_symptom | FAIL | write_report_failed | 501s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260711-144104 |
| 63 | github_issue_dayjs_duration_nan | PASS | - | 474s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan-20260711-144142 |
| 65 | github_issue_gson_lazy_number | PASS | - | 281s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number-20260711-144515 |
| 66 | github_issue_gson_lazy_number_symptom | PASS | - | 274s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260711-144820 |
| 69 | github_issue_napi_force_wasi_env_symptom | PASS | - | 205s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260711-144955 |
| 68 | github_issue_zod_prefault_symptom | FAIL | write_report_failed | 458s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260711-144937 |
| 67 | github_issue_zod_prefault | FAIL | write_report_failed | 473s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260711-144926 |
| 72 | data_jsonl_filter_count | PASS | - | 35s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260711-145716 |
| 70 | data_basic_sum_with_rules | FAIL | read_exit:1 data_terminal_status:failed | 310s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260711-145255 |
| 74 | cflow_resolve_perkind_vs_global_retry | FAIL | no_regex_match:(RetryBudgetByKind|retry_budget_by_kind|RetryBudgetByKindSettings|retryUsedForKind) | 85s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cflow_resolve_perkind_vs_global_retry-20260711-145752 |
| 71 | data_json_strict_ids | FAIL | read_exit:1 data_terminal_status:failed banned:``` | 371s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260711-145321 |
| 75 | cflow_resolve_retry_storm_early_exit | FAIL | no_regex_match:(newAnalyzeRetryStormDetector|analyzeRetryStormDetector|retry.?storm|重试风暴|storm) | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cflow_resolve_retry_storm_early_exit-20260711-145806 |
| 76 | cond_resolve_dispatch_retryable_predicate | PASS | - | 102s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cond_resolve_dispatch_retryable_predicate-20260711-145918 |
| 77 | cond_resolve_stall_timeout | PASS | - | 165s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cond_resolve_stall_timeout-20260711-145933 |
| 78 | qf_architecture | FAIL | missing:analyze missing:explore missing:extract missing:finalize | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260711-150012 |
| 79 | qf_called_by_typed_relation_query | PASS | - | 131s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260711-150101 |
| 73 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]] | 385s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260711-145719 |
| 81 | mr_cross_repo_compare | PASS | - | 95s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_cross_repo_compare-20260711-150219 |
