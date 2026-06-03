# 通用电脑操作 Agent + 外部 Skills/MCP 能力设计

**状态**: 设计落盘, 暂不改生产代码。  
**日期**: 2026-06-03  
**目标**: 支持独立于源码分析、日志/trace 分析、写代码管线之外的通用电脑操作和制品生成能力, 例如桌面操作、浏览器操作、生成 PPT/文档/表格、调用外部技能工作流等。设计必须保持现有读代码、log/trace、写代码、MCP 外部观测能力稳定。

---

## 1. 当前代码结论

### 1.1 REPL 路由现状

当前 REPL 前置分类是 `TurnPolicy`, 位于 `internal/repl/turn_policy.go`。它是 typed tool-call 分类, 不是 Go 关键字匹配:

- `route`: `local | repo | hybrid | clarify`
- `operation`: `chat | transform | summarize | translate | elaborate | investigate`
- 失败兜底: 分类器异常、低置信或未知枚举时回到 `repo`, 避免源码问题被误判后跳过分析。
- `ApplyTurnPolicyGuards` 只用结构化事实修正自相矛盾, 例如 `hasPriorAnswer`、`hasAttachment`、`needs_repo_access`。

这个设计适合扩展, 但不能把电脑操作硬塞进 `local` 或 `repo`:

- `local/chitchat` 是非管线聊天/上一轮答案转换, 不读仓库、不执行外部操作。
- `repo/hybrid` 是现有读模式分析管线, 目标是源码/运行时观测证据和最终答案。
- 电脑操作/PPT 生成是第三类任务域, 可能有文件写入、桌面 UI、副作用和产物验证, 需要独立的 route。

### 1.2 Pipeline 与 stage 现状

`internal/types/enums.go` 与 `internal/types/stage_binding.go` 定义了现有 stage/agent 绑定:

- 读模式: `log_triage? -> perf_triage? -> analyze -> explore -> extract -> finalize`
- 写模式: `write_analyze -> plan -> apply -> verify`
- `PipelineStage.IsWrite()` 是读写隔离的单一真源。

`internal/orchestrator/topology.go` 从 `types.AllStageBindings()` 构造硬编码 topology。现有主干没有 operation stage。

结论: 通用电脑操作不应复用 read pipeline 的 evidence gate, 也不应复用 write pipeline 的 git worktree apply/verify 语义。应新增独立 operation pipeline, 并通过 `StageBinding` 进入统一 stage/agent/skill 表。

### 1.3 MCP 现状

当前 MCP 底座已经具备:

- `internal/mcp`: stdio JSON-RPC server、命名空间化工具 `<server>__<tool>`、资源枚举读取、typed observation envelope。
- `internal/agent/agent.go`: MCP 工具只暴露给 `explorer/sub_explorer` 的 `StageExplore`; `extractor/finalizer/write` 看不到。
- `MCPResponse -> ObservationLedger`: MCP 结果进入 `origin=mcp_resource`, 是外部观测, 不是当前源码 citation。
- 资源只允许读取 `resources/list` 枚举过的 URI, 不拼 URI。
- 大输出进入 blob/PayloadRef, 面板只显示摘要/引用。

当前 MCP 是"外部只读观测"定位, 不是通用电脑操作能力。扩展时不应该新增一套完全不同的 MCP 接入类型; 更稳的是在现有 `MCPServerConfig`/Registry 上增加 capability metadata 和 agent/stage gate。

### 1.4 Skill 现状

`internal/skill/defaults.go` 注册内置 skill, 每个 skill 通过 `ToolSuggestions` 决定 agent 可见工具。Skill 是纯配置, agent 根据 stage + skill 构建 prompt 和工具面。

现有外部 plugin skills 不在本仓统一建模, 但设计上可以抽象成一种 `CapabilityDescriptor`, 和 MCP tool metadata 对齐。

---

## 2. 目标与非目标

### 2.1 目标

1. 增加独立的 `computer_operation` / `artifact_generation` 任务域。
2. 支持外部 skills 扩展: 桌面操作、浏览器操作、PPT/文档/表格生成、公司内部工作流。
3. 扩展 MCP 能力, 让 MCP 工具既可作为外部观测, 也可声明为操作型或制品生成型能力。
4. 所有新增能力默认不影响现有源码分析、log/trace、写代码、MCP 外部观测路径。
5. 所有暴露给模型的新字段后续都必须接入统一 JSON 参数兼容/修复机制。

