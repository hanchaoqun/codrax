# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T12:53:43Z
- sweep_start_ts: 20260820-055342
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-055344 | answer_regex,answer_contains | none | 256s | 33 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B1237 正证：12 条可见边方向和同侧 identity 均正确；但修补后图从权威 production 12 成员扩成 15，混入 3 个 test helper。系统同时给出“生产 12 个”的权威 member_set 和“全 source scope 15 条”的 First-Pass Diagram Reference，属于确定性上下文自冲突（B1239），不是模型随机波动。表格仍正确为 12。 |
| 2 | trace_query_wakeup_causal_io_chain | FAIL | eval/results/trace_query_wakeup_causal_io_chain-20260820-055344 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 262s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | analyzer 把明确的“主要阻塞原因 + 相关链路”分成 performance_bottleneck/call_chain/flow/relational，却把 runtime scope 设为 bounded_fact_set；有限范围提示随后主动抑制 root-cause/rank/seat，family 落到 generic，Trace 因果投影为 0。已有链上 threadpool-400 D/iowait 约 11ms 未进入结论，模型反而把 app-100 自身 20ms S 态写成主阻塞，并推断网络操作/cookie 回传。generic 图又误入源码关系门，导致真实 runtime 唤醒图被拒并删除。B1238。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generalized gaps

### B1238 — runtime causal request classified as bounded facts

- Exact typed contradiction: `intent=trace`, `scenario=performance_bottleneck`, `question_kind=call_chain`, `predicate_axis=flow`, `is_relational_lookup=true`, while `runtime_question_profile.scope=bounded_fact_set` and no causal answer dimension was emitted.
- Consequence is architectural, not merely presentational: bounded scope suppresses causal report materialization; `family=generic`; no `root_cause_rank` principal handoff and no Trace causal projection block. The finalizer was explicitly told not to use root-cause/rank/seat language.
- The model did receive a real wakeup chain, but the wrong scope left it without the ranked chain-only cause contract. It promoted the target's S-state symptom and invented business roles, while the chain-terminal D/iowait evidence was lost from the conclusion.
- The optional runtime diagram then entered the non-runtime source-code relation gate. This second symptom is caused by the same wrong family for this case. Do not solve it with a report-wide runtime bypass: that would let a sibling source-code diagram evade evidence validation. Restore the causal family first; retain existing block-local runtime-temporal authority for finite runtime diagrams and audit other finite causal relation forms separately.

### B1239 — authoritative principal member set and diagram seed disagree

- The authoritative `system:typed_relation_principal_member_set` contains 12 production implementations and the final table preserves exactly those 12.
- The same finalizer prompt publishes a 15-edge First-Pass Diagram Reference from all request-scoped typed relations, including `isolatedPromptEvaluator`, `protocolSoftStopAcceptEvaluator`, and `protocolSoftStopEvaluator` from test files, then says the seed is a floor that may be extended.
- The first model draft had the correct 12-member diagram. After a relation repair, the model copied the system's 15-edge reference. This is deterministic producer-owned context pollution, not an unsupported model hallucination.
- Required follow-up: filter the diagram authoring carrier through the same typed principal-member authority when a closed principal relation set exists, while retaining excluded relations as separately labelled support/audit context. Do not infer membership from labels or final prose and do not system-author graph edges.

## Invariants checked

- No user-request, model-thinking, final-prose, or Mermaid-label keyword scan is proposed as a hard gate.
- System does not select or rewrite the model's root cause, graph edge, node, label, or conclusion.
- Explicit-window Trace causal projection and automatic supplement remain required for causal diagnosis; bounded status/relation queries remain supported through their own typed scopes.
- Trace primary cause remains typed on-chain only; adjacent/background evidence remains support-only.
- Both runs remained active beyond 4ms and completed normally; no first-byte/stall/4-minute/total-age degradation was observed or permitted.
