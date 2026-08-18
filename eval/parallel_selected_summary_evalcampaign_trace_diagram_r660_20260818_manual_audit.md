# r660 人工审计：bounded closure 生产回放与系统锚点表展示缺口

- 基线：`main@a59d817e9`
- 执行：`PARALLEL=2`，恰好两个案例
- runner：`2/2 PASS`
- 产物：
  - `eval/results/trace_query_wakeup_causal_runnable-20260817-203029`
  - `eval/results/qf_logic_view_read_pipeline-20260817-203029`

| 案例 | runner | 人工结论 | 关键审计 |
|---|---|---|---|
| `trace_query_wakeup_causal_runnable` | PASS，181s，0 次成文拒绝 | pass-with-caveat | 精确 `1.000000..1.010000` 窗、app-100 五态、CPU2→CPU1 唤醒、worker-200 链上 #1=8.300ms、实际占时/规则可消双轴、因果投影、邻近/背景权限均完整。正文仍复制 `coverage_status=complete`、`target_direct_blocking_authority=not_provided`、`priority_inversion_candidate=true`、`lock_priority` 等 wire/audit token；同轮 reader policy 已明确禁止，归重复模型服从/长上下文显著性观察，不加答案字符串硬门或系统改写。 |
| `qf_logic_view_read_pipeline` | PASS，380s，1 次成文拒绝/1 次 patch | fail（关系维度） | B1039 生产正证：Finalizer 看到自然语言 `evidence-bounded answer`，没有裸 `result_kind: resolved`，并收到 `requested_relation_spine_status=unproven`。终图只保留三条已证 stage precedence，BusContext/Mutable 以无箭头 grouping+可见未证边界存在，关系门正确；但正文仍宣称 BusContext/Mutable 已完整传递各阶段产物，与同页未证边界冲突，用户要求的数据流未被证明。上下文已有精确禁越界说明，暂归模型服从，不允许系统代写完整 flow。 |

## B1039 生产结论

1. Explorer 对 `flow_participant_coverage` 连续三次无进展后有界结束，completion result 机器态仍为
   `resolved`，并持久携带 Mutable/BusContext 未证 relation caveat。
2. Finalizer 的 `Accepted Closure Status` 精确显示“证据足以形成有边界的答案，但结构化证据边界仍未证”，
   且不存在邻接的裸 `result_kind: resolved`。因此 B1039 的机器态/模型态解耦在生产生效。
3. 该修复没有自动提高模型对关系边界的服从率；它只消除了系统自己的歧义。最终正文越界不能反向解释为
   B1039 失败，也不能据此授权系统扫描、删除或替换模型答案。

## 新确认的确定性系统展示 gap

代码案尾部由系统生成的“关键源码锚点”表出现 `项目/位置/锚点/说明/列 5`，并含
`Mutable documents Mutable` 一类空洞机械说明。它不是模型波动：该块由
`normalizeCurrentSourceCitationSupplement` 创建，row 同时携带 Label、Text 和重复 Cells，renderer 因列宽
漂移补出 synthetic header。记为 `B1040-SYSTEMSOURCESUPPLEMENTTABLE1/P1`。

最优修复：系统块只携带严格两列 `位置/源码锚点`（英文 `Location/Source anchor`），row 使用 cells-only
且与 columns 等长；引用仍由 CitationRef 保留。删除的是重复结构载体与无信息机械说明，不是模型正文、
证据或结论。渲染 pin 必须证明不再出现 `列 3/4/5`。

## 不变量

- 不扫描用户原文、模型 thinking/草稿/答案或 Mermaid 文本做新硬门。
- 不由系统生成关系边、图或结论；未证 requested relation 继续可见。
- Trace 显式窗、因果投影、自动补齐、链上-only 根因、邻近/背景 support-only 和双轴计价保持。
- 活跃流不得因 4ms 或累计年龄降级；本轮两案均等待完整结果。

状态：

`B1039-COMPLETIONLOWDELTAAUTHORITY1=production-closed-r660`；
`B1038-DIAGRAMREQUESTEDRELATIONCOMPONENTS1=production-positive-r660`；
`B999-QFPROSETRANSFEROVERCLAIM1=recurrent/context-sufficient/model-adherence-watch`；
`B1040-SYSTEMSOURCESUPPLEMENTTABLE1=implemented/strict-two-column+full-tool-pass`；
`Trace explicit-window/query/projection/auto-supplement=production-positive-r660`；
`active-stream-fixed-4ms-degrade=forbidden/not-observed`。