### 2.2 非目标

1. 不把 chitchat/local responder 改成电脑操作 agent。
2. 不让电脑操作产物进入当前源码 citation gate。
3. 不让外部 skill/MCP prompt 成为系统指令。
4. 不默认执行高风险桌面/网络/删除/提交动作。
5. 不以 Go 关键字匹配判断用户意图。
6. 不在没有配置 operation 能力时改变任何现有工具面或 prompt。

---

## 3. 红线

1. **默认稳定**: 未启用 operation 时, `local/repo/hybrid/clarify` 行为不变。
2. **typed decision**: 是否进入电脑操作由 typed classifier 字段驱动, 不靠关键词。
3. **能力 gate 精确**: 硬 gate 只能读 capability enum、risk enum、side_effect enum、stage/agent enum, 不读工具描述散文。
4. **外部内容不升权**: MCP resources/prompts/skill docs 都只能作为外部建议或工具 schema, 不得成为系统指令。
5. **产物与证据分离**: PPT/文档/截图/浏览器结果属于 operation result/artifact result, 不进入源码 evidence/citation lane。
6. **副作用显式**: 会写文件、操作桌面、访问网络提交、发送邮件、删除/覆盖等动作必须有 risk/side_effect 标记和 confirmation 策略。
7. **现有 MCP 不破坏**: 未声明 capability metadata 的 MCP server 继续按只读外部观测处理, 只暴露给 explorer/sub-explorer。

---

## 4. 推荐架构

### 4.1 新增任务路由

扩展 `TurnRoute`:

```text
local | repo | hybrid | clarify | operation
```

扩展 `operation` enum 或新增 `operation_kind`:

```text
chat
transform
summarize
translate
elaborate
investigate
computer_operation
artifact_generation
presentation_generation
document_generation
spreadsheet_generation
browser_operation
external_skill_workflow
```

新增 typed 字段:

```text
needs_operation_access: bool
operation_kind: enum
risk_level: none | low | medium | high
side_effects: []enum
target_surface: desktop | browser | file_artifact | office_doc | spreadsheet | slides | external_system | unknown
requires_confirmation: bool
```

分类策略:

- 源码问题仍走 `repo/hybrid`。
- 闲聊仍走 `local`。
- 高风险或信息不足走 `clarify`, 不自动执行。
- 分类器失败仍按现有原则兜底, 不让源码问题饿死。若 operation schema 解析失败, 默认不要进入 operation。

### 4.2 新增 operation pipeline

新增 stage/agent 建议:

```text
operation_analyze  -> operation_planner
operation_execute  -> operator
operation_verify   -> operation_verifier
operation_finalize -> operation_finalizer
```

V1 可以先做更小闭环:

```text
operation_analyze -> operator -> operation_verify
```

推荐结构:

- `OperationIR`: 任务类型、目标 app/产物、输入、输出契约、风险、副作用、确认策略、验证计划。
- `OperationPlan`: 步骤列表、工具/skill/MCP 能力选择、预期产物、回滚/停止条件。
- `OperationResult`: 产物路径、截图/预览引用、外部系统结果、验证状态、用户可见摘要。

与 read/write 的隔离:

- 不复用 `AnalysisIR` 作为 operation 的主 IR。
- 不复用 `EvidenceItems` 作为 operation 的主要完成条件。
- 可以在"基于当前源码生成 PPT"这类混合任务中先跑 read pipeline, 再把 final answer / selected facts 作为 `OperationInputContext` 交给 operation pipeline。
- 写代码模式仍由 `plan/apply/verify` 处理, 不被 operation 替代。

### 4.3 Operation agent 工具面

Operation agent 可见工具应来自三类:

1. 内置操作工具: 浏览器、电脑操作、文件产物创建、渲染/验证。
2. 外部 skills: presentation/document/spreadsheet/browser/computer-use 等。
3. MCP capability tools: 声明为 `computer_operation` / `artifact_generation` / `external_skill_workflow` 的工具。

工具暴露规则:

