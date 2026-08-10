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
