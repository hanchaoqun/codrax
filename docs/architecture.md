# 架构设计文档

## 目录

- [1. 概述](#1-概述)
- [2. 层级架构](#2-层级架构)
- [3. 组件详情](#3-组件详情)
  - [3.1 编排器（第 1 层）](#31-编排器第-1-层--编排层)
  - [3.2 Agent（第 2 层）](#32-agent第-2-层--执行层)
  - [3.3 技能（第 3 层）](#33-技能第-3-层--策略层)
  - [3.4 工具（第 4a 层）](#34-工具第-4a-层--能力层)
  - [3.5 MCP（第 4b 层）](#35-mcp第-4b-层--能力层)
  - [3.6 LLM（第 5 层）](#36-llm第-5-层--智能层)
- [4. 阶段规范](#4-阶段规范)
- [5. 数据结构](#5-数据结构)
- [6. 请求生命周期](#6-请求生命周期)
- [7. 编排器状态机](#7-编排器状态机)
- [8. 关键设计模式](#8-关键设计模式)
- [9. 错误处理与容错](#9-错误处理与容错)
- [10. 可扩展性](#10-可扩展性)

---

## 1. 概述

### 系统目标与设计哲学

本系统是一个多层 AI Agent 架构，旨在将复杂的软件工程任务分解为结构化、可审查、可验证的阶段。系统不依赖单次 LLM 调用，而是将关注点分离到五个层级：

- **编排层** — *做什么*以及*谁来做*
- **执行层** — 通过专业 Agent *执行*工作
- **策略层** — *如何*完成工作（工作流、约束、输出格式）
- **能力层** — *有哪些工具*可用（本地和远程）
- **智能层** — 通过 LLM 进行*推理*和*生成*

**一句话总结：** 编排器决定"谁做什么"，Agent"执行"，技能定义"怎么做"，工具/MCP 提供"用什么"，LLM 是"大脑"。

### 系统概览图

```mermaid
graph TB
    User([用户请求])

    subgraph "第 1 层 — 编排层"
        Orch[编排器<br/>状态机 + YAML 配置]
    end

    subgraph "第 2 层 — 执行层"
        A1[规划器]
        A2[探索器]
        A3[实现器]
        A4[设计审查器]
        A5[代码审查器]
        A6[验证器]
        A7[终结器]
    end

    subgraph "第 3 层 — 策略层"
        S1[任务分析技能]
        S2[仓库探索技能]
        S3[CLI 分析技能]
        S4[实现规划技能]
        S5[代码实现技能]
        S6[设计审查技能]
        S7[代码审查技能]
        S8[验证技能]
        S9[最终回答技能]
    end

    subgraph "第 4 层 — 能力层"
        T[工具<br/>exec, grep, read_file,<br/>repo_map, run_tests ...]
        M[MCP 服务器<br/>GitHub, DB, Notion,<br/>Slack, Browser, API ...]
    end

    subgraph "第 5 层 — 智能层"
        LLM[LLM<br/>推理 + 生成]
    end

    User --> Orch
    Orch -->|调度| A1 & A2 & A3 & A4 & A5 & A6 & A7
    A1 ---|配备| S1 & S4
    A2 ---|配备| S2 & S3
    A3 ---|配备| S5
    A4 ---|配备| S6
    A5 ---|配备| S7
    A6 ---|配备| S8
    A7 ---|配备| S9
    A1 & A2 & A3 & A4 & A5 & A6 & A7 -->|调用| T & M
    A1 & A2 & A3 & A4 & A5 & A6 & A7 -->|调用| LLM
```

---

## 2. 层级架构

### 5 层层级结构

```mermaid
graph LR
    L1["[1] 编排层<br/>编排器"]
    L2["[2] 执行层<br/>Agent × 7"]
    L3["[3] 策略层<br/>技能 × 9"]
    L4["[4] 能力层<br/>工具 + MCP"]
    L5["[5] 智能层<br/>LLM"]

    L1 --> L2 --> L3
    L2 --> L4
    L2 --> L5
```

### 层间通信规则

| 从 → 到 | 是否允许 | 机制 |
|----------|----------|------|
| 第 1 层 → 第 2 层 | 是 | 编排器通过 AgentContext 调度 Agent |
| 第 2 层 → 第 1 层 | 是 | Agent 返回结果；编排器读取更新后的 BusContext |
| 第 2 层 → 第 3 层 | 是 | Agent 加载技能配置来指导其行为 |
| 第 2 层 → 第 4 层 | 是 | Agent 在执行过程中调用工具和 MCP 服务器 |
| 第 2 层 → 第 5 层 | 是 | Agent 调用 LLM 进行推理和生成 |
| 第 3 层 → 第 4 层 | 否 | 技能只*建议*工具；Agent 负责调用 |
| 第 4 层 → 第 5 层 | 否 | 工具不直接调用 LLM |
| 第 1 层 → 第 3/4/5 层 | 否 | 编排器不绕过 Agent |

**关键规则：** 所有工具调用和 LLM 调用都通过**第 2 层（Agent）**进行。编排器（第 1 层）永远不直接调用工具、MCP 或 LLM。技能（第 3 层）是配置，不是执行者。

---

## 3. 组件详情

### 3.1 编排器（第 1 层 — 编排层）

#### 职责

- **Agent 选择** — 根据当前流水线阶段选择要调度的 Agent
- **流水线阶段控制** — 管理状态机中的阶段推进
- **技能选择** — 为 Agent 分配适当的技能（YAML 默认值，可在运行时覆盖)
- **子 Agent 派生** — 在并行有益时派生并发子 Agent
- **执行状态管理** — 维护 BusContext 和 TaskList 作为共享状态
- **终止决策** — 任务循环穷尽 + 全局 max-steps 守护

#### 两阶段执行模型

`Run()` 分两个阶段:

1. **Phase 1 — analyze 阶段**: 调度 `analyze` 一次。analyzer 通过 `todo_write` 工具(或 fail-safe 路径)向 `BusContext.Mutable.TaskList` 写入分解后的任务列表
2. **Phase 2 — 任务循环**: 遍历所有 pending 的 task,为每个 task 跑一次 mini-pipeline(`explore → plan → implement → verify → finalize`),由该 task 的 `Writing`/`HighRisk` 决定走哪条 policy

每个 task 进入循环时**重置 per-task state**:
- `Signals` 清空
- `MissingPiece` 重置为 `MissingFacts`
- `PipelineStage` 重置为 `StageExplore`
- `stageVisits` 计数器清零

每个 task **共享的状态**(跨 task 累积):
- `RepoFacts / ToolResults / MCPResponses` — 探索成果可被后续 task 利用
- `Mutable.TaskList` — 工作面板,工具更新对所有 task 可见

每个 task 的 `finalize` stage 把它的答案写入该 task 的 `Result` 字段(通过 `Mutable.UpdateTaskResult`),并标记 `Status = TaskDone`。`main.go` 渲染时遍历 `Tasks` 展示每个任务的独立结果。

`maxSteps` 是**全 Run 的总预算**,跨 phase 1 和 phase 2;`MaxStageVisits` 振荡守卫是 **per-task 计数**——每进入一个新 task 就清零。

#### YAML 驱动的状态机

编排器完全通过 [`config/orchestrator.yaml`](../config/orchestrator.yaml) 配置。配置定义了：

- **8 个阶段**，带有默认 Agent 和技能绑定
- **优先级加权的转换规则**
- **3 种任务策略**，约束哪些阶段处于活动状态
- **功能开关**，用于运行时行为切换

#### 阶段定义

| 阶段 | 默认 Agent | 默认技能 | 是否终止 | 需要写权限 |
|------|-----------|----------|----------|-----------|
| `analyze` | 分析器 | task-analysis-skill | 否 | 否 |
| `explore` | 探索器 | repo-explore-skill | 否 | 否 |
| `plan` | 规划器 | implementation-plan-skill | 否 | 否 |
| `design_review` | 设计审查器 | design-review-skill | 否 | 否 |
| `implement` | 实现器 | code-implement-skill | 否 | 是 |
| `code_review` | 代码审查器 | code-review-skill | 否 | 否 |
| `verify` | 验证器 | verification-skill | 否 | 否 |
| `finalize` | 终结器 | final-answer-skill | **是** | 否 |

> **注意：** `design_review` 和 `code_review` 是两个独立的阶段，分别由独立的 Agent 负责。`design_review` 在 `plan` 之后审查方案可行性，`code_review` 在 `implement` 之后审查代码正确性。每个阶段产出独立的通过/失败信号（`DesignReviewPassed` / `CodeReviewPassed`）。

#### 转换引擎

转换引擎遵循确定性评估流程：

1. **枚举**当前阶段的所有出口转换
2. **按任务策略过滤** — 移除策略不允许的阶段转换
3. **按功能开关过滤** — 移除已禁用阶段的转换（如 `enable_verify: false` 时的 `verify`）
4. **评估**运行时条件 — 检查 `TaskState.Missing`、`ExecutionSignals` 和阶段结果
5. **选择**最高优先级的有效转换

#### 任务策略系统

任务策略为不同任务类型定义阶段子集：

| 策略 | 允许的阶段 |
|------|-----------|
| `analysis` | analyze → explore → finalize |
| `implementation` | analyze → explore → plan → implement → verify → finalize |
| `high_risk_implementation` | analyze → explore → plan → **design_review** → implement → **code_review** → verify → finalize |

#### 功能开关

| 开关 | 默认值 | 效果 |
|------|--------|------|
| `enable_verify` | `true` | 全局启用/禁用验证阶段 |
| `require_review` | `true` | 为 true 时使用包含 design_review 和 code_review 阶段的 high_risk_implementation 策略 |
| `allow_skip_plan_for_small_change` | `false` | 允许小改动从 explore 直接跳到 implement |

#### 终止检测

整个 `Run()` 在任务队列被清空(所有 task 进入 `Done` / `Failed`)或全局 `maxSteps` 预算用尽时终止。**单个 task 的 mini-pipeline** 在到达 `finalize` 阶段(`terminal: true`)时自然结束——finalize 只是 per-task terminal,不是整个 Run 的 terminal。如果 per-task 循环在到达 finalize 之前就耗尽了预算或被振荡守卫打断,orchestrator 会强制跑一次 finalize 兜底,把 LastError 写到 `task.Result` 里,然后继续处理下一个 task。

#### 决策函数(伪代码)

以下是 per-task mini-pipeline 内部的 stage 转移逻辑,`runTaskPipeline` 在每一步调用:

```
function decide_next_stage(bus_context):
    current = bus_context.TaskState.Stage
    missing = bus_context.TaskState.Missing
    policy  = get_active_policy(bus_context.TaskList)
    flags   = load_feature_flags()

    transitions = get_transitions(current)                  // 从 YAML 获取
    transitions = filter_by_policy(transitions, policy)     // 移除不允许的阶段
    transitions = filter_by_flags(transitions, flags)       // 移除已禁用的阶段
    transitions = filter_by_signals(transitions, bus_context.Signals)  // 运行时条件
    transitions = sort_by_priority(transitions, descending)

    if transitions is empty:
        return "finalize"   // 兜底：无有效转换 → 结束

    return transitions[0].to
```

---

### 3.2 Agent（第 2 层 — 执行层）

#### 职责

- 接收提示词（由 AgentContext + PromptContext 组装）
- 调用 LLM 进行推理和生成
- 使用工具和 MCP 服务器与环境交互
- 维护 ReAct 循环（推理 → 行动 → 观察）直到阶段目标达成
- 产出结构化输出，更新 BusContext

#### 生命周期

```
init → receive_prompt → execute_loop → complete
```

1. **init** — Agent 使用其 AgentContext 实例化
2. **receive_prompt** — 从 AgentContext + 技能配置组装 PromptContext
3. **execute_loop** — ReAct 循环：LLM 推理 → 选择工具 → 执行 → 观察结果 → 重复
4. **complete** — Agent 产出最终输出，编排器更新 BusContext

#### Agent 类型

| Agent | 阶段 | 能力 | 描述 |
|-------|------|------|------|
| `analyzer`（分析器） | analyze | 只读 | 结构化任务并通过 todo_write 标记 writing / high_risk |
| `planner`（规划器） | plan | 只读 | 设计实现方案 |
| `explorer`（探索器） | explore | 只读 | 浏览代码库，收集事实，构建模块映射 |
| `implementer`（实现器） | implement | **读 + 写** | 编写代码，修改文件，生成补丁 |
| `design_reviewer`（设计审查器） | design_review | 只读 | 审查方案可行性、架构影响、风险 |
| `code_reviewer`（代码审查器） | code_review | 只读 | 审查代码正确性、缺陷、风格、副作用 |
| `verifier`（验证器） | verify | 只读 | 运行测试、lint、构建检查 |
| `finalizer`（终结器） | finalize | 只读 | 汇总结果，产出最终输出 |

#### Agent 执行循环（ReAct）

```mermaid
graph TD
    Start([Agent 接收提示词]) --> Think[LLM 推理<br/>下一步行动]
    Think --> Decide{需要<br/>行动？}
    Decide -->|是| Act[选择并调用<br/>工具或 MCP]
    Act --> Observe[处理<br/>工具结果]
    Observe --> Update[更新工作<br/>状态]
    Update --> Think
    Decide -->|否，目标达成| Output[产出<br/>结构化输出]
    Output --> Done([返回编排器])
```

#### 子 Agent 并发

编排器可以在任务相互独立时并行派生多个子 Agent。每个子 Agent 在自己的 AgentContext 切片上操作。结果在完成时合并回 BusContext。父 Agent（或编排器）等待所有子 Agent 完成后再继续。

---

### 3.3 技能（第 3 层 — 策略层）

#### 职责

- 定义特定类型工作的**工作流步骤**
- 建议**使用哪些工具**（但不调用它们）
- 指定**输出格式**和模式
- 明确声明**阶段目标**
- 声明**禁止事项**（Agent 不得做的事情）

#### 技能清单

| 技能 | 阶段 | 用途 |
|------|------|------|
| `task-analysis-skill` | analyze | 结构化和分类用户任务 |
| `repo-explore-skill` | explore | 浏览代码库，收集事实，构建模块映射 |
| `implementation-plan-skill` | plan | 设计分步实现方案 |
| `code-implement-skill` | implement | 编写/修改代码，生成补丁 |
| `design-review-skill` | design_review | 审查方案可行性、架构影响、风险 |
| `code-review-skill` | code_review | 审查代码正确性、缺陷、风格、副作用 |
| `verification-skill` | verify | 运行测试、lint、构建、验证正确性 |
| `final-answer-skill` | finalize | 产出面向用户的最终输出 |

#### 技能定义模式

```go
type SkillConfig struct {
    Name            string   // 唯一标识符，如 "repo-explore-skill"
    Goal            string   // 此技能的目标
    Workflow        []string // Agent 应遵循的有序步骤
    ToolSuggestions []string // 推荐的工具（Agent 决定是否使用）
    OutputFormat    string   // 期望的输出结构（JSON Schema 或描述）
    Prohibitions    []string // Agent 在此技能下不得做的事情
}
```

#### 技能选择逻辑

1. **默认**：每个阶段在 YAML 配置中有一个 `default_skill`（如 `design_review` 阶段默认使用 `design-review-skill`，`code_review` 阶段默认使用 `code-review-skill`）
2. **运行时覆盖**：编排器可以根据任务特定信号覆盖技能

---

### 3.4 工具（第 4a 层 — 能力层）

#### 职责

在宿主环境上执行本地、确定性的操作。

#### 工具接口

```go
type Tool interface {
    Name()        string
    Description() string
    Parameters()  json.RawMessage  // JSON Schema
    Execute(ctx *types.BusContext, params json.RawMessage) (ToolResult, error)
    IsWrite()     bool  // 是否需要文件系统写权限(对齐 requires_write 边界)
}
```

工具通过嵌入 `tool.ReadOnly` 或 `tool.WriteCapable` mixin 来满足 `IsWrite()`。Execute 收到的 `*BusContext` 是窄视图:只有 `RepoRoot`/`Branch`/`Commit` 和 `Mutable` 区域被填充,其他字段为零值,从而物理上隔离工具只能改可变域。

#### 内置工具

| 工具 | 读/写 | 描述 |
|------|------|------|
| `exec_command` | 读 | 执行 shell 命令(dual-use,当前按 read-only 处理,shell 级的写限制靠外部沙箱) |
| `grep` | 读 | 按模式搜索文件内容 |
| `read_file` | 读 | 读取文件内容 |
| `list_files` | 读 | 列出目录中的文件 |
| `repo_map` | 读 | 生成仓库结构映射 |
| `run_tests` | 读 | 执行测试套件(按意图分类,verifier 依赖) |
| `git_diff` | 读 | 显示 git diff 输出 |
| `git_log` | 读 | 显示 git 提交历史 |
| `apply_patch` | **写** | 对文件做目标化改动(write/edit/insert_before/insert_after 四种模式),保留行尾,限定在 workspace 根内,返回 diff 预览 |
| `todo_write` | 读(逻辑上写 `Mutable`) | 在 `BusContext.Mutable.TaskList` 上做**全量替换**式更新,analyzer / explorer / implementer 的 skill 推荐它。sub-agent 不共享 `Mutable`,调用会被拒 |
| `propose_sub_agents` | 读 | **自动注入**给名字匹配已注册 sub-agent 的主 agent,用于在 ReAct 循环里申请派生并行 sub-agent,schema 的 `sub_agent` enum 会被收窄到该主 agent 对应的唯一名字 |

#### ToolResult 格式

```go
type ToolResult struct {
    ToolName  string
    Summary   string    // 人类可读的摘要
    RawRef    string    // 原始输出的引用（如文件路径或键）
    Success   bool
    Timestamp time.Time
}
```

---

### 3.5 MCP（第 4b 层 — 能力层）

#### 什么是 MCP？

**MCP（模型上下文协议）** 是一种将外部系统连接到 LLM 的标准化协议。MCP 服务器暴露能力（工具、资源、提示词），Agent 可以在运行时发现和调用它们。

#### 职责

- 将外部服务桥接到 Agent 的工具集中
- 提供可用能力的运行时发现
- 处理认证、传输和序列化

#### MCP 服务器接口

```go
type MCPServer interface {
    Name()        string
    Transport()   TransportType   // stdio | sse | http
    ListTools()   []ToolSchema
    CallTool(name string, params json.RawMessage) (MCPResponse, error)
}
```

#### 典型 MCP 服务器

| 服务器 | 用途 | 示例操作 |
|--------|------|----------|
| GitHub | 仓库操作 | 创建 PR、读取 issue、审查评论 |
| Database | 数据访问 | 查询表、检查模式 |
| Notion | 文档管理 | 读写页面、搜索工作区 |
| Slack | 通信 | 发送消息、读取频道 |
| Browser | Web 交互 | 获取页面、截图、交互 |
| API | 通用 HTTP | 调用任意 REST/GraphQL 端点 |

#### 传输层

| 传输方式 | 使用场景 | 协议 |
|----------|----------|------|
| `stdio` | 本地进程 | stdin/stdout JSON-RPC |
| `sse` | 远程服务器（流式） | HTTP Server-Sent Events |
| `http` | 远程服务器（请求/响应） | HTTP POST JSON-RPC |

#### MCP 与本地工具对比

| 方面 | 工具（本地） | MCP（外部） |
|------|-------------|-------------|
| 执行 | 进程内或本地子进程 | 远程服务器 |
| 发现 | 构建时硬编码 | 通过 `ListTools()` 运行时发现 |
| 延迟 | 低 | 可变（取决于网络） |
| 状态 | 无状态 | 可能有状态（服务端） |
| 认证 | 无（本地） | Token/OAuth/API key |

#### MCPResponse 格式

```go
type MCPResponse struct {
    ServerName string
    Method     string
    Summary    string
    RawRef     string
    Success    bool
    Timestamp  time.Time
}
```

---

### 3.6 LLM（第 5 层 — 智能层）

#### 职责

- **推理** — 分析上下文，规划行动，评估权衡
- **决策** — 选择调用哪个工具，何时停止，产出什么
- **文本生成** — 生成代码、方案、审查意见、摘要和最终回答

#### LLM 适配器接口

```go
type LLMAdapter interface {
    // Chat 发送提示词并返回模型的响应。
    // messages: 对话历史（system + developer + user 消息）
    // tools: 可用于函数调用的工具模式
    Chat(messages []Message, tools []ToolSchema) (LLMResponse, error)

    // ModelID 返回当前模型标识符。
    ModelID() string

    // MaxContextTokens 返回模型的上下文窗口大小。
    MaxContextTokens() int
}

type Message struct {
    Role    string // "system" | "developer" | "user" | "assistant" | "tool"
    Content string
}

type LLMResponse struct {
    Content    string
    ToolCalls  []ToolCall
    StopReason string
    Usage      TokenUsage
}
```

#### 模型选择与降级

系统支持多个 LLM 后端，具有降级链：

1. **主模型** — 高能力模型，用于复杂推理（如 Claude Opus）
2. **快速模型** — 低延迟模型，用于简单任务（如 Claude Haiku）
3. **降级** — 如果主模型不可用，降级到下一级

模型选择由 Agent 类型和任务复杂度决定。编排器可以通过配置按阶段覆盖模型。

#### 上下文窗口管理

PromptContext 的组装具有 token 预算意识：

1. **系统段** — 始终包含（最高优先级）
2. **开发者段** — 技能指令、约束条件
3. **用户段** — 任务详情、事实、历史
4. **工具结果** — 压缩以适应预算

当上下文超过窗口时，较旧的工具结果和事实按优先级压缩或丢弃。

---

## 4. 阶段规范

### 4.1 analyze — 任务理解

> **核心：** 将"用户输入"转化为"可执行的任务定义"

| 方面 | 详情 |
|------|------|
| **Agent** | analyzer |
| **技能** | task-analysis-skill |
| **输入** | 用户原始输入、当前 BusContext（可能为空）、对话历史（可选） |
| **工作** | 意图识别、任务分解、通过 `todo_write` 工具产出任务列表(每个任务携带 Writing / HighRisk 两个布尔),提取约束 |
| **输出** | 调用 `todo_write` 写入 `BusContext.Mutable.TaskList`;并返回自然语言分类说明 |

### 4.2 explore — 事实收集

> **核心：** 构建"可信的事实基础"

| 方面 | 详情 |
|------|------|
| **Agent** | 探索器 |
| **技能** | repo-explore-skill |
| **输入** | 当前 TaskList / 活动任务、仓库路径 / 分支、已知事实（可能为空） |
| **工作** | 查找代码入口点（main / cmd / router）、搜索函数和配置、构建模块映射、分析调用链、查询文档 / MCP（GitHub / Docs / DB） |
| **输出** | `{ repo_facts, entrypoints, call_chains, relevant_files }` |

### 4.3 plan — 方案设计

> **核心：** 将"做什么"转化为"怎么做"

| 方面 | 详情 |
|------|------|
| **Agent** | 规划器 |
| **技能** | implementation-plan-skill |
| **输入** | 仓库事实、当前 TaskList、约束条件 |
| **工作** | 设计修改方案、确定需要更改的文件、定义补丁结构、评估影响范围、定义验证方法 |
| **输出** | `{ plan: { files_to_modify, steps, risks, validation } }` |

### 4.4 design_review — 设计审查

> **核心：** 判定方案"是否可行且合理"

| 方面 | 详情 |
|------|------|
| **Agent** | 设计审查器（design_reviewer） |
| **技能** | design-review-skill |
| **输入** | 方案、仓库事实、约束条件 |
| **工作** | 检查需求对齐、检查架构完整性、检查边界情况、检查安全/性能风险、检查遗漏步骤 |
| **输出** | `{ review_result: pass/fail, issues, must_fix, suggestions }`；设置信号 `DesignReviewPassed` |

### 4.5 implement — 实现

> **核心：** 将方案转化为实际的代码/配置更改

| 方面 | 详情 |
|------|------|
| **Agent** | 实现器 |
| **技能** | code-implement-skill |
| **输入** | 方案、仓库事实、约束条件 |
| **工作** | 编写代码、修改配置、生成补丁/diff、调用工具（exec / file write / MCP） |
| **输出** | `{ patch, modified_files, implementation_notes }` |

### 4.6 code_review — 代码审查

> **核心：** 判定代码"是否正确且无隐患"

| 方面 | 详情 |
|------|------|
| **Agent** | 代码审查器（code_reviewer） |
| **技能** | code-review-skill |
| **输入** | 补丁、原始方案、仓库事实 |
| **工作** | 检查方案符合度、检查缺陷/边界情况、检查代码风格、检查副作用、检查兼容性 |
| **输出** | `{ review_result: pass/fail, code_issues, must_fix }`；设置信号 `CodeReviewPassed` |

### 4.7 verify — 验证

> **核心：** 证明"这个东西确实能用"

| 方面 | 详情 |
|------|------|
| **Agent** | 验证器 |
| **技能** | verification-skill |
| **输入** | 补丁、验证计划（来自 plan 阶段） |
| **工作** | 编译（go build）、单元测试、lint、冒烟测试、运行时验证 |
| **输出** | `{ verification_result: pass/fail, logs, errors, next_action: fix/explore }` |

### 4.8 finalize — 输出收敛

> **核心：** 将所有结果汇编为"用户可用的最终输出"

| 方面 | 详情 |
|------|------|
| **Agent** | 终结器 |
| **技能** | final-answer-skill |
| **输入** | 所有阶段产物、TaskList、验证结果 |
| **工作** | 总结变更、生成最终描述、输出补丁/使用说明、更新 BusContext、标记任务完成 |
| **输出** | 最终回答（代码 + 描述 + 操作步骤） |

---

## 5. 数据结构

### 设计原则

> **BusContext 不是模型上下文本身。** 它是用于构建 Agent 专属模型上下文的运行时事实源。流程如下：
>
> `BusContext`（完整共享状态） → 裁剪 → `AgentContext`（Agent 范围视图） → 组装 → `PromptContext`（模型提示词载荷） → 发送至 LLM

### 枚举类型

定义在 Go `runtime` 包中：

```go
type PipelineStage string
const (
    StageAnalyze      PipelineStage = "analyze"
    StageExplore      PipelineStage = "explore"
    StagePlan         PipelineStage = "plan"
    StageDesignReview PipelineStage = "design_review"
    StageImplement    PipelineStage = "implement"
    StageCodeReview   PipelineStage = "code_review"
    StageVerify       PipelineStage = "verify"
    StageFinalize     PipelineStage = "finalize"
)

type AgentName string
const (
    AgentAnalyzer       AgentName = "analyzer"
    AgentPlanner        AgentName = "planner"
    AgentExplorer       AgentName = "explorer"
    AgentDesignReviewer AgentName = "design_reviewer"
    AgentCodeReviewer   AgentName = "code_reviewer"
    AgentImplementer    AgentName = "implementer"
    AgentVerifier       AgentName = "verifier"
    AgentFinalizer      AgentName = "finalizer"
)

type TaskStatus string
const (
    TaskPending    TaskStatus = "pending"
    TaskInProgress TaskStatus = "in_progress"
    TaskDone       TaskStatus = "done"
    TaskBlocked    TaskStatus = "blocked"
    TaskFailed     TaskStatus = "failed"
)

// TaskItem 的能力/风险通过两个正交布尔表达:
// - Writing=false: 只读任务,走 analysis policy
// - Writing=true:  可写任务,走 implementation policy
// - HighRisk=true: Writing 任务进一步升级到 high_risk_implementation
//                  policy(要求 design_review + code_review)
```

### 三层上下文模型

```
BusContext（完整共享状态）
    ↓ 裁剪
AgentContext（Agent 范围视图）
    ↓ 组装
PromptContext（模型提示词载荷）
    ↓ 发送
LLM
```

#### 第 1 层：BusContext — 完整共享状态

```go
type BusContext struct {
    // 工具可写域 — 唯一可被工具直接 mutate 的区域
    // 通过指针共享，包含工作中的 task list
    Mutable *MutableState

    // 顶层任务状态
    TaskState TaskState

    // 当前运行时状态
    PipelineStage PipelineStage
    ActiveAgent   AgentName

    // 仓库/环境信息
    RepoRoot  string
    Branch    string
    Commit    string
    ModuleMap []string

    // 共享事实
    RepoFacts    []RepoFact
    ToolResults  []ToolResult
    MCPResponses []MCPResponse

    // 信号与策略
    Signals ExecutionSignals
    Policy  PolicyContext

    // 通用约束
    Constraints []string
    Preferences []string

    // 故障/恢复信息
    LastTransitionReason string
    TraceID              string
}

// MutableState 是 BusContext 中唯一允许工具(如 todo_write)直接
// mutate 的子区域。Sub-agent 不共享这个区域 —— BuildSubAgentContext
// 故意把 ac.Mutable 留成 nil,todo_write 在 sub-agent 环境下会拒绝调用。
// 内置 RWMutex 保护并发访问;调用方必须通过下面这组公开方法而非
// 直接访问私有字段。
type MutableState struct {
    // 受 mu 保护
    taskList TaskList
}

// 公开 API(都是 goroutine-safe):
//   TaskList() TaskList                              // 读取快照
//   SetTaskList(TaskList)                            // 整体替换
//   UpdateTaskStatus(id, status)                     // 单个任务状态
//   UpdateTaskResult(id, result, status)             // 单个任务的 Result + Status
//   SetCurrentTask(id)                               // 切换 current 指针
```

#### 第 2 层：AgentContext — Agent 范围视图

```go
type AgentContext struct {
    AgentName AgentName
    Stage     PipelineStage

    // 任务视图
    Objective           string
    CurrentTaskID       string
    CurrentTask         string
    CurrentTaskWriting  bool
    CurrentTaskHighRisk bool

    // 与此 Agent 相关的裁剪结果
    RelevantFacts         []string
    RelevantFiles         []string
    RelevantToolSummaries []string
    RelevantMCPNotes      []string

    // 前序阶段的摘要
    PlanSummary         string
    PatchSummary        string
    ReviewSummary       string
    VerificationSummary string

    // 控制信息
    Constraints []string
    Preferences []string

    // 当前缺口
    MissingPiece MissingPiece

    // 源信息
    RepoRoot string
    Branch   string
    Commit   string
}
```

#### 第 3 层：PromptContext — 模型提示词载荷

```go
type PromptSection struct {
    Title   string
    Content string
}

type PromptContext struct {
    SystemSections    []PromptSection
    DeveloperSections []PromptSection
    UserSections      []PromptSection

    // 模型可用的工具模式
    EnabledTools []string

    // 调试用元数据
    AgentName AgentName
    Stage     PipelineStage
    SkillName string
}
```

### 辅助数据结构

#### TaskItem / TaskList

```go
type TaskItem struct {
    ID          string
    Title       string
    Description string

    // 能力/风险 — 两个正交布尔
    Writing  bool  // 是否可能修改文件(对应 requires_write 层级)
    HighRisk bool  // 是否需要 design/code review

    Status TaskStatus
    Result string  // 每个 task 由其 finalize 阶段填充
}

type TaskList struct {
    Objective     string
    Tasks         []TaskItem
    CurrentTaskID string
}
```

`TaskList.CurrentTask()` 返回当前正在执行的 `TaskItem`。

#### MissingPiece / TaskState

编排器的核心决策输入 — "我们在哪 + 缺什么"。

```go
type MissingPiece string
const (
    MissingNone          MissingPiece = "none"
    MissingUnderstanding MissingPiece = "understanding"
    MissingFacts         MissingPiece = "facts"
    MissingPlan          MissingPiece = "plan"
    MissingCode          MissingPiece = "code"
    MissingReview        MissingPiece = "review"
    MissingVerification  MissingPiece = "verification"
)

type TaskState struct {
    Stage        PipelineStage
    Missing      MissingPiece
    Completed    []string
    Remaining    []string
    LastDecision string
    LastError    string
    IsTerminal   bool
}
```

`Missing` 字段驱动编排器的转换决策：根据缺失的内容选择下一个阶段。

#### RepoFact

```go
type RepoFact struct {
    Key        string
    Value      string
    Source     string
    Confidence float64
}
```

#### ToolResult / MCPResponse

```go
type ToolResult struct {
    ToolName  string
    Summary   string
    RawRef    string
    Success   bool
    Timestamp time.Time
}

type MCPResponse struct {
    ServerName string
    Method     string
    Summary    string
    RawRef     string
    Success    bool
    Timestamp  time.Time
}
```

#### ExecutionSignals

BusContext 中的布尔标志，驱动编排器决策。

```go
type ExecutionSignals struct {
    HasEnoughFacts       bool
    HasPlan              bool
    HasPatch             bool
    DesignReviewPassed   bool   // 由 design_reviewer Agent 设置
    CodeReviewPassed     bool   // 由 code_reviewer Agent 设置
    VerificationPassed   bool
    LastStageFailed      bool
    LastFailureReason    string
    RetryCount           int
}
```

#### PolicyContext

```go
type PolicyContext struct {
    AllowWrite          bool
    RequireReview       bool
    RequireVerification bool
    MaxRetriesPerStage  int
}
```

#### StageConfig

启动时从 YAML 加载。

```go
type StageConfig struct {
    Name          PipelineStage
    DefaultAgent  AgentName
    DefaultSkill  string
    Terminal      bool
    RequiresWrite bool
}
```

#### Transition / TaskPolicy

```go
// Transition 是阶段间带优先级的有向边。
// 按任务策略过滤，并根据运行时条件评估。
type Transition struct {
    From     PipelineStage
    To       PipelineStage
    Priority int
}

// TaskPolicy 定义给定任务类型允许的阶段。
type TaskPolicy struct {
    Name          string
    AllowedStages []PipelineStage
    Constraints   []string
}
```

---

## 6. 请求生命周期

### 端到端序列图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Orch as 编排器
    participant Agent
    participant Skill as 技能
    participant Tool as 工具 / MCP
    participant LLM

    User->>Orch: 用户请求
    Note over Orch: 初始化 BusContext<br/>Mutable = NewMutableState(Objective=request)

    Note over Orch: Phase 1: analyze(一次)
    Orch->>Agent: 调度 analyzer + task-analysis-skill
    Agent->>Tool: 调用 todo_write 写入 TaskList
    Tool-->>Agent: 更新成功
    Agent-->>Orch: StageOutput

    Note over Orch: Phase 2: 遍历 pending tasks
    loop 每个 pending task
        Orch->>Orch: 重置 per-task state<br/>(Signals / Missing / stage / 振荡计数)
        loop per-task stage 循环(explore→...→finalize)
            Orch->>Orch: 振荡守卫 + decide_next_stage
            Orch->>Agent: 调度当前 stage 的 agent
            Agent->>Skill: 加载技能配置
            loop ReAct 循环
                Agent->>LLM: 发送 PromptContext
                LLM-->>Agent: 推理 + 工具调用
                Agent->>Tool: 调用工具 / MCP
                Tool-->>Agent: ToolResult / MCPResponse
            end
            Agent-->>Orch: StageOutput
            Orch->>Orch: applyStageOutput
        end
        Orch->>Orch: 本 task 的 finalize.FinalAnswer<br/>→ Mutable.UpdateTaskResult(task, Done)
    end

    Note over Orch: 所有 task 完成,Run 返回
    Orch-->>User: BusContext(每个 task 自带 Result)
```

### 逐步状态机演练

下面的演练针对**单个 task**(即 Phase 2 里的一次 per-task 循环)。多任务 Run 把 step 2 执行一次,然后重复 step 3–14 每个 task 一次,最后每个 task 各自落一份 Result 到 `Mutable.TaskList.Tasks[i].Result`。

1. **用户请求到达** → 编排器创建初始 BusContext,`Mutable = NewMutableState(Objective=request)`
2. **`analyze`**(Phase 1,只跑一次) — analyzer + task-analysis-skill:解析意图,通过 `todo_write` 工具在 Mutable 中产出 TaskList,每个 task 带 `Writing` / `HighRisk` 两个布尔
3. **编排器重新评估**(进入 Phase 2 per-task 循环) — 读取当前 task 的 `MissingFacts`:
   - `MissingFacts` → 路由到 `explore`（优先级 100）
   - `MissingPlan` → 路由到 `plan`（优先级 80）
   - `MissingNone` → 路由到 `finalize`（优先级 20）
4. **`explore`** — 探索器 Agent + repo-explore-skill：浏览代码，构建事实库
5. **编排器重新评估** — 事实已收集：
   - 路由到 `plan`（优先级 100）
   - 或自循环 `explore`（如需更多事实，优先级 30）
6. **`plan`** — 规划器 Agent + implementation-plan-skill：设计实现方案
7. **编排器重新评估** — 方案已存在：
   - 路由到 `design_review`（优先级 100）
   - 或路由到 `implement`（优先级 80，跳过设计审查）
   - 或回退到 `explore`（事实不足时，优先级 60）
8. **`design_review`** — 设计审查器 Agent + design-review-skill：审查方案可行性 *（仅限 high_risk_implementation 策略）*
   - 通过 → 设置 `DesignReviewPassed = true`，路由到 `implement`
   - 不通过 → 回退到 `plan` 修订方案
9. **`implement`** — 实现器 Agent + code-implement-skill：编写代码，生成补丁
10. **编排器重新评估** — 补丁已存在：
    - 路由到 `code_review`（优先级 100）
    - 或路由到 `verify`（优先级 80，跳过代码审查）
11. **`code_review`** — 代码审查器 Agent + code-review-skill：审查代码正确性 *（仅限 high_risk_implementation 策略）*
    - 通过 → 设置 `CodeReviewPassed = true`，路由到 `verify`
    - 不通过 → 回退到 `implement` 修复问题
12. **`verify`** — 验证器 Agent + verification-skill：运行测试、lint、构建
13. **编排器重新评估** — 验证结果：
    - 通过 → 路由到 `finalize`（优先级 100）
    - 失败 → 回退到 `implement`（优先级 80）
14. **`finalize`** — 终结器 Agent + final-answer-skill：汇编最终输出 → 返回给用户

---

## 7. 编排器状态机

### 完整状态机图

```mermaid
stateDiagram-v2
    [*] --> analyze

    analyze --> explore : 优先级 100
    analyze --> plan : 优先级 80
    analyze --> finalize : 优先级 20

    explore --> plan : 优先级 100
    explore --> finalize : 优先级 40
    explore --> explore : 优先级 30（自循环）

    plan --> design_review : 优先级 100
    plan --> implement : 优先级 80
    plan --> explore : 优先级 60（回退）

    design_review --> implement : 优先级 100
    design_review --> plan : 优先级 70（回退修订）
    design_review --> finalize : 优先级 20

    implement --> code_review : 优先级 100
    implement --> verify : 优先级 80
    implement --> plan : 优先级 50（回退）

    code_review --> verify : 优先级 100
    code_review --> implement : 优先级 70（回退修复）
    code_review --> finalize : 优先级 20

    verify --> finalize : 优先级 100
    verify --> implement : 优先级 80（修复并重试）

    finalize --> [*]
```

### 任务策略叠加

```mermaid
graph LR
    subgraph "analysis 策略"
        A1[analyze] --> A2[explore] --> A3[finalize]
    end
```

```mermaid
graph LR
    subgraph "implementation 策略"
        B1[analyze] --> B2[explore] --> B3[plan] --> B4[implement] --> B5[verify] --> B6[finalize]
    end
```

```mermaid
graph LR
    subgraph "high_risk_implementation 策略"
        C1[analyze] --> C2[explore] --> C3[plan] --> C4[design_review] --> C5[implement] --> C6[code_review] --> C7[verify] --> C8[finalize]
    end
```

### YAML 配置参考

完整的编排器配置维护在 [`config/orchestrator.yaml`](../config/orchestrator.yaml) 中。该文件是阶段定义、转换规则、任务策略和功能开关的权威来源。

---

## 8. 关键设计模式

### 状态机模式（编排器）

编排器实现了一个**有限状态机**，其中：
- **状态** = 流水线阶段（analyze, explore, plan, design_review, implement, code_review, verify, finalize）
- **转换** = 阶段间优先级加权的有向边
- **守卫** = 任务策略、功能开关和运行时条件过滤转换
- **动作** = 调度适当的 Agent 及正确的技能

此模式确保确定性、可审计的流水线推进，同时支持回退和条件路径。

### ReAct 循环（Agent）

每个 Agent 遵循 **ReAct（推理 + 行动）** 模式：
1. **推理** — LLM 分析当前状态并决定下一步行动
2. **行动** — Agent 调用选定的工具或 MCP
3. **观察** — Agent 处理结果并更新其工作状态
4. 重复直到阶段目标达成

此模式允许 Agent 处理动态、多步骤的任务，无需硬编码逻辑。

### 策略模式（技能）

技能实现了**策略模式** — Agent 的行为完全由它绑定的技能配置决定，更换技能即可改变工作流、提示词和工具偏好，无需修改 Agent 代码。例如 `analyzer` 通过 `task-analysis-skill` 完成任务分类，而 `planner` 通过 `implementation-plan-skill` 设计实现方案——两者都是 BaseAgent 的薄包装，差异完全来自技能配置。

### 适配器模式（工具 / MCP）

工具和 MCP 服务器都实现了统一接口（`Execute` / `CallTool`），允许 Agent 互换调用它们。适配器模式将本地执行和远程 MCP 调用的差异隐藏在通用抽象之后。

### 带回退的流水线

与线性流水线不同，本系统支持**非线性流程**：
- **正向推进** — 阶段间的默认路径
- **回退** — 当新信息使先前工作失效时返回较早的阶段（如 `implement → plan`，方案需要修订时）
- **自循环** — 需要更多工作时重复某个阶段（如 `explore → explore`）
- **跳过路径** — 不需要时跳过某些阶段（如 `analyze → finalize`，用于简单问题）

---

## 9. 错误处理与容错

### 层间错误传播

| 错误来源 | 处理方式 |
|----------|----------|
| 工具失败 | ToolResult.Success = false；Agent 观察后重试或使用替代工具 |
| MCP 失败 | MCPResponse.Success = false；如有可用的本地工具则降级 |
| LLM 失败 | LLMAdapter 以指数退避重试；降级到备用模型 |
| Agent 失败 | 编排器接收错误；设置 `LastStageFailed = true`，评估回退转换 |
| 阶段失败 | 编排器递增 `RetryCount`；重新进入阶段或回退 |

### Agent 重试与降级

- Agent 在每个阶段有可配置的重试预算（`PolicyContext.MaxRetriesPerStage`）
- 失败时，Agent 可以：
  1. 使用修改后的参数重试相同工具
  2. 尝试替代工具
  3. 向编排器报告失败（编排器可能会回退）

### 工具 / MCP 超时处理

- 每次工具调用有可配置的超时时间
- 超时时：ToolResult 标记为失败，附带超时原因
- Agent 观察超时后决定是重试还是不使用该结果继续执行

### 优雅降级

- 如果 MCP 服务器不可用，Agent 降级到本地工具
- 如果主 LLM 不可用，系统降级到备用模型
- 如果非关键阶段反复失败，编排器可能跳过它（如在最大重试次数后跳过 `verify`，带警告进入 `finalize`）

### 阶段级重试（编排器）

编排器可以通过回退转换重新进入失败的阶段：
- `implement → plan` — 如果实现过程揭示了方案缺陷
- `verify → implement` — 如果验证失败，代码需要修复
- `explore → explore` — 自循环以收集更多事实

`ExecutionSignals` 中的 `RetryCount` 防止无限循环。

---

## 10. 可扩展性

### 添加新工具

1. 实现 `Tool` 接口：
   ```go
   type MyTool struct{}
   func (t *MyTool) Name() string        { return "my_tool" }
   func (t *MyTool) Description() string  { return "做一些有用的事情" }
   func (t *MyTool) Parameters() json.RawMessage { return schema }
   func (t *MyTool) Execute(params json.RawMessage) (ToolResult, error) { ... }
   ```
2. 在工具注册表中注册
3. 在相关技能的 `ToolSuggestions` 中引用

### 添加新 MCP 服务器

1. 实现 `MCPServer` 接口或使用现有的 MCP 兼容服务器
2. 配置传输方式（stdio / SSE / HTTP）和连接详情
3. 系统通过 `ListTools()` 在运行时发现可用工具

### 创建新技能

1. 定义 `SkillConfig`，包含目标、工作流步骤、工具建议、输出格式和禁止事项
2. 在技能注册表中注册
3. 在 `orchestrator.yaml` 中绑定到某个阶段（作为默认或覆盖）

### 自定义 Agent 类型

1. 定义新 Agent 的阶段绑定和能力（只读 vs. 读写）
2. 在 Agent 注册表中注册
3. 添加到 `orchestrator.yaml` 的 `agents` 部分
4. 作为 `default_agent` 绑定到某个阶段

### 添加新流水线阶段

1. 在 `PipelineStage` 中添加阶段常量
2. 在 `orchestrator.yaml` 中定义阶段，包含默认 Agent、技能和终止标志
3. 添加到/从新阶段的转换规则（带优先级）
4. 更新任务策略以在适当位置包含新阶段
5. 为新阶段创建或分配技能
