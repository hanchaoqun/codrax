# T11 Explorer Convergence Audit

- date: 2026-06-20T10:27:48Z
- sweep_start_ts: 20260620-182250
- cases: 2
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| arkts_repomap | FAIL | — | 0 | 0 | 0 | 5 | 1 | 1 | 1 | 3 | 6 | 1 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 12 | 4 | 4 | 4 | 4 | 17 | 1 | 9 | 0 | 0 | 1 | 0 | 0 | 0 | contract_warning context_prune |

**flagged: 2 / 2**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