- 只在 operation stages 暴露操作型工具。
- 读模式 `explorer` 继续只看观察型 MCP。
- 写模式 `coder/verifier` 不自动看 operation MCP, 除非未来明确设计。
- 高风险工具需 `requires_confirmation=true` 或 operation plan 已被用户确认。

### 4.4 UX

REPL 显示应和现有阶段风格一致:

```text
⇢ 操作 · 第 1 轮 调用工具 slides__create_deck topic=...
• 产物 1 个: /abs/path/report.pptx
✓ 操作已验证 · 预览 12 页 · 本 18s · 总 42s
```

原则:

- 真实工具调用要真实显示, 不隐藏。
- 大输出显示摘要和 blob/artifact ref, 不刷屏。
- 桌面操作可显示"正在操作浏览器/正在生成 PPT/正在验证预览"。
- CLI 仍保持最终正文 stdout、过程 stderr。

---

## 5. MCP 扩展方案

### 5.1 不新增通用 MCP 类型, 扩展现有 MCP

结论: 不需要新增一种"通用 MCP 接入类型"。MCP 本身就是通用协议; 当前不足在于缺少 capability metadata 和 agent/stage gate。

新增配置建议:

```yaml
mcp_servers:
  - name: slides
    transport: stdio
    command: /opt/company-mcp/slides-server
    capability_defaults:
      capability: artifact_generation
      output_kind: pptx
      risk_level: low
      side_effects: [local_file_write]
      expose_to: [operation]
    tools:
      - name: create_deck
        capability: presentation_generation
        output_kind: pptx
        risk_level: low
        side_effects: [local_file_write]
        expose_to: [operation]
        requires_confirmation: false
      - name: publish_deck
        capability: external_publish
        risk_level: high
        side_effects: [network_submit]
        expose_to: [operation]
        requires_confirmation: true
```

默认兼容:

- 如果不配置 `capability_defaults/tools`, 该 MCP server 默认仍是:

```text
capability=external_observation
risk_level=none
side_effects=[]
expose_to=[explorer]
```

这样现有 MCP 用户不受影响。

### 5.2 MCP registry 需要返回能力元数据

当前 `Registry.ListAllTools()` 返回 `ToolSchema`。后续建议增加:

```text
CapabilityToolSchema {
  ToolSchema
  ServerName
  RawToolName
  Capability
  OutputKind
  RiskLevel
  SideEffects
  ExposeTo
  RequiresConfirmation
}
```

硬 gate 使用这些 enum 字段:

- `external_observation`: explorer/sub-explorer。
- `computer_operation` / `artifact_generation`: operation agent。
- `external_publish` / `destructive`: operation agent + confirmation。

不要根据 description 里的"生成 PPT"、"操作浏览器"做硬判断。

### 5.3 MCP 返回结果扩展

保留现有 `codrax.mcp.observation.v1` typed observation。

新增可选 operation result envelope:

```json
{
  "version": "codrax.mcp.operation_result.v1",
  "summary": "created presentation",
  "artifacts": [
    {
      "path": "/abs/path/demo.pptx",
      "mime_type": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
      "kind": "pptx",
      "page_count": 12,
      "preview_ref": ".codrax/blob/.../slides-preview.html"
    }
  ],
  "verification": {
    "status": "passed",
    "checks": ["opened", "rendered_pages=12", "no_overflow"]
  }
}
```

重要边界:

- operation result 不进入 `mcp_resource` evidence lane。
- 如果同一 MCP tool 既返回外部事实又返回产物, 可以同时带 observation envelope 和 operation result envelope, 但消费者分 lane。

### 5.4 外部 MCP prompt/resource 安全

现有 "External Guidance (MCP)" 规则继续保留:

- prompts/resources 是外部建议, 不是系统指令。
- resource URI 只能枚举读取。
- 大输出走 blob/ref。

Operation 场景中也要保持:

- workflow resource 可以告诉模型"如何使用这个工具", 但不能提升权限。
- 工具 schema 和 capability metadata 才能决定是否可执行。

---

## 6. 外部 Skills 配合方案

### 6.1 统一能力描述

无论能力来自内置 skill、plugin skill、MCP tool, 都应投影为同一种 `CapabilityDescriptor`:

