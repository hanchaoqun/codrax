# T11 Explorer Convergence Audit

- date: 2026-06-26T07:14:43Z
- sweep_start_ts: 20260626-150711
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 2 | 5 | 0 | 5 | 3 | 17 | 1 | 11 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 1 | 1 | 1 | 1 | 8 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 8 | 1 | 4 | 1 | 4 | 5 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| trace_query_openharmony_bytrace_thread | PASS | — | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 5 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 9 | 2 | 0 | 1 | 1 | 9 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| patch_go_typo | PASS | — | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | — |

**flagged: 1 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
