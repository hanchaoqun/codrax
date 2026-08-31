# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T22:02:39Z
- sweep_start_ts: 20260831-150237
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-150239 | primary_answer | none | 98s | 29 | read=5,repo_map=4,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Typed source and final boundary identify `AuditLog.record -> System.out.println` with no storage/durability authority, but the model again says stdout “完成落库动作”. The only reject was a precise standalone-anchor identity repair; no diagram was authored or deleted, so B1512/B1513 were not production-triggered. Third consecutive semantic miss promotes the conceptual-sink resolution gap beyond one-run model variance. |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-150239 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 213s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/2,fin_reject=0,unavail=0,prune=0 | fail (partial) | B1514 production-positive: after a fact-family conflict, analyzer explicitly preserved `runtime_work_relation_requested=true`; model then selected the typed VerifyClass 0.285ms row and concluded relation_unproven, while explicit window, on-chain ranking, two ledgers, auto-supplement and Trace causal projection remained. However the lead incorrectly calls scheduler-state chains “two deterministic work chains”, contradicting the dedicated work seat; retain as model/soft-context residual, no prose hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