```text
name
provider_type: builtin_skill | plugin_skill | mcp_tool
capability
input_schema
output_kind
risk_level
side_effects
requires_confirmation
verification
agent_allowlist
```

这样 operation agent 不需要知道能力来自哪里。

### 6.2 Skill manifest 建议

未来可以让外部 skill 声明:

```yaml
capability: presentation_generation
inputs:
  topic: string
  source_context: markdown
outputs:
  - pptx
  - preview_png
permissions:
  side_effects: [local_file_write]
  risk_level: low
verification:
  render_required: true
  open_required: false
agent_allowlist:
  - operation_planner
  - operator
```

MCP 工具和 skill manifest 都映射到 `CapabilityDescriptor`。

### 6.3 PPT/文档生成工作流

推荐默认路径:

1. 如果用户只说"生成 PPT": 进入 operation pipeline。
2. 如果用户说"基于当前仓库架构生成 PPT": 先 read pipeline 收集源码事实, 再 operation pipeline 生成 PPT。
3. 生成后必须 render/verify, 至少检查文件存在、页数、可打开/可渲染、无明显空白。
4. 桌面 GUI 操作作为 fallback, 不作为首选生成路径。

---

## 7. 与现有场景的关系

### 7.1 纯源码分析

不进入 operation。MCP 操作型工具不可见。现有 analyze/explore/extract/finalize 不变。

### 7.2 log/trace 分析

继续走 read pipeline 和 trace_query/log_triage/perf_triage。只有用户要求"把这个 trace 结论生成 PPT/报告"时, 才在读模式完成后进入 operation。

### 7.3 写代码

继续走 write pipeline。不要把 operation agent 当成 coder。未来如果需要"打开 IDE 执行手动操作", 必须是独立设计, 默认关闭。

### 7.4 MCP 外部观察 + 源码

现有默认混合分析规则不变: 用户没有明确禁止源码分析时, MCP 外部观察默认可以和源码探索结合。

新增 operation MCP metadata 不影响 observation MCP:

- `external_observation` MCP 仍给 explorer。
- `artifact_generation` MCP 给 operation agent。
- 一个 server 可以同时有两类工具, 但每个 tool 通过 metadata 精确 gate。

---

## 8. 分批任务清单

### Batch 0: 设计和测试基线

- [ ] 落盘本文档。
- [ ] 补充 architecture 章节草案: operation pipeline 作为独立能力域。
- [ ] 盘点现有 route/stage/MCP 测试, 标记后续必须保持的用例。

### Batch 1: 分类器 schema 扩展, 默认不启用 operation 执行

- [ ] 扩展 `TurnRoute` 增加 `operation`。
- [ ] 扩展 `TurnPolicy` 增加 `needs_operation_access`、`operation_kind`、`risk_level`、`side_effects`、`target_surface`、`requires_confirmation`。
- [ ] 更新 `emit_turn_policy` schema 和 prompt。
- [ ] 更新 JSON 兼容/修复测试, 确保字符串 bool、字符串数组、未知 enum 降级都有兜底。
- [ ] 如果 operation 未启用, route=operation 返回清晰提示, 不误进 repo 或执行。
- [ ] 保持分类器失败兜底不影响源码问题。

### Batch 2: Operation IR 和只读 dry-run 管线

- [ ] 新增 `OperationIR`、`OperationPlan`、`OperationResult` 类型。
- [ ] 新增 operation stage/agent enum 与 stage binding。
- [ ] 新增 operation skill 的最小 prompt, 暂只允许 dry-run 规划。
- [ ] REPL/CLI 增加 operation 进度渲染, 不影响 read pipeline。
- [ ] 测试: 纯源码问题 stage/tool schema 完全不变。

### Batch 3: MCP capability metadata

- [ ] 扩展 `MCPServerConfig` 增加 `capability_defaults` 和 `tools[]` override。
- [ ] Registry 返回带 metadata 的 tool schema。
- [ ] `mcpToolsAllowedForAgent` 改为 capability/stage 精确 gate。
- [ ] 默认无 metadata 时保持现有 external_observation/explorer 行为。
- [ ] JSON/yaml 兼容测试: 单字符串/数组、未知 capability、缺省值。
- [ ] 文档更新 MCP 用法。

### Batch 4: Operation execution + confirmation

