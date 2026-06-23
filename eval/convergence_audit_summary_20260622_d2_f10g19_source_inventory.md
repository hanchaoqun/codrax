# T11 Explorer Convergence Audit

- date: 2026-06-22T10:27:56Z
- sweep_start_ts: 20260622-180755
- cases: 2
- runs_per_case: 1
- parallel: 2
- results_root: eval/results/d2_f10g19_20260622

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| arkts_repomap | PASS | — | 0 | 0 | 0 | 6 | 1 | 1 | 1 | 1 | 8 | 1 | 4 | 0 | 0 | 2 | 2 | 0 | 0 | finalizer |
| cangjie_repomap | TIMEOUT | — | 0 | 0 | 0 | 17 | 34 | 3 | 33 | 3 | 77 | 3 | 33 | 0 | 0 | 1 | 0 | 0 | 0 | verdict explorer_long wide_search context_prune |

**flagged: 2 / 2**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged advisory violations; `auto_repair` = renderer/compat auto-repair was applied; `context_prune` = tool history pruning occurred.
