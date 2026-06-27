# T11 Explorer Convergence Audit

- date: 2026-06-27T16:41:18Z
- sweep_start_ts: 20260628-003311
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| cangjie_repomap | FAIL | — | 0 | 0 | 0 | 4 | 4 | 1 | 3 | 4 | 8 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 2 | 1 | 2 | 2 | 6 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 4 | 3 | 0 | 2 | 2 | 19 | 1 | 8 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 7 | 1 | 0 | 0 | 1 | 6 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_trace_current_source_explanation | PASS | — | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 1 | 4 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| trace_query_openharmony_bytrace_thread | PASS | — | 0 | 0 | 0 | 4 | 2 | 0 | 0 | 1 | 9 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | — |

**flagged: 1 / 6**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
