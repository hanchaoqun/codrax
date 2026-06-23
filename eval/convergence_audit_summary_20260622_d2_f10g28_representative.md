# T11 Explorer Convergence Audit

- date: 2026-06-22T14:24:04Z
- sweep_start_ts: 20260622-221024
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 3 | 2 | 3 | 3 | 6 | 3 | 3 | 0 | 0 | 2 | 2 | 0 | 0 | finalizer explorer_long |
| cangjie_repomap | FAIL | — | 0 | 0 | 0 | 8 | 2 | 2 | 2 | 3 | 10 | 1 | 5 | 0 | 0 | 4 | 8 | 0 | 0 | verdict finalizer |
| cangjie_repomap_fixture | PASS | — | 0 | 0 | 0 | 3 | 1 | 0 | 1 | 2 | 4 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| qf_type_relation_loop_controller | PASS | — | 0 | 0 | 0 | 34 | 0 | 0 | 0 | 2 | 27 | 4 | 11 | 0 | 0 | 1 | 0 | 0 | 0 | explorer_long wide_search contract_warning auto_repair |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 8 | 3 | 0 | 1 | 2 | 11 | 1 | 6 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |
| read_combo_trace_current_source_explanation | FAIL | — | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 2 | 5 | 1 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | verdict contract_warning |

**flagged: 5 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
