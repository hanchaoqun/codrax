# T11 Explorer Convergence Audit

- date: 2026-06-20T02:54:03Z
- sweep_start_ts: 20260620-104506
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 11 | 2 | 0 | 0 | 3 | 19 | 0 | 8 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| qf_architecture | PASS | — | 0 | 0 | 0 | 2 | 2 | 0 | 0 | 3 | 4 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_log_current_code_dimensions | FAIL | — | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |
| trace_query_wakeup_causal_io_chain | PASS | — | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 2 | 4 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| arkts_repomap | FAIL | — | 0 | 0 | 0 | 4 | 0 | 1 | 0 | 3 | 16 | 1 | 9 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |
| sr_cpp_virtual_chain | PASS | — | 0 | 0 | 0 | 4 | 1 | 0 | 0 | 1 | 4 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning |

**flagged: 3 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
