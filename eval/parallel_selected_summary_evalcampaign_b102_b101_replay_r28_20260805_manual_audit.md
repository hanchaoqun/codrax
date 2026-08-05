# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T10:27:34Z
- sweep_start_ts: 20260805-032732
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260805-032734 | answer_regex,answer_contains | none | 227s | 22 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=4,inv=4/0,fin_reject=2,unavail=1,prune=0 | fail | The typed production projection and all 38 row-local citations are correct, but the answer duplicates the full roster as both Markdown table and ordered list, omits an explicit visible `function=5` category count, and invents an `iota` mechanism absent from source. The `iota` wording first appears in Explorer completion, not Analyzer hints; B101 did not reproduce it. |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260805-032734 | answer_regex,answer_contains | none | 272s | 29 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=3,unavail=0,prune=0 | fail | B101 handoff fix is active: Finalizer sees `requested_sink_existence_proof=definition_only`. The model correctly states `gate.Run -> RunWith` in its lead, then contradicts itself in the final caveat by claiming the reverse; diagram validation removes the unsupported arrow but does not rewrite model prose. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## B101 production close

`EVAL-B101-BOUNDARYHANDOFF1` is closed. The Finalizer prompt now carries the Explorer handoff definition as `requested_sink_existence_proof=definition_only`; the previous `unproven` regression is absent. The remaining sequence error is downstream model inconsistency, not evidence loss across the stage boundary.

## `EVAL-B102-GRAPHSTATUS1` (P1 context precision)

The same capsule exposed `evidence_status=endpoint_unresolved` beside `requested_sink_existence_proof=definition_only`. These are not logically contradictory—the former describes membership/resolution in the grounded call-edge graph, while the latter proves exact symbol existence—but the generic name obscures that axis split. The rendered key is now `call_graph_status`, with an explicit typed note that graph resolution and endpoint existence are separate. No enum, edge, conclusion, or gate changes.

The model's final reverse-direction sentence is not converted into a prose hard gate: the prompt already supplied the correct boundary and the model's own lead was correct. This single-run, internally inconsistent output is retained as model fluctuation for heterogeneous replay. The system continues to reject unsupported diagram edges while leaving prose conclusions model-owned.

## `EVAL-B102-PATCHTARGET1` (P1 repair convergence)

The inventory's second draft added an authored Markdown table containing all 38 visible identities but no structured row/citation sidecars. The coverage gate correctly rejected it. The next generic patch hint said “ADD missing rows” without naming the existing table, so the model used `add_blocks` to create a second complete ordered list; the rejected table remained in the patch base and both copies shipped.

The source-inventory repair context now uses the same precise table classifier and visible first-column identity matcher as the gate. When an existing principal Markdown table already renders missing obligation identities, the hint names its exact block ID and visible-row count, directs `replace_blocks` to attach matching `items[]` citation sidecars in place, and explicitly forbids `add_blocks` from duplicating that roster. It remains model-executed repair guidance; the system does not mutate or delete model blocks.

## Deferred observations

- The false `iota` mechanism originated in Explorer/Finalizer model prose after exact source-inventory rows were available, and did not reproduce in B101. It is not repaired by scanning request or answer text; retain for heterogeneous replay before considering any typed source-mechanism carrier.
- The missing explicit `function=5` category header occurred despite the model correctly counting five functions in thought and rendering all five rows. The previous replay emitted the count correctly. Treat as model variance unless another inventory family shows a recurring need for a compact typed per-role census.
- B99 patch-citation drift still lacks the exact stable-ID insertion/reference-displacement production shape and remains replay-pending.

Verification: targeted agent/tool tests passed; full `internal/agent` (2.480s) and `internal/tool` (161.239s) passed. Both implemented changes are isolated from RootCauseTrace, explicit-window causal projection, double-axis root-cause analysis, and automatic supplementation.
