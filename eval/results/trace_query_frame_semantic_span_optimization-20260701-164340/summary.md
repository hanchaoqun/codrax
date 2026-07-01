# Eval results — trace_query_frame_semantic_span_optimization

- case: `trace query — frame drop with on-chain VerifyClass optimization point`
- question: `只分析这份 trace，不分析代码。目标线程是 app-100，请分析它在 5.000s 到 5.007s 这一帧窗口内的丢帧根因。请分层说明直接唤醒/依赖关系、链上主要原因，并单独指出链路上遇到的确定性可优化点（如运行时编译/类校验/着色器编译等）。`
- runs: 3
- timestamp: 20260701-164340

## Verdicts

| run | result | wall(s) | reasons |
|----:|--------|--------:|---------|
| 1 | PASS | 152 | — |
| 2 | PASS | 181 | — |
| 3 | PASS | 150 | — |

**pass rate: 3 / 3**

## Mechanism trace metrics

| metric | run 1|run 2|run 3 | median |
|--------|---|---|---|------|
| data_rounds | 0 | 0 | 0 | 0 |
| data_repair_rounds | 0 | 0 | 0 | 0 |
| data_record_count | 0 | 0 | 0 | 0 |
| data_action_failed | 0 | 0 | 0 | 0 |
| data_answer_len | 0 | 0 | 0 | 0 |
| tool_read_file | 0 | 0 | 0 | 0 |
| tool_repo_map | 0 | 0 | 0 | 0 |
| tool_list_files | 0 | 0 | 0 | 0 |
| tool_trace_query | 3 | 3 | 4 | 3 |
| trace_query_dimension_families | 4 | 4 | 5 | 4 |
| trace_query_root_cause_views | 1 | 1 | 1 | 1 |
| trace_query_wakeup_views | 1 | 1 | 1 | 1 |
| trace_query_blocking_views | 0 | 0 | 1 | 0 |
| trace_query_timeline_views | 1 | 1 | 1 | 1 |
| trace_query_resource_views | 1 | 1 | 1 | 1 |
| trace_query_windowed_calls | 3 | 3 | 4 | 3 |
| trace_query_pid_filtered_calls | 3 | 3 | 0 | 3 |
| trace_query_thread_filtered_calls | 3 | 0 | 4 | 3 |
| trace_query_target_inherited | 0 | 0 | 0 | 0 |
| trace_query_final_projection_blocks | 1 | 1 | 1 | 1 |
| runtime_prestage_dispatches | 1 | 1 | 1 | 1 |
| log_triage_dispatches | 0 | 0 | 0 | 0 |
| perf_triage_dispatches | 1 | 1 | 1 | 1 |
| emit_log_triage_calls | 0 | 0 | 0 | 0 |
| emit_perf_trace_calls | 1 | 1 | 1 | 1 |
| tool_mcp_read_resource | 0 | 0 | 0 | 0 |
| repeated_mcp_resource_reads | 0 | 0 | 0 | 0 |
| mcp_tool_calls | 0 | 0 | 0 | 0 |
| source_inventory_lens | 0 | 0 | 0 | 0 |
| repo_lens_discovery_hints | 0 | 0 | 0 | 0 |
| transient_retry_checkpoints | 0 | 0 | 0 | 0 |
| unavailable_tool_attempts | 0 | 0 | 0 | 0 |
| checkpoint_continuation_broad_hint | 0 | 0 | 0 | 0 |
| closure_only_repeated | 0 | 0 | 0 | 0 |
| mermaid_source_repair_applied | 0 | 0 | 0 | 0 |
| answer_contract_violations | 0 | 0 | 0 | 0 |
| answer_contract_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_advisories | 2 | 0 | 0 | 0 |
| answer_contract_first_pass_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_final_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_auto_repaired_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_violations | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_advisories | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_first_pass_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_final_strict_violations | 0 | 0 | 0 | 0 |
| answer_contract_lane_block_kind_auto_repaired_strict_violations | 0 | 0 | 0 | 0 |
| repair_debt_checkpoints | 0 | 0 | 0 | 0 |
| repair_debt_close_ready_filters | 0 | 0 | 0 | 0 |
| repair_debt_principal_blocking_max | 0 | 0 | 0 | 0 |
| repair_debt_surgical_grounding_max | 0 | 0 | 0 | 0 |
| repair_debt_advisory_max | 0 | 0 | 0 | 0 |
| tool_history_prunes | 0 | 0 | 0 | 0 |
| max_context_tokens_est | 43025 | 53553 | 54574 | 53553 |
| max_context_window | 200000 | 200000 | 200000 | 200000 |
| max_context_window_pct | 22 | 27 | 27 | 27 |
| concrete_values | 0 | 0 | 0 | 0 |
| synthesis_runs | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 |
| enumeration_push | 4 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 |
| t11_gate_skip | 0 | 0 | 0 | 0 |
| t11_gate_run | 0 | 0 | 0 | 0 |
| dataflow_intent_lookup | 1 | 1 | 1 | 1 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 |
| midloop_inject | 0 | 0 | 0 | 0 |
| analyze_refine_dispatches | 0 | 0 | 0 | 0 |
| read_loop_add_proof_selected | 0 | 0 | 0 | 0 |
| read_loop_add_proof_consumed | 0 | 0 | 0 | 0 |
| parallel_sibling_skips | 0 | 0 | 0 | 0 |
| mixed_origin_autocomplete_blocks | 0 | 0 | 0 | 0 |
| finalizer_rejects | 0 | 0 | 0 | 0 |
| finalizer_rewrites | 0 | 0 | 0 | 0 |
| answer_chain_lines | 3 | 1 | 1 | 1 |
| analyzer_iters | 2 | 1 | 1 | 1 |
| explorer_iters | 3 | 2 | 2 | 2 |
| extractor_iters | 0 | 0 | 0 | 0 |
| finalizer_iters | 1 | 1 | 1 | 1 |
| analyzer_dispatches | 1 | 1 | 1 | 1 |
| explorer_dispatches | 1 | 1 | 1 | 1 |
| extractor_dispatches | 0 | 0 | 0 | 0 |
| finalizer_dispatches | 1 | 1 | 1 | 1 |
| pipeline_dispatches | 4 | 4 | 4 | 4 |
| completion_lane_fired | 0 | 0 | 0 | 0 |
| investigation_complete_calls | 1 | 1 | 1 | 1 |
| investigation_complete_rejects | 0 | 0 | 0 | 0 |
| repair_plan_lines | 0 | 0 | 0 | 0 |
| repair_exec_lines | 0 | 0 | 0 | 0 |
| repair_exec_promote | 0 | 0 | 0 | 0 |
| repair_exec_failloud | 0 | 0 | 0 | 0 |
| semantic_quality_dispatches | 0 | 0 | 0 | 0 |
| semantic_quality_concerns | 0 | 0 | 0 | 0 |
| strict_decode_remap_events | 0 | 0 | 0 | 0 |
| strict_decode_carrier_events | 0 | 0 | 0 | 0 |
| strict_decode_element_shape_events | 0 | 0 | 0 | 0 |

