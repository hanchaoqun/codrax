# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T02:25:59Z
- sweep_start_ts: 20260831-192558
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260831-192559 | primary_answer | none | 159s | 46 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=3/2,fin_reject=4,unavail=0,prune=0 | pass | B1522 production positive: the finalizer no longer loops on `whole_replace_not_authorized`, and the answer identifies the capacity guard at `VisitService.schedule:18` plus the real stdout terminal. Residual process churn remains: one roster/order sequencing rejection, an optional sequence reply rejection, and an attempted second summary for the conceptual terminal. The visible answer also calls six nodes “6 hops”, linearizes sibling config/count calls in one numbered list, and briefly calls stdout an “in-memory write”; the Mermaid and terminal caveat keep the requested conclusion understandable, so these are advisory rather than a human fail. |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-192559 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 44 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The typed 10ms window, six trace queries, chain-only ranking, target-state conservation, semantic `VerifyClass` row, runtime-work relation receipt, automatic supplement, and final Trace causal projection all survived. The model-owned prose nevertheless reverses the wakeup direction for NetworkService, treats `runnable` as CPU occupancy, and titles NetworkService/CookieMonster scheduler rows as deterministic optimization threads. The system projection later states the correct typed facts and unproved semantic binding but cannot replace the model conclusion. This repeats r958/r960 and is B1523, not a one-off runner/oracle miss. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case disposition

- B1521/B1522 are production-positive: exact duplicate receipts and the delegated same-block relation/evidence replacement capability no longer create the prior impossible loop.
- Java remains a correctness pass with P2 presentation/process debt; no case-specific hard gate is justified.
- Trace is a human fail. The finalizer did receive the existing typed scheduler/semantic separation guidance, but the explorer had already emitted a high-confidence model-authored aggregate titled as a semantic-work roster while its members were only scheduler-state rows. The final context therefore contained a noisy synthesized handoff beside the correct typed inventory.
- B1523 repairs the earliest generalized handoff point using only `RuntimeQuestionProfile.RuntimeWorkRelationRequested`: explorer guidance now keeps scheduler and semantic-work inventories distinct, pins wakeup edge direction, and distinguishes runnable from running before `emit_investigation_complete`. It remains soft evidence calibration; it does not scan request/answer prose, reject an answer, select a cause, or rewrite the model's conclusion.
