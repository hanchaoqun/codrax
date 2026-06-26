# D2-F10g80 Representative Eval Audit

Date: 2026-06-26T06:16:32Z
Scope: representative read/write convergence batch after D1-F10g.78-.80, 2-way parallel, 6 cases.
Baseline: a9902c7b87
Sweep start: 20260626-141632

| # | Case | Verdict | Reason | Wall | Result Dir |
|---:|---|---|---|---:|---|
| 1 | `qf_relation_subagent_registry` | PASS | - | 97s | `eval/results/qf_relation_subagent_registry-20260626-141636` |
| 2 | `arkts_repomap` | PASS | - | 186s | `eval/results/arkts_repomap-20260626-141636` |
| 3 | `cangjie_repomap` | FAIL missing_dimension:package:demo.stringext missing_dimension:package:demo.ffi missing_dimension:package:demo.greeter | - | 148s | `eval/results/cangjie_repomap-20260626-141813` |
| 4 | `trace_query_openharmony_bytrace_thread` | PASS | - | 240s | `eval/results/trace_query_openharmony_bytrace_thread-20260626-141942` |
| 5 | `read_combo_log_current_source_explanation` | PASS | - | 203s | `eval/results/read_combo_log_current_source_explanation-20260626-142042` |
| 6 | `patch_go_typo` | PASS | - | 118s | `eval/results/patch_go_typo-20260626-142343` |

## Raw Metrics Pointers
- `qf_relation_subagent_registry`: `eval/results/qf_relation_subagent_registry-20260626-141636/run-1.metrics.txt`, `eval/results/qf_relation_subagent_registry-20260626-141636/summary.md`
- `arkts_repomap`: `eval/results/arkts_repomap-20260626-141636/run-1.metrics.txt`, `eval/results/arkts_repomap-20260626-141636/summary.md`
- `cangjie_repomap`: `eval/results/cangjie_repomap-20260626-141813/run-1.metrics.txt`, `eval/results/cangjie_repomap-20260626-141813/summary.md`
- `trace_query_openharmony_bytrace_thread`: `eval/results/trace_query_openharmony_bytrace_thread-20260626-141942/run-1.metrics.txt`, `eval/results/trace_query_openharmony_bytrace_thread-20260626-141942/summary.md`
- `read_combo_log_current_source_explanation`: `eval/results/read_combo_log_current_source_explanation-20260626-142042/run-1.metrics.txt`, `eval/results/read_combo_log_current_source_explanation-20260626-142042/summary.md`
- `patch_go_typo`: `eval/results/patch_go_typo-20260626-142343/run-1.metrics.txt`, `eval/results/patch_go_typo-20260626-142343/summary.md`

## Manual Audit

- `cangjie_repomap` exposed D1-F10g.81: the first source-inventory lens was broad/root and budget-truncated, but the accepted completion answered only the visible fixture `.cj` subset. The typed observation carried a same-language thirdparty Cangjie source-class sample scope, so this is not a Cangjie-specific oracle patch; it is a general explicit-inventory closure boundary. A model-owned fixture member_set must not close a repo-wide source inventory while a precise same-language source-class follow-up remains executable.
- Non-precise source-inventory debt remains caveat-only: pure pagination/root-width debt without sample scopes is not allowed to become a hard gate, because that would recreate the completed-investigation loop. The fix must distinguish typed `missing_source_class_family` follow-up from generic navigation truncation.
- Other cases were acceptable for this batch. `read_combo_log_current_source_explanation` improved after D1-F10g.79/.80 (`read_file=2`, `repo_map=1`) but still has high context and one answer-contract warning; keep it in the next representative batch after D1-F10g.81.

## Follow-up Validation

- D1-F10g.81 focused rerun passed: `eval/convergence_audit_summary_20260626_d2_f10g81_cangjie.md`, result dir `eval/results/cangjie_repomap-20260626-144706`, verdict PASS. The final answer includes the previously missing same-language thirdparty Cangjie packages `demo.stringext`, `demo.ffi`, and `demo.greeter`.
- Residual system gaps remain and are tracked in `docs/design/ir_execution_engine_stage_direction_plan_20260621.md` as D1-G127/D1-G128: the pass still used `repo_map=5`, `list_files=8`, `exp_it=23`, `midloop=8`, and `answer_contract_violations=3`. Correctness improved, but source-inventory UX/performance is not fully converged.
- After the analyzer auxiliary/exclusion conflict guard, the second focused rerun also passed: `eval/convergence_audit_summary_20260626_d2_f10g81_cangjie_after_aux_conflict.md`, result dir `eval/results/cangjie_repomap-20260626-145827`. Metrics improved to `repo_map=1`, `list_files=1`, `source_lens=1`, `exp_it=8`, `midloop=2`, with no finalizer reject/rewrite. This supports the class-level fix: typed source-scope/exclusion contradictions are rejected before tools silently hide repo-owned auxiliary source classes.
