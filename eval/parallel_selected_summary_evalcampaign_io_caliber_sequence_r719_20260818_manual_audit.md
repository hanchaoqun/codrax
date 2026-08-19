# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T06:06:37Z
- sweep_start_ts: 20260818-230636
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260818-230637 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 153s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | State and frequency arithmetic remain correct; policy-to-target binding stays unproven. B1143 receives a production-positive witness: Sleep is now called only interruptible sleep and no voluntary-yield/preemption mechanism is asserted. B1142 improves but remains partial: the final labels D/io_wait as uninterruptible/kernel IO wait, yet later says no target “IO wait event” and leaks `target_window_wait_occurrences=0,status=complete`; it never states the absent completion-closed S-state ruler was not assessed. B1139 remains open. This bounded effect question correctly has no full causal projection. |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-230637 | answer_regex,answer_contains | none | 297s | 34 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | B1144 analyzer carrier is production-positive: two independent dimensions are emitted, Mermaid first and key intermediates second. The final preserves the truthful shared-callee topology and the complete compiler/risk/hdp/binder/RunWith roster, but renders that roster before the diagram, so requested order remains unmet. New B1145/P1: the member_set roster was also tagged principal_path_edge/call_edge, causing three deterministic finalizer rejects before it was split from the exact endpoint-edge block. This is a typed facet-ownership collision, not malformed JSON or model-only relation loss. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Conclusions

- Runner status is 2/2 PASS, while human correctness is 2/2 partial. Both answers retain their core facts; neither runner oracle covers the remaining reader semantics/layout contract.
- B1143 is production-closed for this witness: `S` is presented as interruptible sleep only. The system still does not infer a sleep mechanism or classify ordinary S as IO.
- B1142 remains partial and B1139 is reconfirmed. A scheduler-marked zero is visibly narrowed to D/uninterruptible wait, but the absent completion-closed ruler is not disclosed as not assessed and internal typed keys still leak. Continue improving model-facing reader labels; do not add final-answer keyword rejection or system rewriting.
- B1144's analyzer half is production-positive: `requested_answer_dimensions` carries the diagram and post-diagram member roster as separate ordered rows. The final member roster survives, but model block order does not yet honor the typed order.
- B1145/P1 is a generalized contract conflict: one requested member roster is descriptive, while QFCallChain independently requires a principal endpoint-edge carrier. The first draft merged both typed responsibilities into one block, and surgical patches needed three rejects to separate them. Root-fix the initial block-ownership teaching/plan so a member_set roster never inherits principal_path_edge/call_edge unless it is itself the exact endpoint-edge set.
- The final Mermaid is syntactically valid and directionally honest: `buildAnalysisIR -> gate.RunWith <- gate.Run`. It does not invent `buildAnalysisIR -> gate.Run`. Source repair applied once; no degraded answer, active-stream age fallback, system-authored conclusion, or Trace projection regression occurred.
