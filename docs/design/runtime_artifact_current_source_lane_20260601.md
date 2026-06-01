# Runtime Artifact 与当前源码 Lane 决策修复

## 背景

客户日志 `comp_reject.log` / `comp_reject_repl.txt` 暴露了一个系统性问题：用户问题的主体是
runtime trace，模型也通过 `trace_query` 得到了足够的 runtime 观测结论，但
`emit_investigation_complete` 在完成前仍把 `.systrace`、`sys.systrace`、`info.txt`、`sys`
等路径当成当前仓源码实现文件，触发 `primary_anchor_unread` / `phase1_unread` forced-read gate，
导致探索阶段被迫重新读文件并循环。

本问题不能通过用户问题关键字处理。默认规则仍然是：没有明确禁止源码分析时，外部观察可以和当前源码一起分析。
但默认允许源码分析不等于“没有当前源码锚点时必须硬阻塞完成”。硬门只能消费 typed precise signals。

## 日志中发现的问题

1. Runtime artifact 被误当作 current-source implementation file。
   - `primary_anchor_unread` 对 `.systrace` exact-entity ranked path 排队 forced read。
   - `phase1_unread` 对 trace 相关文件排队 forced read。
2. 默认 external observation + source allowed 被混同为 current-source required。
   - 用户未禁止源码分析，系统应允许源码探索。
   - 但当 runtime lane 已足够且没有当前源码锚点时，不应把 runtime artifact 当源码硬门。
3. `aggregate_facts.total_count` 被模型用于小数时长。
   - 例如 `119.227 ms` 是 scalar duration，不是 integer count。
4. Runtime artifact 的 decorated `member_set` 被要求 `support_refs`。
   - 例如 `binder_wait (synchronous-looking)` 是 runtime observation 成员，不是源码符号装饰。
   - 当前只在 explicit observation-only 时放行，默认 mixed 场景仍被误拒。
5. `aggregate_facts` 超过 16 条会硬拒。
   - 对 runtime 支撑事实，硬拒会放大循环；应保留主事实并有界压缩可选事实。
6. JSON 兼容修复后曾出现 `reason` 缺失。
   - 现有硬拒是对的，但提示要强调：不要把终止结论只放在工具调用前的散文里。
7. forced-read UX 文案把 runtime artifact 说成 source files。
   - 对 trace/log 应给软提示或 runtime line window 建议，不能说“必须读实现文件”。

## 设计

新增内部 typed decision，不新增模型可见字段：

- `required`：当前源码是硬要求。来源包括当前源码解释 profile、resolved current repo files、明确 current-key-code 维度、current-source required file hints。
- `allowed_optional`：默认 external runtime/log/trace/MCP 混合场景。允许读源码；如果 runtime artifact lane 足够且无当前源码锚点，不硬阻塞完成。
- `excluded`：用户通过 typed `ExternalObservationPolicy=exclude` 明确排除当前源码。
- `satisfied_absent`：保留为后续能力，用于结构化负搜索证明当前源码无交集后降级硬门。

实现原则：

- forced-read gates 只在 `required` 时硬阻塞。
- 即使 `required`，`.log/.trace/.systrace/.htrace/.atrace/.perfetto` 等 runtime artifact path 也不能作为 current-source forced-read seed。
- 默认 external observation 仍然进入 AnswerContract 的 current_source + runtime_artifact mixed origins；这保证源码结合分析能力不被关闭。
- Runtime artifact provenance 的 decorated members 可以依赖 runtime origin，不要求 current-source `support_refs`；当前源码 required 时仍保持原校验。
- count/scalar 兼容只在结构化安全条件下做：小数 `total_count` 自动转为 `scalar_value`，整数 count 语义不变。
- 超量 aggregate facts 只压缩 optional runtime facts；current-source required 场景仍 fail loud。

## 开发任务

1. 在 `internal/types` 增加 runtime artifact path helper 与 `CurrentSourceLaneDecision`。
2. 收敛 `HasRuntimeArtifactCurrentVerificationAnchor`，避免 unresolved external exact targets 自动打开 current-source hard lane。
3. 在 `emit_investigation_complete` 的 `primary_anchor_unread` / `phase1_unread` 中接入 lane decision，并跳过 runtime artifact paths。
4. 放宽 runtime artifact decorated `member_set` 的 origin-specific support 判定，保持 current-source required 场景不变。
5. 增加 `aggregate_facts` 兼容：
   - 小数 `total_count` → `scalar_value`。
   - optional runtime facts 超 16 时有界压缩。
6. 更新工具提示文案，让模型区分 count 与 scalar duration。
7. 添加看护测试：
   - trace-only enough 不被 current-source forced-read 卡住。
   - mixed trace+source required 仍强制源码证据。
   - `.systrace` 在仓库目录下也不作为 implementation file。
   - runtime decorated member_set 默认 mixed 场景可通过。
   - current-source decorated member_set 仍要求 support refs。
   - decimal total_count 兼容为 scalar。

## 非目标

- 不改变 repo_map / source citation gate。
- 不用用户问题关键字作为 hard gate。
- 不自动把 trace/log/MCP 行号当成当前源码引用。
- 不关闭默认“外部观察 + 当前源码”混合分析能力。
