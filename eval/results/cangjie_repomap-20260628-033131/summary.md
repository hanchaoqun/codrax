# Eval results — cangjie_repomap

- case: `repomap Cangjie extend + foreign func + public class enumeration`
- question: `仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。`
- runs: 1
- timestamp: 20260628-033131

## Verdicts

| run | result | wall(s) | reasons |
|----:|--------|--------:|---------|
| 1 | PASS | 251 | — |

**pass rate: 1 / 1**

## Mechanism trace metrics

| metric | run 1 | median |
|--------|---|------|
| data_rounds | 0 | 0 |
| data_repair_rounds | 0 | 0 |
| data_record_count | 0 | 0 |
| data_action_failed | 0 | 0 |
| data_answer_len | 0 | 0 |
| tool_read_file | 13 | 13 |
| tool_repo_map | 4 | 4 |
| tool_list_files | 2 | 2 |
| tool_trace_query | 0 | 0 |
| tool_mcp_read_resource | 0 | 0 |
| repeated_mcp_resource_reads | 0 | 0 |
| mcp_tool_calls | 0 | 0 |
| source_inventory_lens | 4 | 4 |
| repo_lens_discovery_hints | 0 | 0 |
| transient_retry_checkpoints | 0 | 0 |
| unavailable_tool_attempts | 0 | 0 |
| checkpoint_continuation_broad_hint | 0 | 0 |
| closure_only_repeated | 0 | 0 |
| mermaid_source_repair_applied | 0 | 0 |
| answer_contract_violations | 0 | 0 |
| answer_contract_strict_violations | 0 | 0 |
| answer_contract_advisories | 0 | 0 |
| answer_contract_lane_block_kind_violations | 0 | 0 |
| answer_contract_lane_block_kind_strict_violations | 0 | 0 |
| answer_contract_lane_block_kind_advisories | 0 | 0 |
| repair_debt_checkpoints | 0 | 0 |
| repair_debt_close_ready_filters | 0 | 0 |
| repair_debt_principal_blocking_max | 0 | 0 |
| repair_debt_surgical_grounding_max | 0 | 0 |
| repair_debt_advisory_max | 0 | 0 |
| tool_history_prunes | 0 | 0 |
| max_context_tokens_est | 59873 | 59873 |
| max_context_window | 200000 | 200000 |
| max_context_window_pct | 30 | 30 |
| concrete_values | 0 | 0 |
| synthesis_runs | 0 | 0 |
| function_boundary_push | 0 | 0 |
| enumeration_push | 7 | 7 |
| focus_warning | 0 | 0 |
| t11_gate_skip | 0 | 0 |
| t11_gate_run | 0 | 0 |
| dataflow_intent_lookup | 0 | 0 |
| dataflow_intent_propagate | 0 | 0 |
| midloop_inject | 8 | 8 |
| parallel_sibling_skips | 0 | 0 |
| mixed_origin_autocomplete_blocks | 0 | 0 |
| finalizer_rejects | 0 | 0 |
| finalizer_rewrites | 0 | 0 |
| answer_chain_lines | 3 | 3 |
| analyzer_iters | 2 | 2 |
| explorer_iters | 18 | 18 |
| extractor_iters | 0 | 0 |
| finalizer_iters | 1 | 1 |
| analyzer_dispatches | 1 | 1 |
| explorer_dispatches | 1 | 1 |
| extractor_dispatches | 0 | 0 |
| finalizer_dispatches | 1 | 1 |
| pipeline_dispatches | 3 | 3 |
| completion_lane_fired | 0 | 0 |
| investigation_complete_calls | 4 | 4 |
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
| 1 | high_wall_seconds | wall_seconds=251 limit=180 |
| 1 | high_explorer_iters | explorer_iters=18 limit=12 |
| 1 | repeated_midloop_inject | midloop_inject=8 limit=4 |
| 1 | high_source_reads | tool_read_file=13 limit=8 |
