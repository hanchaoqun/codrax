# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T20:40:28Z
- sweep_start_ts: 20260831-134026
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260831-134028 | answer_regex | none | 110s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 五条源码调用边、分支实现、walker 的递归收集职责与完整 sequenceDiagram 均保留。首稿因 principal list 没有 edge_anchors 被拒，模型以一次局部 patch 补齐；本轮列表和图原生分开输出，未触发 fused-split，故只能证明 B1510 无回归，不能冒充其生产触发正证。 |
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-134028 | primary_answer | none | 147s | 29 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Finalizer typed boundary 已明确 `AuditLog.record -> System.out.println` 且 storage/durability 未证，模型仍把内存 `rows.add` 和 stdout 描述为“持久化/落库”，按模型消费波动保留，不加正文硬门。确定性新 gap 在图：错误 guard anchor 被移除后，`R-->>S: n >= max` 位于任何 S→R 调用之前，却被校验器用后面的 `S->>R: insert` 反向端点配成合法 reply；结果 countOpenVisits 调用消失、返回悬接到另一操作。登记 B1512。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1510 replay disposition

Rust 本轮没有生成 fused block；系统没有机会执行 `fused_diagram_split` lineage 恢复。最终图完整且关系校验通过，说明 B1510 没有破坏普通独立图，
但生产状态继续是 `implemented/pending-trigger-replay`。不能用 runner PASS 把未触发修复记成闭环。

### B1511 terminal effect disposition

Java explorer 读取 `AuditLog.java`，typed answer chain、evidence context 与 Final Call-Chain Evidence Boundary 均精确发布
`AuditLog.record calls System.out.println @ AuditLog.java:6`；finalizer 教学还明确 storage/durability/flushing/completion 未证。模型 thinking 也看到了
stdout，却仍把 `rows.add` 称为持久化并沿用“审计落库”。当前证据足以回答，残余归为模型波动；禁止扫描最终正文、替换结论或按 Java/println 特例加硬门。

### B1512-SEQUENCEREPLYORDER1（P1/new confirmed）

Java 首稿把 `S->>R: countOpenVisits` 锚错标为 guard，关系门正确要求移除/纠正。patch 删除该 forward edge 后，图的顺序变为：

1. `R-->>S: n >= max`；
2. 稍后才出现 `S->>R: 持久化就诊记录`（真实 typed identity 是 insert）。

当前 `diagramSequenceStructuralReplyKeySet` 先扫描全图构建无序 forward endpoint set，再把任意反向 `-->>` 认成 structural reply；因此“回复先于调用”及
“一个操作的回复借用后来另一个同 actor-pair 调用”均被放行。最优根修是纯结构、顺序敏感的 pairing：按 edge 出现顺序扫描，`-->>` 只能消费此前最近一个
尚未配对、端点恰好反向的 forward invocation；没有先前 invocation 时继续进入普通 typed relation gate。不得读取消息文案、请求/答案 prose 或业务方法名，
不得自动新增/移动/改写模型边。需覆盖同 actor pair 多次调用、嵌套 actor、activation suffix、显式 typed return 与先 reply 后 call 的正反 pin。
