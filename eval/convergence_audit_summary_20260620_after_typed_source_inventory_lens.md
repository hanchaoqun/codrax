# T11 Explorer Convergence Audit

- date: 2026-06-20T12:47:41Z
- sweep_start_ts: 20260620-203549
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| qf_architecture | PASS | — | 0 | 0 | 0 | 4 | 1 | 0 | 0 | 3 | 7 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning auto_repair |
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 3 | 1 | 0 | 1 | 2 | 7 | 1 | 7 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 1 | 4 | 1 | 4 | 12 | 1 | 5 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 13 | 2 | 1 | 1 | 5 | 16 | 1 | 7 | 0 | 0 | 1 | 0 | 0 | 0 | context_prune |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 3 | 1 | 0 | 0 | 1 | 8 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| read_combo_trace_current_source_explanation | PASS | — | 0 | 0 | 0 | 9 | 1 | 0 | 0 | 2 | 12 | 1 | 6 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |

**flagged: 5 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
