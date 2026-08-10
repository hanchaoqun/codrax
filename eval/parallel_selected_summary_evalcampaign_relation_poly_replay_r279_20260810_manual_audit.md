# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T22:38:06Z
- sweep_start_ts: 20260810-153804
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-153806 | answer_regex | none | 121s | 21 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B480 has a direct production witness: emit_evidence normalized the three call rows to `FastTokenizer.tokenize -> _fastlex.tokenize_bytes`, `FastTokenizer.tokenize -> self._tokenize_slow`, and `py.tokenize_bytes -> tokenize_bytes`. The answer no longer merges caller identities, but it still says `_HAVE_NATIVE` is always true and then inconsistently describes import failure; Explorer emitted no evidence for the `except ImportError` / false-state producer. The Rust core item also cites registration line 47 instead of definition line 10. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-153806 | answer_regex,answer_contains,mermaid_edge_count | none | 712s | 47 | read=8,repo_map=5,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=12,unavail=0,prune=0 | fail | The final answer is readable but needed 12 deterministic diagram repairs. The model repeatedly oscillated between missing boundaries and boundary participants not visible, because the typed contract did not publish a per-participant visible-node recipe. The accepted diagram contains helper call/data-flow edges but drops the three verified stage precedence edges, so the requested relationship view remains incomplete. Unsupported stage/Mutable edges were correctly rejected and no system edge was minted. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner: `2/2 PASS`; human correctness: `0/2 PASS`.
- `B480-MRCALLER1` is production-closed. The owner-routed caller correction is visible in the production log before grounding and finalization.
- New `B481-PARTBOUNDRECIPE1/P0`: the finalizer receives exact typed participant obligations and a hard visible-node/boundary contract, but not a machine-isomorphic recipe for each uncovered participant. The model needed 12 retries to discover that `participant=analyzer` requires a visible disconnected node whose ID or first-line label resolves to `analyzer`. Publish per-participant `identity / visible_node_label / status=unproven / edge_forbidden` recipes from the same typed slate; do not loosen the validator and do not synthesize nodes.
- `B479-STAGECLOSEBOUND1` remains open. The verified stage recipes are present, but this sample chose a different evidence-backed subset after repeated repair. Treat this as a soft-consumption/completion issue, not permission to require system-authored stage edges.
- New `B482-FALLBACKPROOF1/P1`: a requested fallback mechanism needs evidence for both the state/exception producer and the downstream guard/sink. Here the typed set covered import success, guard, native call, and slow call but omitted `except ImportError` plus `_HAVE_NATIVE=False`; final prose therefore contradicted the source. Generalize by fallback-branch proof roles, not Python keywords.
- B478/B475 remain open for structured item/citation identity: Rust core definition line 10 was available but the final core item cited registration line 47.
- No raw request/model-answer prose hard gate and no system-authored conclusion/edge were added. Trace explicit-window selection, automatic supplement, causal projection, on-chain root-cause election, and non-chain background handling remain unchanged.