## Runtime authority path audit

| run | attachment | authority_path | log_triage | perf_triage | trace_query | emit_log | emit_perf |
|----:|------------|----------------|-----------:|------------:|------------:|---------:|----------:|
| 1 | trace | perf_triage+trace_query | 0 | 1 | 3 | 0 | 1 |
| 2 | trace | perf_triage+trace_query | 0 | 1 | 3 | 0 | 1 |
| 3 | trace | perf_triage+trace_query | 0 | 1 | 4 | 0 | 1 |

## Trace query coverage audit

| run | dimensions | root | wakeup | blocking | timeline | resource | windowed | pid | thread | inherited | final_projection |
|----:|-----------:|-----:|-------:|---------:|---------:|---------:|---------:|----:|-------:|----------:|-----------------:|
| 1 | 4 | 1 | 1 | 0 | 1 | 1 | 3 | 3 | 3 | 0 | 1 |
| 2 | 4 | 1 | 1 | 0 | 1 | 1 | 3 | 3 | 0 | 0 | 1 |
| 3 | 5 | 1 | 1 | 1 | 1 | 1 | 4 | 0 | 4 | 0 | 1 |

## Efficiency advisories

| run | advisory | detail |
|----:|----------|--------|
| 1 | answer_contract_advisory_churn | answer_contract_advisories=2 limit=0 |
| 2 | high_wall_seconds | wall_seconds=181 limit=180 |

