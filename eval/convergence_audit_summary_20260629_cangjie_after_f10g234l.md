# T11 Explorer Convergence Audit

- date: 2026-06-28T17:30:21Z
- sweep_start_ts: 20260629-012554
- cases: 1
- runs_per_case: 1
- parallel: 1
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | refine | proof | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------:|------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 12 | 2 | 0 | 1 | 5 | 10 | 1 | 7 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |

Manual audit note:
- Metric PASS is not a functional pass for this run. The answer still broadened exact `public class` into a 13-row public type universe, so this run must not be counted as solved.
- D1-F10g.234k reduced noisy exploration cost (`repo_map 4->2`, `list_files 3->0`, `explorer_iters 20->10` compared with the previous focused run), proving the structured alias normalization helped convergence.
- Remaining P0: when analyzer `source_inventory` prescan times out and the analyzer retries through a broad fallback path, exact source-inventory surface-family authority is not preserved strongly enough. This is tracked as D1-F10g.234l.

**flagged: 0 / 1**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `repair_churn` = deterministic repair coincided with finalizer retry/reject/rewrite, suggesting repair instability rather than harmless carrier normalization; `adaptive_loop` = AnalyzeRefine or read-loop add-proof was consumed more than once in one run; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `context_prune` = tool history pruning occurred. Lossless deterministic carrier/render repairs remain visible in per-case metrics/advisories but do not create a top-level flag by themselves.
