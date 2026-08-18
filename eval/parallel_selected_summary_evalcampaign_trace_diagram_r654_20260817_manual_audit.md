# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T00:53:52Z
- sweep_start_ts: 20260817-175351
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-175353 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Exact requested-window supplementation remains intact: the final causal projection crowns only the typed on-chain worker-200 seat at 8.300ms effective attribution, keeps the target's 10.000ms sleep as a symptom, and leaves supply pressure in the background lane. No fixed-4ms fallback, missing answer, or system-authored conclusion occurred. Customer presentation remains partial because the evidence/audit appendix exposes raw internal tokens such as `tier=primary`, `causality=on_wakeup_chain`, `priority_inversion_candidate`, and `chain_relevance=background`; retain as B756 and fix through typed customer-facing display metadata, never prose scanning or rewriting. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-175353 | answer_regex,answer_contains,mermaid_edge_count | none | 397s | 41 | read=10,repo_map=1,list=1,trace=0,source_lens=0 | midloop=10,inv=6/0,fin_reject=3,unavail=0,prune=0 | partial | B1031 is production-positive: after reading `BuildAgentContext(o.busCtx, types.AgentExtractor, ...)`, the model emitted the exact caller/callee row and both requested argument-flow rows; the final diagram can therefore render `BusContext -> BuildAgentContext` without a fabricated edge. The remaining `Mutable` gap is deterministic B1032: although `BuildAgentContext`'s already-read body contains `bus.Mutable.Objective()`, participant repair restarted from unrelated local `MutableState.EmittedAnswerSymbols` instead of continuing through the uniquely resolved receiving callable. Three final rejects correctly blocked invented/colliding edges but raised runtime to 397s and left Mutable explicitly unproven. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized disposition

- `B1031-CROSSFILECARRIERARGUMENTFLOW1`: production-positive in r654. Cross-file field identity, imported package-call identity, and checkout-verified stage arguments all reached model-authored grounded evidence; retain the ambiguity-fail-closed negatives.
- `B1032-CALLEEBODYRELATIONFRONTIER1`: confirmed P1. Once a grounded exact handoff reaches one uniquely resolved callable whose body is already in the read closure, participant repair must prefer an exact parser-owned body operation incident to the still-missing participant over an unrelated local occurrence. This is navigation/repair debt only: the system must not emit the call or draw the edge.
- `B756-RUNTIMEENUMCUSTOMERLANGUAGE1`: remains open and visible in the Trace answer. Preserve raw enums in audit storage while giving the model/customer a localized typed display surface; do not hard-scan user or answer prose.
- Trace explicit-window causal projection and deterministic supplementation stayed protected. The active run did not use a fixed-time fallback and produced a complete answer.
