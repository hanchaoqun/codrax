# Eval results — read_combo_trace_current_source_explanation

- case: `read combo — trace plus current source explanation profile`
- question: `这段 HiTrace 里 RenderService DoFrame 用时明显偏高；请结合当前源码解释系统如何解析 trace span 并判断这类耗时是否构成性能线索，同时说明证据边界。`
- runs: 1
- timestamp: 20260628-020337

## Verdicts

| run | result | wall(s) | reasons |
|----:|--------|--------:|---------|
| 1 | PASS | 275 | — |

**pass rate: 1 / 1**

## Mechanism trace metrics

| metric | run 1 | median |
|--------|---|------|
| data_rounds | 0 | 0 |
| data_repair_rounds | 0 | 0 |
| data_record_count | 0 | 0 |
| data_action_failed | 0 | 0 |
| data_answer_len | 0 | 0 |
| tool_read_file | 6 | 6 |
| tool_repo_map | 4 | 4 |
| tool_list_files | 0 | 0 |
| tool_trace_query | 0 | 0 |
| tool_mcp_read_resource | 0 | 0 |
| repeated_mcp_resource_reads | 0 | 0 |
| mcp_tool_calls | 0 | 0 |
| source_inventory_lens | 0 | 0 |
| repo_lens_discovery_hints | 1 | 1 |
| transient_retry_checkpoints | 0 | 0 |
| unavailable_tool_attempts | 0 | 0 |
| checkpoint_continuation_broad_hint | 0 | 0 |
| closure_only_repeated | 0 | 0 |
| mermaid_source_repair_applied | 0 | 0 |
| answer_contract_violations | 0 | 0 |
| answer_contract_strict_violations | 0 | 0 |
| answer_contract_advisories | 3 | 3 |
| answer_contract_lane_block_kind_violations | 0 | 0 |
| answer_contract_lane_block_kind_strict_violations | 0 | 0 |
| answer_contract_lane_block_kind_advisories | 0 | 0 |
| repair_debt_checkpoints | 0 | 0 |
| repair_debt_close_ready_filters | 0 | 0 |
| repair_debt_principal_blocking_max | 0 | 0 |
| repair_debt_surgical_grounding_max | 0 | 0 |
| repair_debt_advisory_max | 0 | 0 |
| tool_history_prunes | 0 | 0 |
| max_context_tokens_est | 76841 | 76841 |
| max_context_window | 200000 | 200000 |
| max_context_window_pct | 38 | 38 |
| concrete_values | 7 | 7 |
| synthesis_runs | 0 | 0 |
| function_boundary_push | 0 | 0 |
| enumeration_push | 0 | 0 |
| focus_warning | 0 | 0 |
| t11_gate_skip | 0 | 0 |
| t11_gate_run | 0 | 0 |
| dataflow_intent_lookup | 0 | 0 |
| dataflow_intent_propagate | 0 | 0 |
| midloop_inject | 7 | 7 |
| parallel_sibling_skips | 0 | 0 |
| mixed_origin_autocomplete_blocks | 0 | 0 |
| finalizer_rejects | 0 | 0 |
| finalizer_rewrites | 0 | 0 |
| answer_chain_lines | 1 | 1 |
| analyzer_iters | 5 | 5 |
| explorer_iters | 11 | 11 |
| extractor_iters | 0 | 0 |
| finalizer_iters | 2 | 2 |
| analyzer_dispatches | 1 | 1 |
| explorer_dispatches | 1 | 1 |
| extractor_dispatches | 0 | 0 |
| finalizer_dispatches | 1 | 1 |
| pipeline_dispatches | 4 | 4 |
| completion_lane_fired | 0 | 0 |
| investigation_complete_calls | 1 | 1 |
| investigation_complete_rejects | 0 | 0 |
| repair_plan_lines | 0 | 0 |
| repair_exec_lines | 0 | 0 |
| repair_exec_promote | 0 | 0 |
| repair_exec_failloud | 0 | 0 |
| semantic_quality_dispatches | 0 | 0 |
| semantic_quality_concerns | 0 | 0 |
| strict_decode_remap_events | 0 | 0 |
| strict_decode_carrier_events | 0 | 0 |
| strict_decode_element_shape_events | 0 | 0 |

## Efficiency advisories

| run | advisory | detail |
|----:|----------|--------|
| 1 | high_wall_seconds | wall_seconds=275 limit=180 |
| 1 | repeated_midloop_inject | midloop_inject=7 limit=4 |
| 1 | high_context_tokens | max_context_tokens_est=76841 limit=60000 |
| 1 | high_context_window_pct | max_context_window_pct=38 limit=30 |
| 1 | answer_contract_advisory_churn | answer_contract_advisories=3 limit=0 |

