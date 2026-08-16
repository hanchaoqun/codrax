# T11 Explorer Convergence Audit

- date: 2026-08-16T14:01:08Z
- sweep_start_ts: 20260816-065526
- cases: 2
- runs_per_case: 1
- parallel: 2
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | refine | proof | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------:|------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| read_combo_answer_document_tools | PASS | — | 0 | 0 | 0 | 8 | 1 | 0 | 0 | 3 | 22 | 1 | 18 | 0 | 0 | 0 | 0 | 6 | 5 | 0 | 0 | finalizer repair_churn |
| real_trace_h4_supply_thermal_witness | FAIL | — | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 5 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | verdict |

**flagged: 2 / 2**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `repair_churn` = deterministic repair coincided with finalizer retry/reject/rewrite, suggesting repair instability rather than harmless carrier normalization; `adaptive_loop` = AnalyzeRefine or read-loop add-proof was consumed more than once in one run; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `context_prune` = tool history pruning occurred. Lossless deterministic carrier/render repairs remain visible in per-case metrics/advisories but do not create a top-level flag by themselves.
