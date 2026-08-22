# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T07:14:09Z
- sweep_start_ts: 20260822-001409
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-001409 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 207s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=3,inv=1/0,fin_reject=3,unavail=0,prune=0 | uncertain | 显式窗、三条 typed 唤醒边、11ms IO 第一席、三个 1ms 优先级候选、实际占时/规则可消双轴、业务线索、因果投影与系统补齐均保留；但正文把链上 IO 候选写成“主要阻塞原因/完成后才触发后续”，又同时披露直接帧因果未证，并把四节点三边称为“四跳”，存在模型过度归因与计数不一致。三次拒绝中两次是模型把仅允许放在 principal summary 的 caliber 元数据重复放到 list/section；合同教学本身无冲突。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-001409 | answer_regex,answer_contains | none | 693s | 53 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=11,unavail=0,prune=0 | uncertain | 可见结论正确覆盖 JsonPlugin、run_pipeline→resolve、REGISTRY 查找、cls() 实例化、callback 与 mixin，但结构关系最终只剩两条直接 call，lookup/return 断言缺少实际行引用，图在 11 次关系拒绝后被删。生产证据含 selector application、return、callback/type，却缺 registration、lookup assignment、同调用点 argument_flow，故 B1329 编译器按设计扣留；确认上游 typed producer GAP B1330，而非放松关系门或继续堆 prose 教学。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
