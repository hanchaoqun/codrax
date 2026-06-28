# D2 Representative Eval Audit After D1-F10g.191

- date: 2026-06-28
- baseline: `6de9e3cc6` (`fix: type source inventory path miss advice`)
- batch size: 6 cases, 2-way parallel
- result: 5 / 6 PASS

| case | result | result dir | key metrics / notes |
|---|---:|---|---|
| `cangjie_repomap` | FAIL | `eval/results/cangjie_repomap-20260628-042220` | Missing `package:demo.greeter`; `wall_seconds=258`, `midloop_inject=5`, `transient_retry_checkpoints=3`, `pipeline_dispatches=9`. Manual audit shows `read_file` saw `package demo.greeter`, but finalizer handoff rendered the `Greeter` row as `attributes=1` instead of carrying `package:demo.greeter`. |
| `arkts_repomap` | PASS | `eval/results/arkts_repomap-20260628-042220` | Correct but slow: `wall_seconds=267`, `transient_retry_checkpoints=3`. |
| `qf_relation_subagent_registry` | PASS | `eval/results/qf_relation_subagent_registry-20260628-101031` | Clean: `wall_seconds=102`, `repo_map=2`, `read_file=5`, no finalizer reject. |
| `read_combo_log_current_source_explanation` | PASS | `eval/results/read_combo_log_current_source_explanation-20260628-101022` | Correct but slow: `wall_seconds=205`, `read_file=8`, `repo_map=2`. |
| `read_combo_trace_current_source_explanation` | PASS | `eval/results/read_combo_trace_current_source_explanation-20260628-101406` | Correct but costly: `wall_seconds=246`, `midloop_inject=7`, `max_context_tokens_est=70237`, `finalizer_rejects=2`, `answer_contract_advisories=1`. |
| `trace_query_openharmony_bytrace_thread` | PASS | `eval/results/trace_query_openharmony_bytrace_thread-20260628-101406` | Correct and uses trace_query first: `trace_query=3`, `repo_map=0`, `wall_seconds=128`; one unavailable tool attempt remains advisory debt. |

## Architecture Gaps

1. **P0: Source-inventory row-local attributes are not load-bearing in finalizer handoff.**
   - Class: attribute-bearing source inventories across all languages, such as member -> package/module/owner/handler/default/entrypoint.
   - Evidence: `cangjie_repomap` finalizer prompt had `member=Greeter ... attributes=1` while the read file and typed observation carried `package demo.greeter`.
   - Required fix: finalizer handoff must render typed row-local attribute role/name/location values, bounded by row, instead of only an attribute count. This is a projection/handoff fix, not a new hard gate.

2. **P1: Runtime/current-source mixed lane still over-packs context and churns finalizer advisory repair.**
   - Evidence: `read_combo_trace_current_source_explanation` passed but had `max_context_tokens_est=70237`, `midloop_inject=7`, and `finalizer_rejects=2`.
   - Required follow-up: inspect typed answer-contract advisory and current-source forced-read packing before the next broad eval.

3. **P1: Source-inventory cases still pay transient checkpoint cost under provider EOF.**
   - Evidence: ArkTS and Cangjie both had `transient_retry_checkpoints=3`; Cangjie continued after EOF with incomplete principal attribute projection.
   - Required follow-up: after P0 attribute handoff, audit whether transient failure with incomplete principal inventory should trigger bounded local landing/retry before finalization.

## Current Batch

- D1-F10g.192 starts from this audit and targets P0 only: source-inventory finalizer handoff now renders row-local attribute values from typed observation rows.
- Red-line boundary: the fix consumes only typed `SourceInventoryObservationMember.Attributes` fields and source locations. It does not parse user keywords, model rationale/prose, tool summaries, rendered repo_map markdown, localized UI text, elapsed time, final answer prose, or eval labels.

## Focused Follow-Up: cangjie_repomap After D1-F10g.192

