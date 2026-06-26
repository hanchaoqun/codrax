# D2-F10g71 representative post-gap eval

- date: 2026-06-26T02:16:47Z
- baseline: 828e01fe9
- parallel: 2
- timeout_seconds: 1200
- results_root: eval/results
- cases: qf_relation_subagent_registry, arkts_repomap, cangjie_repomap, trace_query_openharmony_bytrace_thread, read_combo_log_current_source_explanation, patch_go_typo

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------------|
| 2 | arkts_repomap | PASS | - | 135s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/arkts_repomap-20260626-101651 |
| 1 | qf_relation_subagent_registry | PASS | - | 142s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/qf_relation_subagent_registry-20260626-101651 |
| 4 | trace_query_openharmony_bytrace_thread | PASS | - | 147s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/trace_query_openharmony_bytrace_thread-20260626-101912 |
| 3 | cangjie_repomap | FAIL missing_dimension:package:demo.stringext missing_dimension:package:demo.ffi missing_dimension:package:demo.greeter | - | 210s | 1 | 1 | 1 | 1 | 0 | 2 | 2 | 0 | 0 | eval/results/cangjie_repomap-20260626-101906 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 255s | 1 | 1 | 1 | 1 | 0 | 0 | 1 | 0 | 0 | eval/results/read_combo_log_current_source_explanation-20260626-102140 |
| 6 | patch_go_typo | FAIL write_report_failed | - | 247s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | eval/results/patch_go_typo-20260626-102237 |

## Rollup

- pass: 4 / 6
- fail_or_unknown: 2 / 6
- completed_at: 2026-06-26T02:26:41Z

## Manual Audit

- `qf_relation_subagent_registry`, `arkts_repomap`, `trace_query_openharmony_bytrace_thread`, and `read_combo_log_current_source_explanation` reached usable PASS results without reopening the known forced-read historical-path class.
- `cangjie_repomap` failed for a generalized source-inventory authority boundary: a pre-scan/navigation `repo_map(source_inventory)` over implementation files was later projected as `system:source_inventory_principal_row_set`, so finalization required unrelated Go implementation rows while the model already had grounded Cangjie fixture evidence. This is a typed source-family/source-class mismatch gap, not a Cangjie-only fix.
- `patch_go_typo` applied the correct source change, but verification failed because a bounded pre-suite verification probe was not importable as a Go package. The verifier treated this probe-authoring/environment failure as authoritative `tests_failed` instead of continuing to a typed project suite or lowering proof confidence. This is a cross-language verification authority gap, not a Go-only issue.
- Follow-up fixes must target these classes before the next broad eval: source-inventory synthetic principal rows need a typed family-overlap boundary, and pre-suite probe-authoring failures must not hard-fail a patch when an authoritative suite surface is available.
