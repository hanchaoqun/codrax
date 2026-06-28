# T11 Explorer Convergence Audit

- date: 2026-06-28T17:21:37Z
- sweep_start_ts: 20260629-011750
- cases: 1
- runs_per_case: 1
- parallel: 1
- results_root: eval/results

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | refine | proof | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------:|------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 15 | 4 | 3 | 1 | 2 | 20 | 1 | 7 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |

Manual audit note:
- Metric PASS is not a functional pass for this run. The final answer broadened the exact requested `public class` universe into public class/interface/struct/enum rows.
- The run did verify that analyzer same-batch stale prescan tools are now skipped as successful non-evidence instead of becoming false failed-search debt.
- Remaining correctness gap: structured source-inventory carrier roles can be dropped or broadened when model output uses construct-family labels. This is tracked as D1-F10g.234k/D1-F10g.234l in `docs/design/ir_execution_engine_stage_direction_plan_20260621.md`.

**flagged: 0 / 1**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `repair_churn` = deterministic repair coincided with finalizer retry/reject/rewrite, suggesting repair instability rather than harmless carrier normalization; `adaptive_loop` = AnalyzeRefine or read-loop add-proof was consumed more than once in one run; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `context_prune` = tool history pruning occurred. Lossless deterministic carrier/render repairs remain visible in per-case metrics/advisories but do not create a top-level flag by themselves.