- result dir: `eval/results/cangjie_repomap-20260628-102428`
- result: FAIL, `missing_dimension:package:demo.ffi`
- important movement: previous `demo.greeter` row-local package loss was addressed, but the run exposed a more general P0 identity gap.
- root class: the final `emit_investigation_complete` payload correctly contained two `native_add` principal members with distinct source locations and packages (`demo.bridge`, `demo.ffi`). Downstream aggregate/row projection collapsed same-name members by bare symbol, leaving one visible `native_add` row and one package.
- follow-up fix: D1-F10g.193 makes structured source location part of principal member identity during aggregate member-set merge and disambiguates duplicate principal row IDs by location. This is language-agnostic and applies to same-named functions, methods, classes/types, routes, config keys, native/foreign declarations, imports, entry points, and operation sites.

## Focused Follow-Up: cangjie_repomap After D1-F10g.193

- result dir: `eval/results/cangjie_repomap-20260628-103820`
- result: FAIL, `missing_dimension:package:demo.greeter`
- important movement: duplicate `native_add` identity stayed fixed, but the run exposed an upstream completion-boundary gap rather than a row-merge bug.
- root class: typed source-inventory observation already contained `Greeter` as a Cangjie `type` row with `package:demo.greeter`. The accepted aggregate facts covered one Java `public class` row and other Cangjie construct rows; requested-universe closure accepted the answer from source-class/language census coverage and did not account for the observed same surface-family row omitted from the principal member_set.
- follow-up fix: D1-F10g.194 makes observed duplicate-location / surface-family gaps veto requested-universe closure and lets surface-family selection come from exact structured row-location coverage, not from model member text repeating the surface phrase. This is language-agnostic and applies to public/exported types, decorators, routes, imports, package/module declarations, foreign/native declarations, extension blocks, same-name operation sites, and other typed source-inventory surface families.

## Focused Follow-Up: cangjie_repomap After D1-F10g.194

- result dir: `eval/results/cangjie_repomap-20260628-105140`
- result: FAIL, `missing_dimension:package:demo.ffi`
- important movement: the upstream completion payload was correct and contained both `native_add` declarations with distinct support refs; the residual failure moved to final answer rendering.
- root class: finalizer normalization already computed missing typed rows from accepted `aggregate_facts.member_set`, but suppressed deterministic supplement whenever a model-authored list/table existed for that set. This allowed a visible answer to claim one `foreign func` while typed principal facts carried two.
- follow-up fix: D1-F10g.195 changes principal enumeration normalization to append only missing typed rows even when a model-authored carrier exists, preserving model text for audit while preventing exact member-set shrinkage. This is language-agnostic and applies to same-name declarations, route/config/entrypoint rows, cross-column tables, markdown tables, and any source-inventory/exhaustive member-set answer backed by precise support refs.

## Focused Follow-Up: cangjie_repomap After D1-F10g.199

- failed control dir: `eval/results/cangjie_repomap-20260628-115407`
- recovered dir: `eval/results/cangjie_repomap-20260628-120436`
- recovered result: PASS
- root class: source-inventory source-class coverage was not role-scoped. File-row observations could satisfy a requested `type` / `function` principal inventory obligation even when thirdparty/corpus principal rows were still uncovered. That let completion close before discovering `demo.stringext`, `demo.ffi`, and `demo.greeter`.
- landing fix: source-class coverage now intersects observed coverage with the requested principal roles. File rows cannot cover member-role obligations. The analyzer/schema layer also preserves `package`, `module`, and `namespace` as valid display fields, so cross-language inventory dimensions do not get dropped as form noise.
- finalizer follow-up: duplicate-label citation repair now prefers exact row-local attribute matches before label-only fallback. This prevents same-name rows such as two `native_add` declarations from borrowing the wrong package/source citation when typed attributes already disambiguate them.
- remaining commercial debt: the PASS still costs `wall_seconds=212`, `read_file=14`, `repo_map=4`, `source_inventory_lens=4`, `explorer_iters=13`, and `midloop_inject=5`. Correctness is recovered, but source-inventory convergence cost remains open and should be addressed before the next broad representative batch.
