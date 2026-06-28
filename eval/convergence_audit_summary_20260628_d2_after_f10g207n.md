# T11 Explorer Convergence Audit

- date: 2026-06-28T07:58:22Z
- sweep_start_ts: 20260628-155000
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 3 | 2 | 0 | 1 | 2 | 8 | 1 | 5 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| arkts_repomap | PASS | — | 0 | 0 | 0 | 0 | 16 | 1 | 16 | 4 | 8 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | wide_search |
| cangjie_repomap | FAIL | — | 0 | 0 | 0 | 2 | 9 | 2 | 8 | 4 | 13 | 1 | 4 | 0 | 0 | 1 | 0 | 0 | 0 | verdict wide_search |
| trace_query_openharmony_bytrace_thread | PASS | — | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 1 | 4 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 4 | 2 | 0 | 0 | 2 | 11 | 1 | 4 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_trace_current_source_explanation | FAIL | — | 0 | 0 | 0 | 2 | 1 | 0 | 0 | 1 | 5 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |

**flagged: 3 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
