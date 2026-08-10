# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T21:00:39Z
- sweep_start_ts: 20260810-140038
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-140039 | answer_regex | none | 187s | 22 | read=0,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B475 typed advisory was present and correctly marked native/slow definitions unproven, but the structured list cited native at the slow callsite and slow at the native callsite. Registration was re-read at add_function, yet no exact registered-export handoff or wrapper→core call entered the final graph. Runner regex is a false green. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260810-140039 | answer_regex,answer_contains,mermaid_edge_count | none | 864s | 39 | read=29,repo_map=3,list=0,trace=0,source_lens=0 | midloop=23,inv=8/1,fin_reject=9,unavail=0,prune=0 | fail | B473 has a direct production witness: finalizer zero-edge violation re-opened Explore and the typed repair was not swallowed. B476 then targeted topology/stage-binding/Run/context files, but the finalizer prompt's authoritative canonical stage sequence was not accepted by the diagram evidence gate. Nine finalizer rejects ended in a zero-edge diagram plus unrelated responsibility citations. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- `B473-CLOSUREDROP1` closes with a direct production witness: after finalizer emitted a required diagram with zero edges, `required_diagram_edge_absent` selected owner=Explore, requeued the read DAG, and dispatched new Explore work. The old accepted closure did not clear the typed violation.
- `B476-OPSUPPLY1` is production-positive but not sufficient: the repair sent the model to `topology.go`, `stage_binding.go`, `orchestrator.go`, and `context.go`, and the second evidence pass found the canonical stage roster, responsibilities, `Run` phase description, BusContext initialization, and MutableState. It still emitted no usable exact participant transfer tuple and converged with all six participant relations unproven.
- `B477-STAGEAUTHDUAL1` (P0): the finalizer is taught `canonical_read_main_sequence=analyze -> explore -> extract -> finalize` from a checkout-verified typed stage-lane authority, while the hard diagram relation gate accepts only EvidenceItem-backed precedence. The same exact authority is therefore usable in prose but unusable in the required diagram. This is a prompt/validator source split, not a model-only failure. Root fix: expose one typed relation-provider representation consumed by both teaching and validation; do not synthesize answer prose or scan model text.
- `B478-ITEMCITEID1` (P1): QFCallChain ordered-list items carry structured `label` and `citation_ref`, but validation does not require the cited row to contain the label identity. Exact label/citation swaps therefore pass even when B475 says definition unproven. Root fix: structured code-identity label ↔ cited typed evidence/source-surface alignment for call-chain items, language-neutral and fail-closed; do not inspect item free-form text.
- `B474` remains pending: this replay emitted a weaker `add_function -> tokenize_bytes` registration shape and omitted the wrapper definition/call row, so the exact owner/reference join did not fire. Keep the binding as Note-only until a precise endpoint join exists.
- `B475` is production-consumed but not closed: the matrix appeared verbatim and correctly withheld cross-file definition authority, proving wiring; the model ignored it. A precise structured-item validation arm is required rather than more prose teaching.
