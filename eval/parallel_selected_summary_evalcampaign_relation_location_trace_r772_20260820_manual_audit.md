# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T12:33:08Z
- sweep_start_ts: 20260820-053307
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-053308 | answer_regex,answer_contains | none | 176s | 31 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B1236 正向：最终模型答案、direct definition 与引用均为 `agent.go:519`，系统不再发射职责注释行 515/516 为精确定位；12 个 production 成员及各自路径仍齐全。新确定性 gap B1237：Mermaid 画成 `LoopController --implements--> 实现类`，方向反了；更关键的是 edge anchor 同时声明 `from_identity=实现类` 但 `from_node=LC(LoopController)`，节点身份与端点身份自相矛盾仍签绿。一次 citation evidence_id 修补和一次 member_set facet 修补，均保留模型图；问题不是系统改图。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-053308 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 363s | 41 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、四节点链、11.000ms 链上 iowait 首因、三个 1.000ms runnable 席、实际占时/规则可消双轴、Trace 因果投影和背景隔离全部保留，零成文拒绝。上轮 11→14ms 错位未复现；本轮模型把 `threadpool→network` 的 2.016000s 唤醒写成 2.014000s（后者是 threadpool 自身被唤醒），而同页系统投影和 typed handoff 均正确为 2.016000s。连续两轮漂移字段不同，暂判模型压缩/抄写波动，不加正文扫描硬门或系统代写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- Runner：2/2 PASS；人工：2/2 partial。两路持续活跃，未因 4ms、4m、首字节、stall 或累计年龄降级；Trace 363s 完成后仍使用本轮有效答案，没有旧稿/空答案降级。
- B1236 已获输出正证，职责注释仍作为 EvidenceRef 提供 WHAT 上下文，但不再污染最终精确位置；共享类型层和真实持久化 pin 提供确定性闭环。
- B1237-EDGEANCHORNODEIDENTITYBINDING1（P1，确定性）：当前合同验证 edge anchor 的 typed `from_identity/to_identity`，却未要求它们分别绑定到 Mermaid `from_node/to_node` 解析出的节点身份。模型可用正确 identity 对通过关系证据门，同时把节点 ID 指向相反实体，导致反向/错端点图签绿。最优修复是统一节点符号表后校验 `node_id ↔ canonical identity`，不扫描用户原文、thinking、业务标签词义，也不让系统改边。
- Trace 的 2.016→2.014 与 r771 的 11→14 是不同字段漂移；精确上下文和确定性投影均正确，暂无稳定合同缺失证据。继续观察更高优先级异构 Trace，不用单 case 数值硬编码。