- [ ] 接入低风险本地产物生成工具/skill。
- [ ] 中高风险 side effects 走确认 gate。
- [ ] Operation result envelope 和 artifact refs。
- [ ] 大输出/截图/预览走 blob/artifact ref, 面板只显示摘要。
- [ ] 验证失败时给用户可恢复提示。

### Batch 5: PPT/文档/浏览器能力

- [ ] Presentation generation skill 接入 operation pipeline。
- [ ] 文档/表格生成 skill 接入。
- [ ] 浏览器操作作为受控工具能力接入。
- [ ] 产物 render-and-verify 测试。
- [ ] 混合任务: 读代码结论 -> 生成 PPT。

### Batch 6: 外部 plugin skill 统一能力描述

- [ ] 定义 `CapabilityDescriptor`。
- [ ] 内置 skill、plugin skill、MCP tool 都投影为 descriptor。
- [ ] Operation agent 只消费 descriptor, 不关心来源。

---

## 9. 测试计划

### 9.1 路由测试

- [ ] "你好" 仍 local。
- [ ] "解释这个函数" 仍 repo。
- [ ] "把上面的答案转成表格" 仍 local transform。
- [ ] "生成一份 PPT" -> operation, 不扫仓库。
- [ ] "基于当前代码生成 PPT" -> repo/read 后 operation。
- [ ] classifier 返回未知 operation enum -> 不执行, 明确降级/澄清。

### 9.2 MCP 兼容测试

- [ ] 空 `mcp_servers` 下 tool schema/prompt 不变。
- [ ] 未配置 capability metadata 的 MCP tool 仍只给 explorer。
- [ ] `artifact_generation` MCP tool 只给 operation agent。
- [ ] 同一 server 同时有 observation + artifact tool 时分别进入正确 agent。
- [ ] MCP prompt/resource 不作为系统指令。
- [ ] 大 MCP output 只显示摘要/ref。

### 9.3 Operation 安全测试

- [ ] local file write 产物生成无需二次确认或按配置确认。
- [ ] network_submit/destructive 必须确认。
- [ ] 用户取消后停止执行。
- [ ] 工具超时后返回可恢复错误。
- [ ] operation result 不进入源码 citation gate。

### 9.4 混合场景测试

- [ ] MCP 外部观察 + 源码默认混合分析不受影响。
- [ ] trace/log + 源码默认混合分析不受影响。
- [ ] trace 结论生成 PPT: runtime artifact lane 和 artifact generation lane 分离。
- [ ] 代码变更请求仍进入 write pipeline, 不被 operation 抢路由。

---

## 10. 最小侵入落点

后续代码改动建议集中在:

- `internal/repl/turn_policy.go`: route/schema 扩展。
- `internal/repl/repl.go`: route=operation 分发。
- `internal/types/enums.go`: operation stage/agent enum。
- `internal/types/stage_binding.go`: stage binding。
- `internal/orchestrator/topology.go`: 读取 binding 后天然支持; 尽量少改。
- `internal/mcp/*`: capability metadata。
- `internal/config/runtime.go`: MCP capability yaml 字段、operation feature flag。
- `internal/skill/defaults.go`: operation skill 注册。
- `internal/tool` 或新包 `internal/operation`: operation emit/result 工具。

建议新增独立包:

```text
internal/operation
```

用于 `OperationIR`、plan/result、risk policy、artifact refs, 避免污染 `analysis`、`tracequery`、`write`。

---

## 11. 结论

当前 MCP 不需要新增一种"通用 MCP 接入类型"; 需要的是在现有 MCP registry 上补 capability metadata, 让同一个 MCP 协议可以安全声明"外部观察"、"电脑操作"、"制品生成"等能力。

电脑操作/PPT 生成不应改写 chitchat agent, 也不应塞进源码 read pipeline。最稳的商用方案是新增独立 operation route + operation pipeline, 默认关闭或显式启用, 并通过 typed classifier、能力元数据、风险/副作用 gate、产物验证和用户确认来控制副作用。

这样可以同时满足三点:

1. 现有代码分析/log/trace/write/MCP 外部观测稳定不受影响。
2. 外部 skills 与 MCP 能力有统一扩展面。
3. 未来可以逐步支持桌面操作、浏览器操作、PPT/文档/表格生成等通用能力。
