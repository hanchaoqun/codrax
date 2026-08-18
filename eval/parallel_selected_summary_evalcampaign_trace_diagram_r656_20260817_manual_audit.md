# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T01:36:21Z
- sweep_start_ts: 20260817-183619
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | FAIL | eval/results/trace_query_wakeup_causal_runnable-20260817-183621 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 120s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Exact 1.000000..1.010000 window, system supplementation, worker-200→app-100 wake edge, worker #1 effective 8.300ms and typed priority relation all exist. The model classified the request as `bounded_fact_set` instead of causal diagnosis, so the finite-scope finalizer hint forbade root-cause wording and the declared oracle failed. This same case passed r655, making classification a model-variance witness rather than grounds for a request-text hard gate. Two independent context defects are real: the compact final recap listed `20/ohos_cfs` and `52/ohos_rt` without binding them to waker/wakee, after which the model swapped the roles; and zero target wait-reason rows was over-interpreted as cooperative `sleep/nanosleep`, although absence of a typed reason row cannot prove a mechanism. Deterministic projection retained the exact ranked row but correctly did not replace the model conclusion. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-183621 | answer_regex,answer_contains,mermaid_edge_count | none | 265s | 39 | read=18,repo_map=1,list=1,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=2,unavail=0,prune=1 | pass | B1032 received its first production-positive witness: after exact caller argument-flow evidence, the repair frontier directed the model into `BuildAgentContext`; it emitted the grounded `BuildAgentContext -> bus.Mutable.Objective` call. The accepted diagram preserves the four-stage pipeline and a connected `BusContext -> BuildAgentContext -> Mutable` component using customer-facing labels. B1033's reverse branch was not exercised because the forward handoff was available first. Two rejects correctly removed retargeted/unsupported edges and repaired identity metadata; runtime fell from r655 419s/3 rejects to 265s/2 rejects, still leaving convergence cost to observe rather than weakening the relation gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized disposition

- `B1032-CALLEEBODYRELATIONFRONTIER1`: production-positive in r656; the model consumed the typed navigation and emitted the missing callee-body call itself.
- `B1033-CALLEEBODYREVERSECALLFRONTIER1`: implemented and pinned, but the reverse branch remains pending a production trigger.
- `B1034-TRACEWAKEUPROLECAPSULE1`: confirmed P1. A compact final handoff must bind every exact priority/class/CPU value to the typed waker or wakee role; an unordered semantic-value list is insufficient and enabled a role swap.
- `B1035-WAITABSENCEMECHANISMBOUNDARY1`: confirmed P1. Zero typed wait-reason rows authorizes only absence from that roster; it cannot prove cooperative/voluntary sleep, a `sleep/nanosleep` syscall, or any other wait mechanism.
- The r656 bounded-scope choice is retained as model variance because the identical request reached causal diagnosis in r655. Do not add a raw request/output keyword hard gate or let the system choose the conclusion.
- Explicit-window query, typed on-chain ranking and deterministic supplementation remained intact. Background evidence was not promoted to root cause, and an active response stream was never replaced by a fixed-4ms fallback.
