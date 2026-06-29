# Selected Eval Audit After Source-Inventory Snapshot Cutover

- Date: 2026-06-29
- Runner summary: `eval/parallel_selected_summary.md`
- Batch shape: 6 representative cases, 2-way parallel, 1200s timeout
- Machine verdict: 6/6 PASS

## Cases

| Case | Machine verdict | Manual audit |
| --- | --- | --- |
| `arkts_repomap` | PASS | Correctly preserved ArkTS `@Entry` / `@Builder` thirdparty corpus rows. Still shows a soft "verify not stable enough" status despite no completion reject; track as UX/status noise, not correctness. |
| `cangjie_repomap` | PASS | Not fully correct. The final answer grouped `runOnMainThread` as a `foreign func` even though the row is a normal public function in the same file as a real foreign function. The answer also stated `public class` count 9 while visibly listing 8 rows. |
| `qf_relation_subagent_registry` | PASS | Correct principal answer, but deterministic final supplement duplicated the already-covered row and added a generic weak-evidence caveat. |
| `read_combo_trace_current_source_explanation` | PASS | Correctly combined runtime observation with current-source explanation. `trace_query=0` is acceptable for a small pre-stage parsed trace, but the status/audit path should say which typed runtime authority was used. |
| `read_combo_log_current_source_explanation` | PASS | Correct answer, but still costly: 10 explorer iterations, 8 reads, and repeated mid-loop injects for a bounded runtime-source mechanism question. |
| `sr_ts_workspace_impls` | PASS | Correct relation answer. First completion used an invalid repo-grounded negative-observation lane and self-repaired; final answer duplicated deterministic supplement rows. |

## Architecture Finding

The recent source-inventory fixes made real progress, but the history still shows a ping-pong pattern: follow-up universe, cursor ownership, support refs, principal rows, citation floors, required-files, analyzer grep noise, and finalizer materialization were each patched from different local views of the same question: "what inventory universe is answer-critical?"

`types.SourceInventoryAuthoritySnapshot` is the right consolidation direction. It already carries completion authority, follow-up debt, projected principal aggregate facts, requested fields, required-file coverage, and mechanical landing readiness. However, only the explorer mechanical landing and required-file verification surfaces currently consume it. Completion gates, citation/support gates, finalizer materialization, and eval row/category oracles still have local helper logic that can re-open the ping-pong.

## New Gaps

- D1-G155: row surface-family leakage. Construct family must be row-local and typed; file-level or adjacent row surface terms cannot classify another row.
- D1-G156: deterministic inventory count/list consistency. Visible category counts and visible rows must come from one typed row-set or carry a caveat.
- D1-G157: deterministic supplement duplication and false caveats. Supplements should append missing answer-critical rows only, not duplicate complete principal answers.
- D1-G158: runtime artifact-value lane. Trace/log derived exact values need typed artifact support instead of request-prose quote contracts.
- D1-G159: eval PASS can hide manual correctness gaps. Add typed row/category oracles and preserve manual audit summaries beside machine summaries.

## Immediate Priority

Continue D1-G153 snapshot cutover before running the next eval batch:

1. Move `emit_investigation_complete` source-inventory completion authority through the snapshot view.
2. Move source-inventory landing facts through the snapshot view.
3. Keep exact universe/duplicate/surface-family gaps as separate precise row-set checks until they are also represented in the snapshot.
4. Add focused tests for adjacent mixed construct rows and count/list consistency before the next six-case eval.
