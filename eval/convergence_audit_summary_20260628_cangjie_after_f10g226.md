# T11 Explorer Convergence Audit

- date: 2026-06-28T13:05:34Z
- sweep_start_ts: 20260628-210308
- cases: 1
- runs_per_case: 1
- parallel: 1
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | refine | proof | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------:|------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 8 | 2 | 0 | 2 | 2 | 7 | 1 | 3 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |

**flagged: 0 / 1**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `repair_churn` = deterministic repair coincided with finalizer retry/reject/rewrite, suggesting repair instability rather than harmless carrier normalization; `adaptive_loop` = AnalyzeRefine or read-loop add-proof was consumed more than once in one run; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `context_prune` = tool history pruning occurred. Lossless deterministic carrier/render repairs remain visible in per-case metrics/advisories but do not create a top-level flag by themselves.
