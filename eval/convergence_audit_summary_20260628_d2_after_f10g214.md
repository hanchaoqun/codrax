# T11 Explorer Convergence Audit

- date: 2026-06-28T09:48:52Z
- sweep_start_ts: 20260628-174140
- cases: 6
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 7 | 13 | 0 | 11 | 2 | 30 | 2 | 7 | 0 | 0 | 1 | 0 | 0 | 0 | explorer_long wide_search context_prune |
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 2 | 1 | 1 | 4 | 5 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 8 | 6 | 1 | 6 | 3 | 13 | 1 | 6 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_log_current_source_explanation | PASS | — | 0 | 0 | 0 | 6 | 4 | 0 | 0 | 2 | 6 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| read_combo_trace_current_source_explanation | FAIL | — | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 | 1 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |

**flagged: 2 / 5**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
