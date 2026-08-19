# r715 人工审计：参与者关系收敛与有限窗 Trace 口径

- 日期：2026-08-18
- 基线：`main@1dd1187af`
- 并发：严格 2；同一不可变二进制
- 案例：`qf_logic_view_read_pipeline`、`real_trace_h4_supply_thermal_witness`

## 结论

| 案例 | Runner | 人工判定 | 关键结论 |
|---|---|---|---|
| `qf_logic_view_read_pipeline` | PASS | partial | B1134 首轮候选已经生产生效，但 Explorer 的参与者关系收敛键把变化的导航坐标当成证明进展，造成 43 轮探索、28 次读取、4 次成文拒绝；终稿关系正确但仍有一个同名 `Mutable` 幽灵节点。 |
| `real_trace_h4_supply_thermal_witness` | FAIL | partial | Runner 仅因未接受“policy 上限”同义形而误报；四态与 CPU 供给结论正确。正文却把未做区间 join 的 50 条 blocked_reason/Σ16.358ms 越权解释成 70.338ms Sleep 的主要来源，与同页 typed 口径附注矛盾。 |

## B1134 生产复核

首轮 Finalizer prompt 已发布与 hard gate 同源的 typed participant candidates，包括：

- `o.busCtx -> BuildAgentContext`，`BusContext` 位于 from 端，业务标签“作为参数传递”；
- `context.BuildAgentContext -> bus.Mutable.Objective`，`Mutable` 位于 to 端，业务标签“调用”。

因此 B1134 的数据通道已生产正证，但不能据此宣称重试问题闭环。模型在前置探索阶段已被要求反复寻找把所有参与者连成一个组件的关系；大量上下文和变化中的搜索前沿淹没了首轮 recipe。终稿最终保留四阶段顺序以及上述两条真实边，未由系统代画或代写结论。

## 新确认 B1135/P1：导航坐标误充关系证明进展

`flowParticipantCoverageBlockerKey` 当前同时哈希缺失参与者集合和下一处 parser-owned 导航坐标。坐标从 carrier call、callee body、caller handoff、sibling argument 逐步变化时，即使缺失参与者集合完全没有缩小，收敛计数仍不断清零。r715 因此出现 43 轮 Explorer、21 次 mid-loop 注入与 90k 级上下文。

最优泛化修复是：收敛身份只由 typed 缺失参与者集合决定；导航坐标继续决定下一次精确读取和提示，但不再授予无限重试额度。只有关系证据真正覆盖参与者、使缺失集合缩小时才重置额度。保留三次关闭机会和最终 typed unproven boundary，不放松关系 hard gate，也不扫描用户或模型原文。

另记 P2：终稿出现两个可见标签同为 `Mutable` 的节点，其中一个未连接。后续审计 requested participant 的可见身份去重；不得通过删除图或系统重画来处理。

## 新确认 B1136/P1：blocked_reason census 被越权解释成整段 Sleep 机理

有限窗问题不要求 Trace 因果投影；本轮没有强行生成因果投影是正确行为。`target_window_states` 精确给出 233.190ms 窗口的 Running 157.248ms、Runnable 5.604ms、Sleep 70.338ms、D-state 0。模型正确指出 policy 行存在但缺少 target-running-slice 与 policy overlap，因而不能证明性能影响。

错误在于模型随后把 50 条 blocked_reason 及 Σ16.358ms 说成 70.338ms Sleep 的“主要来源”。typed 事实只证明调用点 census；没有 `record_to_state_occurrence_mapping`，不能把它配给任一 Sleep 段，更不能从函数名词形推导存储/网络缓存页机理。系统附注已正确披露这一点，但提示出现得太晚、且有限窗 hint 中“interval-joined subset”措辞与真实 `unjoined_distinct_observation_domains` 冲突。

修复应在模型成文前统一发布简洁、无矛盾的 typed caliber：未提供 interval join 时，blocked_reason census 对任何 Sleep/D/IO 段的来源解释权限均为 unproven；只允许按调用点记录口径并置。系统仍不扫描或改写模型正文。

## Eval 口径

Trace Runner 的失败是 oracle false negative：答案使用“policy 上限……性能影响绑定未获证明”，语义与既有 hard oracle 一致。仅补充接受“policy 上限”字面，不放宽 CPU4、2.10GHz、未证绑定及状态数值要求。

## 不变量

- 显式时间窗、Trace 因果投影和自动补齐路径不变；有限事实问题不强制因果投影。
- 主因仍只来自 typed on-chain 证据；邻近与背景只作支撑。
- 实际占用/业务线索与规则可消除量双轴不丢失。
- 系统不代写模型关系、诊断、结论或图。
- 活跃字节流不因 4ms 或固定累计年龄降级。
