# T11 Explorer Convergence Audit

- date: 2026-06-20T10:12:40Z
- sweep_start_ts: 20260620-175858
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 2 | 2 | 1 | 3 | 9 | 1 | 5 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 14 | 3 | 5 | 1 | 4 | 15 | 1 | 8 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning context_prune |
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 3 | 1 | 0 | 1 | 3 | 8 | 1 | 4 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| qf_architecture | PASS | — | 0 | 0 | 0 | 23 | 8 | 0 | 0 | 3 | 42 | 0 | 13 | 0 | 0 | 1 | 0 | 0 | 0 | explorer_long contract_warning auto_repair |
| sr_cpp_virtual_chain | PASS | — | 0 | 0 | 0 | 5 | 1 | 0 | 0 | 2 | 5 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 13 | 3 | 0 | 0 | 5 | 18 | 1 | 5 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |

**flagged: 5 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
