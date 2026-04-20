# Prompt / Skill / Hint 一致性与冲突审计（2026-04-20）

## 审计范围（端到端）

本次审计按“编排 → 构建 prompt → agent 补充 → tool/runtime gate → retry/hint 回注”整链路阅读：

1. 阶段绑定：`analyze/explore/extract/finalize` 与 skill 的固定绑定。
2. Prompt 组装：`BuildPromptContext` 的 system/user section 与顺序约束。
3. Evaluator 动态补充：`BuildInitialInstruction` 是否重述 skill 静态契约。
4. Runtime gate：prompt 提示是否有对应的运行时硬约束。
5. Hint 通路：`RetryHint` 从 orchestrator 注入到下一轮 prompt 的结构化程度与重复情况。

---

## 总体结论

- **总体架构方向正确**：已经明确了 **Skill(静态契约)** 与 **Evaluator(动态补充)** 边界，并在接口注释、builder 注释、测试层做了制度化约束。
- **存在“内容重复但非致命冲突”**：尤其在 `explore` 阶段，builder 已给出 Retry Directive，explorer evaluator 在 retry 分支再次嵌入 `Retry directive`，形成重复指令。
- **存在“文本与运行时 gate 不一致”风险点**：部分文案仍保留“2 轮硬上限”表述，但实际代码引入了可配置/可放宽路径（例如分类 line-level grep 的 C0' 机制）；若不统一“术语与边界”，LLM 会接收到混合信号。
- **存在“标题层级歧义”**：`Retry Directive (READ FIRST)`（builder 标题）与 hint composer 输出 `## Retry Directive`（内容内二级标题）叠加，语义上相同但表现层两套命名。

---

## 发现清单

### 1) 重复：Explore retry 指令双写

**现象**

- builder 已把 `TaskState.RetryHint` 作为首个 user section 注入：`Retry Directive (READ FIRST)`。
- explorer evaluator 在 retry 分支再次写入 `**Retry directive:** ...`。

**影响**

- 增加 token 噪声；同一轮内可能出现“上层窗口 hint + evaluator 本地 hint”双重措辞。
- 当两处文案未来演进不一致时，可能演化为软冲突。

**证据**

- `internal/context/builder.go`：Retry section 前置。
- `internal/agent/explorer.go`：retry 分支再次注入 directive。

---

### 2) 标题语义重复：Retry Directive 外层/内层命名不统一

**现象**

- 外层 section 标题是 `Retry Directive (READ FIRST)`。
- F4 HintComposer 的渲染正文又以 `## Retry Directive` 起头。

**影响**

- 同一 user message 内出现“标题套标题”效果，视觉可读性下降。
- 对“canonicalUserSectionOrder 禁止重名段落”的规则是规避式通过（因为一个是 section title，一个是内容内 markdown 标题），但认知上仍重复。

**证据**

- `internal/context/builder.go`：`Retry Directive (READ FIRST)`。
- `internal/analysis/hint/composer.go`：`Render()` 固定输出 `## Retry Directive`。

---

### 3) 文案与 gate 边界存在潜在漂移

**现象**

- `analysis-skill` 文案里同时出现“最多 2 轮预扫硬上限”和“Round 2 可 files_only=false（受触发条件 + 预算）”。
- runtime 侧确实实现了放行逻辑（trigger + call/byte budget + clamp），且 analyzer evaluator 还有 budget override（多子题启发）。

**影响**

- 规则虽可共存，但“round 上限”和“call 上限”是两套维度，文案若不精确定义优先级，模型易误解为互斥。

**证据**

- `internal/skill/analysis_contract.go`：Pre-scan 与 ClassificationGrep 文案并存。
- `internal/agent/analyzer.go`：prescan round 计数与 must-emit hint。
- `internal/agent/agent.go`：`validateAnalyzerPrescanToolCall` 的 C0' 放行与预算 gate。

---

### 4) 边界总体健康：Skill / Evaluator 分工有明确护栏

**现象**

- Evaluator 接口注释明确“只允许动态补充，禁止重述 skill 静态合同”。
- prompt 组装路径固定为 assemble → render → append dynamic。

**正向结论**

- 这是当前系统最稳的约束点，建议继续沿用并把“重复检测”自动化。

**证据**

- `internal/agent/agent.go`（Evaluator 注释与 `buildInitialMessages` 三步）。
- `internal/agent/prompt_assembler.go`（AppendDynamicInstruction 设计）。

---

## 冲突等级评估

- **硬冲突（会导致执行错误）**：暂未发现直接硬冲突。
- **软冲突（会导致模型指令分散）**：中等，主要在 retry 指令重复与标题重复。
- **歧义（模型可能误读）**：中等，主要在 analyze 阶段“round/call/budget”三层规则并存的描述精度不足。

---

## 改进方案（建议按优先级分三步）

### P0（立即，低风险）——去重与命名统一

1. **去掉 explorer retry 分支中的二次 Retry directive 文案**，保留 builder 的统一入口。
2. **HintComposer 渲染从 `## Retry Directive` 改为无标题正文**（或改成 `### Details`），避免与外层 section 重名。
3. 在 `BuildPromptContext` 的 Retry section 注释中加一句：
   - “retry hint body 不应再自带同名一级标题”。

### P1（短期，中风险）——统一 analyze 规则术语

1. 在 `analysis-skill` 文案把规则拆成三段固定模板：
   - **Round budget**（按 LLM response 计）
   - **ClassificationGrep call/byte budget**（按 tool call 计）
   - **Priority**（当冲突时以 runtime gate 为准）
2. 在 analyzer must-emit hint 中附带当前预算快照（round/calls/bytes）以减少误解。

### P2（中期，收益高）——自动化“重复与冲突检测”

新增一个 prompt lint（单测即可），在 assemble+append 之后做：

- 检测重复标题（忽略大小写）
- 检测 retry 语义重复短语（如“Retry Directive”出现次数 > 1）
- 检测 skill.Prohibitions 与 evaluator 动态文案中的显式反向命令（正反冲突）

---

## 建议落地顺序

1. 先做 P0（几乎不改行为，纯降噪）。
2. 再做 P1（只改文案与提示拼装，不触发工具行为变化）。
3. 最后做 P2（把本次人工审计规则固化为 CI 约束）。

---

## 附：本次审计读取的核心代码入口

- 编排入口：`internal/orchestrator/orchestrator.go`, `internal/orchestrator/topology.go`
- Prompt 组装：`internal/context/builder.go`, `internal/agent/prompt_assembler.go`
- Agent 核心循环：`internal/agent/agent.go`
- 分阶段 evaluator：`internal/agent/analyzer.go`, `internal/agent/explorer.go`, `internal/agent/extractor.go`, `internal/agent/answer_document_evaluator.go`
- Skill 合同：`internal/skill/defaults.go`, `internal/skill/analysis_contract.go`
- Hint 合成：`internal/analysis/hint/composer.go`
