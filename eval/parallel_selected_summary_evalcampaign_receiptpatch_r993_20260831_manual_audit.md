# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T23:52:13Z
- sweep_start_ts: 20260831-165212
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-165213 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 42 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 10ms window, three typed queries, target four-state account, on-chain ranking, raw occupancy versus rule-eliminable accounts, business clues, auto-supplement and Trace causal projection all survived. The model-owned runtime-work receipt also surfaced VerifyClass 0.285ms as relation unproven with 0.000ms attributable impact. However the same model answer says all three on-chain threads completed deterministic work before waking the target and calls that the fundamental cause. Only T7 has a typed semantic span and even that completion-to-wait relation is unproven, so the visible model answer contradicts its own receipt and the accurate system projection. Treat as semantic synthesis failure, not evidence/projection loss; no prose hard gate authorized. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260831-165213 | primary_answer | none | 381s | 60 | read=10,repo_map=1,list=0,trace=0,source_lens=1 | midloop=15,inv=4/0,fin_reject=11,unavail=0,prune=0 | fail | B1516 kept body enrichment out of principal topology and the final answer exposed the grounded source chain, guard and stdout call. B1518 published the new atomic receipt operation and correctly rejected an invented/non-published pair transactionally. Production nevertheless exposed two contract gaps: the final evidence capsule presented exact static-call row ev-f632b19f56f19361 for AuditLog.record to System.out.println, while the closed receipt candidates excluded that row because parser enrichment ran before the deeper edge arrived; the model therefore could not preserve its correct current_terminal_differs conclusion. Separately, three retries placed from_node at the edit top level after the strict-decode hint claimed the only valid path was blocks edge_anchors, even though the live patch schema correctly required diagram_edge_edits[].edge.from_node. Final prose still calls stdout printing audit persistence, and the optional Mermaid omits two visible calls despite structured relation carriers. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
