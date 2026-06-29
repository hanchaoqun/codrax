# T11 Explorer Convergence Audit

- date: 2026-06-29T01:33:31Z
- sweep_start_ts: 20260629-093149
- cases: 1
- runs_per_case: 1
- parallel: 1
- results_root: eval/results/cangjie_after_f10g253_20260629

This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence.

| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | refine | proof | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |
|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------:|------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|
| cangjie_repomap | PASS | — | 0 | 0 | 0 | 0 | 3 | 0 | 3 | 4 | 4 | 1 | 2 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | — |

**flagged: 0 / 1**

Flag meanings: `finalizer` = finalizer took multiple turns or had document/patch rejects/rewrite renders; `repair_churn` = deterministic repair coincided with finalizer retry/reject/rewrite, suggesting repair instability rather than harmless carrier normalization; `adaptive_loop` = AnalyzeRefine or read-loop add-proof was consumed more than once in one run; `explorer_long` = multiple explorer dispatches or very high explorer iterations; `wide_search` = high read_file/repo_map/list_files cost; `lane_wait` = typed mixed-origin closure correctly waited for missing lanes; `semantic` = semantic reviewer emitted concerns; `contract_warning` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; `context_prune` = tool history pruning occurred. Lossless deterministic carrier/render repairs remain visible in per-case metrics/advisories but do not create a top-level flag by themselves.

## Manual Audit

- Correctness: PASS. The final answer lists 2 extend blocks, 2 foreign func declarations, and 8 public class declarations across both `eval/fixtures/testdata/cangjie_minimal` and repo-owned `internal/thirdparty/tree-sitter-cangjie/corpus/sources`, including the previously missed package dimensions `demo.stringext`, `demo.ffi`, and `demo.greeter`.
- Convergence: improved. The run used `repo_map=3`, `source_inventory_lens=3`, `read_file=0`, `list_files=0`, `unavailable_tool_attempts=0`, `investigation_complete_calls=1`, and `finalizer_rejects=0`.
- Residual architecture note: analyzer still spent three classification pre-scan rounds on `grep(files_only=true)` before emitting `source_inventory_profile`. This is not a correctness failure, but it indicates prompt-level first-hop noise. It is tracked and addressed by D1-F10g.253 in `docs/design/ir_execution_engine_stage_direction_plan_20260621.md`.
