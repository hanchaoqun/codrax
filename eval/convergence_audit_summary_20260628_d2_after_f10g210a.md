# T11 Explorer Convergence Audit

- date: 2026-06-28T08:54:32Z
- sweep_start_ts: 20260628-165112
- cases: 3
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 4 | 1 | 4 | 3 | 7 | 1 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | — |
| cangjie_repomap | FAIL | — | 0 | 0 | 0 | 8 | 5 | 0 | 5 | 4 | 12 | 1 | 7 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |
| qf_relation_subagent_registry | PASS | — | 0 | 0 | 0 | 3 | 2 | 0 | 1 | 4 | 6 | 1 | 4 | 0 | 0 | 1 | 0 | 0 | 0 | — |

**flagged: 1 / 3**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
