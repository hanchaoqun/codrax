# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T22:12:10Z
- sweep_start_ts: 20260810-151209
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-151210 | answer_regex | none | 243s | 22 | read=6,repo_map=3,list=1,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | Explorer read the exact Python caller, Rust wrapper, wrapper-to-core call, and registration lines, but the accepted typed call row kept model-authored subject `_fastlex` instead of enclosing caller `FastTokenizer.tokenize`. Finalizer therefore consumed a false directed edge, merged Rust wrapper/core, cited the Rust public function for `_tokenize_slow`, misstated the import-failure owner, and removed the rejected diagram. This is a grounder/owner-routing defect, not a model-only fluctuation. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-151210 | answer_regex,answer_contains,mermaid_edge_count | none | 342s | 34 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | fail | The new shared-provider recipe produced all three exact adjacent `precedence` edges, a direct positive witness for B477b. The six other requested participants remained honestly disconnected/unproven in the diagram, but the prose still asserted Orchestrator-driven stage writes and Mutable/BusContext data flow without exact operation edges. Completion also spent 15 Explore iterations and accepted only after bounded convergence. The requested full logical/data-flow view remains incomplete. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner: `2/2 PASS`.
- Human correctness: `0/2 PASS`.
- B477b is production-positive and can close: the final QF diagram contains exactly the three provider-derived adjacent `precedence` edges.
- B479 remains open: stage authority reduced the previous 70 Explorer iterations to 15, but the completion lane still lacks a typed, bounded way to finish when requested shared-state/data-flow relations remain unproven. It must not mint those relations merely to close.
- New `B480-MRCALLER1/P0`: in a multi-repository run, call grounding can verify the callee syntax on a source line while preserving a false model-authored caller when `ground.Context.Graph` points at another sub-repository's compatibility graph. Hard relation gates then consume a typed-but-wrong directed edge.
- B480 is language-neutral: caller identity must be derived from the graph that owns the cited source file, for every supported language. The remedy is owner-routed graph/file resolution plus caller canonicalization at evidence admission, not Python/PyO3 keywords and not final-answer prose scanning.
- B478/B474 remain pending behind B480. Item/citation validation cannot repair a poisoned directed evidence row, and wrapper/core/register relations must remain independently proven.
- No system-authored answer edge or conclusion is permitted. Trace explicit-window selection, automatic supplement, causal projection, on-chain root-cause election, and non-chain background classification were not exercised or changed by this replay.
