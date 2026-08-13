# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T21:48:04Z
- sweep_start_ts: 20260813-144803
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-144804 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 207s | 49 | read=5,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Analyzer accepted the passive binding question as `bounded_fact_set` and gave its third required dimension only `evidence_source`; the report-shape authority therefore suppressed the full Trace causal projection despite an explicit window and typed root-cause rows. The prose preserves the policy-ceiling/target-binding distinction, but also states an unsupported `~59%` relation for 58.320ms versus 157.248ms and overstates cpu=4 as the target's principal running CPU. This is typed classification drift, not a projection compiler deletion. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-144804 | answer_regex,answer_contains,mermaid_edge_count | none | 411s | 39 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | The final Mermaid is valid and explains stage precedence, but it does not prove the requested data handoff between the four agents and Mutable/BusContext. B744's `argument_flow` teaching reached Explorer, yet no exact `argument` EvidenceItem was emitted for an already-readable site such as `BuildAgentContext(o.busCtx, ...)`; three finalizer rejects then removed unsupported carrier edges. The source/validator vocabulary is implemented, while operation-site discovery and selection remain open. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
