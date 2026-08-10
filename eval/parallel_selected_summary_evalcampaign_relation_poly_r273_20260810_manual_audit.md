# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T17:52:19Z
- sweep_start_ts: 20260810-105218
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-105219 | answer_regex | none | 149s | 22 | read=2,repo_map=3,list=1,trace=0,source_lens=1 | midloop=4,inv=4/0,fin_reject=0,unavail=0,prune=0 | fail | Python `_fastlex.tokenize_bytes` -> PyO3 `py::tokenize_bytes` -> Rust core `tokenize_bytes` and `_tokenize_slow` fallback were located, but the summary says the native path “绕过 PyO3 FFI”, contradicting both source and its own detailed steps. It also calls `_HAVE_NATIVE=True` “hardcoded” while the complete `try/except ImportError` block owns both values. Runner regex does not cover bridge semantics or cross-block consistency. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-105219 | answer_regex,answer_contains,mermaid_edge_count | none | 450s | 39 | read=25,repo_map=3,list=1,trace=0,source_lens=0 | midloop=15,inv=6/0,fin_reject=3,unavail=0,prune=1 | fail | B469 succeeded: block-level `participant_boundaries` persisted and no schema-unknown quarantine/degraded delivery occurred. Semantic relation completeness still failed: Analyzer/Explorer/Extractor/Finalizer/BusContext remain disconnected and explicitly unproven; three retries reduced the requested data-flow view to low-level assignments plus one scheduler call. Current relation kinds cannot honestly express produced/merged/passed/consumed state flow, while assignment/call endpoint contracts reject or misrepresent those arrows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner: `2/2`; human: `0/2`.
- `B469` is production-closed. It repaired a JSON carrier conflict only; it did not solve relation completeness.
- `B470` is confirmed and upgraded: the missing source-flow relation is an expression/authority gap, not one model fluctuation. A generalized solution needs a typed data/state-flow relation whose direction is checked against exact assignment/initializer/return evidence. It must not weaken call/assignment gates or synthesize edges from participant rosters.
- `B471` is newly filed: assignment/initializer grounding currently corroborates only that an assignment-shaped line exists; it does not verify model-authored `subject/object` against LHS/RHS. Those rows can therefore mint false diagram authority and must fail closed or be grounded to exact endpoints before a new data-flow relation consumes them.
- `B472` is newly filed: cross-language binding answers can preserve endpoint names while conflating export/registration, FFI invocation, wrapper adaptation, and core call. The fix should carry typed bridge roles and branch scope to synthesis; no Python/PyO3 keyword hard gate and no system-authored conclusion rewrite.

## Follow-up implementation status

- `B471` code-complete: simple assignment authority now requires exact grounded LHS/RHS endpoints in emit-time stabilization, flow-operation selection, discover-sink selection, and diagram-edge validation.
- Ambiguous/nested initializer surfaces remain citable local facts but cannot authorize a directed assignment edge without the shared exact matcher; registration keeps its independent typed selection contract.
- The system neither creates a replacement edge nor rewrites the model's conclusion. A mismatched but parseable assignment is retained as `text_reference` with an exact endpoint repair note.
- `B470` remains the next independent batch: introduce an evidence-backed data/state-flow relation after false assignment authority is closed, then replay exact-two rather than fitting this single QF answer.

### B470 follow-up

- `B470` code-complete: `relation_kind=data_flow` now means only one exact assignment/initializer in RHS value/source -> LHS receiver execution direction.
- The binding view remains `assignment` in LHS -> RHS direction, while factory/function return remains `return`; no relation is inferred from diagram labels, participant lists, request text, or answer prose.
- AxisFlow receives source-derived advisory recipes from exact tuples, but the model still chooses whether and how to use them. The system does not add an edge to the answer or replace the model's conclusion.
- Joint B470/B471 production replay is still required before marking the r273 QF gap closed.

### B472 follow-up

- `B472` code-complete: call, registration, guard, assignment, return, and literal support entries now retain distinct typed roles instead of one broad value/binding bucket.
- An exact call-target/registered-export pair produces a soft `call_targets_registered_export` handoff. Owner-qualified registered callable identity can join an independently grounded downstream call; a short callable name remains unresolved and cannot guess a wrapper.
- Exact scalar writes to the same-file guard symbol are carried together as state alternatives/updates. Branch-to-call ownership remains explicitly unproven without a typed carrier; line order is never used as authority.
- Explorer teaching uses one atomic JSON evidence item per call/registration/guard/assignment/return and asks for the complete bounded initialization/fallback block. No request/answer keyword hard gate or system-authored conclusion was added.
- Joint B470/B471/B472 exact-two replay remains required before closing the human failures recorded above.
