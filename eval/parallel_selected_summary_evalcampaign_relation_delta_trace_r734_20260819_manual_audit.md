# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T15:21:18Z
- sweep_start_ts: 20260819-082117
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260819-082119 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 101s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass-with-wording-residual | False negative: the principal answer says `实际 CPU 运行 157.248 ms`; the oracle only accepts `实际运行/运行占/运行时间`. Exact window, 157.248/5.604/70.338/0.000ms state account, CPU4 2.10GHz policy row, target CPU4 35.960ms row, deterministic supplement and honest overlap-unproven conclusion are all present. This bounded-fact request did not ask for root-cause/wakeup causality, so `final_projection=0` is correct scope routing, not a lost Trace projection. Residual model wording says broad `目标绑定仍未建立` where the typed boundary is narrower temporal slice-policy overlap; record as soft quality residue, not a prose gate. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-082118 | answer_regex,answer_contains,mermaid_edge_count | none | 508s | 40 | read=33,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=6/0,fin_reject=1,unavail=1,prune=0 | partial | B1170 bounded the repeated exact relation obligation and B1169 reduced r733 finalizer rejects 5→1; the local retry hint is 3,295 bytes instead of the repeated ~28KB authority. Final graph retains four-stage precedence and one Explorer→ToolResults local write, but it does not show the requested BusContext/Mutable data flow across analyzer/explorer/extractor/finalizer; Mutable is only an unproven boundary. The prose is useful but the diagram requirement remains incomplete. This is B1167: table-selected stage role + generic dispatch + BusContext→AgentContext reads + StageOutput→BusContext/Mutable writes are separate exact atomic facts with no typed composed value-flow recipe. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-run findings

1. `B1169-DIAGRAMREPAIRDELTA1` has production-positive evidence. The first full rejection remains explanatory; the retry lane carries a 3,295-byte producer-owned participant delta and one bounded candidate per failed participant. The model authored the replacement block and all visible labels/edges. No system diagram authorship was observed.
2. `B1170-REQUIREDRELATIONITEMREPAIRCONVERGENCE1` also fired: the same exact relation obligation no longer locked completion forever. QF still needed 39 Explorer iterations, 6 completion calls, two Explorer dispatches and 508s, so bounded convergence is closed while investigation cost remains material.
3. `B1167-DIAGRAMCROSSCOMPONENTDATAFLOW1` is production-reconfirmed. The general solution must compose exact atomic typed value-flow facts across role selection, argument handoff, field copy/assignment, return and consumer/write steps, then offer that recipe to the model. It must not infer from prose/node names, invent a bridge, or author the Mermaid edge.
4. H4 protects an important scope distinction: finite observed-state/frequency questions should not be forced into the root-cause/causal-projection contract. Explicit-window Trace queries and deterministic supplement remain mandatory; causal projection remains available for causal/root-cause scope.
