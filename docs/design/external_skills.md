# 外部 Skill 接入 —— 设计文档

**状态**：设计审查。未开始实现。
**作者**：session-35 kickoff
**目标**：为外部（yaml 声明）skill 引入**最窄的接入面**，在保留全部内置 skill 稳定性的前提下扩展 codrax 能力。
**非目标**：本文**只**覆盖 skill 的外部化。工具（`tool.Registry`）、MCP 的外部化是平行工作，各自的接入面不同；本文只写**当外部 skill 需要引用** MCP 工具时要约束什么。

---

## 1. 现状审计（evidence-backed，每条均附 file:line）

### 1.1 Skill 数据流骨架

```
codrax.yaml → 加载 RuntimeSettings ── (不参与)
                                    ↓
providers.yaml → LLM adapter ──────┐↓
                                    ↓↓
cmd/root.go:1348  skill.NewRegistry()          ← 永远返回空
cmd/root.go:1349  skill.RegisterDefaults(reg)  ← 注入 10 个内置 skill（literal）
cmd/root.go:1371  orchestrator.New(..., reg, ...)
                    ↓
orchestrator.dispatchStage(stage) → topology.go:24-46 查 skillName
                    ↓
skillRegistry.Get(skillName) ← 未注册 → 阶段失败
                    ↓
BaseAgent.Execute(ctx, *skill.Config)
                    ↓
buildToolSchemas(sk)                       ← agent/agent.go:1402-1418
BuildPromptContext(ac, sk)                 ← context/builder.go:381-396
```

### 1.2 硬编码 skill 名称清单（来自 Audit Section B）

| Skill 名称 | 管线阶段 | 结构测试锁定 |
|---|---|---|
| `log-triage-skill` | `log_triage`（条件前置） | log_triager_test.go |
| `log-segmentation-skill` | log-triage 两步 fallback | log_triager_test.go:277 |
| `analysis-skill` | `analyze` | analyzer_prompt_test.go:122-123, 519-531 |
| `explore-skill` | `explore` | defaults_test.go:12-23 |
| `extract-skill` | `extract` | extractor_test.go:40-91 |
| `answer-document-skill` | `finalize` | answer_document_evaluator_test.go:1143, answer_document_e2e_test.go:307 |
| `change-plan-skill` | `plan` | （依赖 write_mode_red_lines_test.go 间接保证） |
| `code-write-skill` | `apply` | （同上） |
| `test-execute-skill` | `verify` | （同上） |

**结论**：10 个名称 × 2 处绑定（`topology.go` + `defaults.go`），每次引用都是**字面值**。任何第三方给 yaml 写 `name: analysis-skill` 就可以**静默覆盖**内置 skill（`Registry.Register` 用 `skills[cfg.Name] = cfg` 直接覆写，`skill.go:28-31`）。

### 1.3 消费点（field-level）

| 消费点 | 字段 | 空值处理 |
|---|---|---|
| `context/builder.go:381-396` | Goal / Workflow / OutputFormat | **无 nil 检查**，空串被 `append` 成空 PromptSection → LLM 看到空段标题 |
| `context/builder.go:393` | Prohibitions | `len(sk.Prohibitions) > 0` 保护 |
| `context/builder.go:206-214` | ToolSuggestions | → `skillToolSet(sk)` map 化 → `reasoningHygieneFor()` 检查 |
| `context/builder.go:232` | Name | 字面值比较 `sk.Name == "extract-skill"`（扩展 Turn-B 抑制） |
| `context/builder.go:374` | Name | 字面值比较 `sk.Name == "explore-skill"`（OutputFormat section 标题改写） |
| `agent/agent.go:1402-1418` | ToolSuggestions | 对每个名字 `tool.Registry.Get()`，**失败静默 continue**（无日志、无错误） |
| `agent/agent.go:1422-1429` | — | MCP 工具**无条件**加入 schema（**不经过 ToolSuggestions 过滤**） |
| `skill/glossary_test.go:26-62` | Goal / Workflow / OutputFormat / Prohibitions / ToolSuggestions | 扫 `InternalTermsBlocklist`，任一命中即失败 |

### 1.4 "看似保护"其实不保护的地方

1. **Registry.Register 静默覆盖**（skill.go:31）。相同 Name 二次注册直接覆写。**无错误、无日志、无测试**。
2. **ToolSuggestions 引用不存在工具**（agent/agent.go:1405）。`continue` 跳过，LLM 永远看不到该工具。**无日志、无测试**。
3. **空 ToolSuggestions + 存在 MCP server**（agent/agent.go:1402 vs 1422）。循环零次 → MCP 工具仍会加入 schema。**无测试断言这是不是意图。**
4. **pipelineTopology 引用未注册 skill**（orchestrator.go:2478）。`Get()` 失败 → 阶段失败。**无启动时 pre-flight 校验，要运行到对应阶段才炸**。
5. **空 Goal / Workflow / OutputFormat**（builder.go:381-389）。无 nil 检查，LLM 拿到空段。**无任何通用测试断言全非空**。

---

## 2. 威胁模型

按"外部 yaml 能做什么坏事"枚举。每条都标记当前是否有防御。

| 威胁 | 效果 | 现有防御 | 缺口 |
|---|---|---|---|
| T1: 外部 skill name 与内置冲突（如 `name: analysis-skill`） | 内置 skill 被静默覆盖；pipeline 阶段的 prompt 整段换成第三方内容 | **无** | **critical** — 必须在加载时拒绝 |
| T2: 外部 skill `ToolSuggestions` 指向写工具 `apply_patch` / `exec_command` / `run_tests`，但声明自己是读阶段 skill | 读阶段 agent 看到写工具 schema → 可能真的调用 → 主仓字节变更 | `reasoning_hygiene` 只是**说服式**提示，不是阻止；工具 schema 已构建 | **critical** — 外部 skill 的 ToolSuggestions 必须经过 write-capability 过滤 |
| T3: 外部 skill 空 ToolSuggestions，MCP server 已配置 | LLM 拿到全部 MCP 工具，allowlist 机制失效 | **无** | **high** — 空 ToolSuggestions 对外部 skill 必须等价于"显式零工具"，不自动扩到 MCP |
| T4: 外部 skill Goal / Workflow / OutputFormat 空 | LLM 拿到空段标题，行为未定义（通常 degenerate） | builder.go 无 nil 检查 | **medium** — 加载时 reject |
| T5: 外部 skill 在 LLM-facing 文本里用内部术语（"Turn A" / "HypothesisSet" / "Phase 0"） | 误导 LLM；glossary 红线失守 | `glossary_test.go:26-62` 已覆盖整个 registry | **已覆盖** ✓ |
| T6: 外部 skill 引用不存在的 tool 名 | LLM schema 少一个工具；dispatch 时才发现 | agent.go:1405 静默 `continue` | **high** — 必须在加载时拒绝 |
| T7: 外部 skill 注入 prompt-injection payload（如"ignore previous instructions"） | agent 可能偏离职责 | **无** | **medium** — 短期难彻底防，加载时做长度上限 + 禁用关键字黑名单 |
| T8: 外部 skill 被注册后 pipelineTopology 某阶段仍缺 skill | 阶段运行时炸（"skill not found"） | 无启动 pre-flight | **low**（fail-loud 可观测）但**建议**加启动时 topology ↔ registry 完整性校验 |
| T9: 外部 skill yaml 文件被 in-tree checkout 污染（第三方 fork 注入恶意 skill） | 供应链攻击 | `write_enabled` + sandbox 仅部分缓解 | **out-of-scope** —— 本文档不解决供应链；部署方自行审核 |

---

## 3. 设计原则

**P1：内置 skill 名称是保留命名空间。** 外部 skill 必须使用不同前缀。

**P2：外部 skill 不得进入 `pipelineTopology`。** 管线阶段-skill 绑定保持硬编码（topology.go 不变）。外部 skill 只能被**子 agent 调度 / REPL 用户显式指定 / 未来的 custom 工具链**消费。

**P3：Fail-loud 在加载时，不在 dispatch 时。** 任何外部 skill 能触发的问题（T1-T8）都必须在启动阶段被拒绝，不允许拖到阶段分派才报错。

**P4：外部 skill 的 ToolSuggestions 默认是闭世界。** 空 ToolSuggestions 等价于"零工具"，不自动扩展到 MCP。

**P5：外部 skill 不得持有写能力，除非 yaml 显式请求 + 配合 write-mode gate。** 默认 ToolSuggestions 里出现 `WriteCapable` 工具 → 加载时 reject。

**P6：glossary 已覆盖的保持，其他普遍性校验补齐（非空字段、tool 名存在性）。**

**P7：所有新校验作用于**所有**registered skill**，包括内置的（间接保证内置 skill 自身也满足同样的健康性，避免将来内置 skill 跟外部 skill 校验标准分叉）。

---

## 4. 接入点设计（三层漏斗）

```
   外部 yaml 文件（磁盘）
           ↓  ① yaml.Unmarshal → skill.Config
           ↓
   ExternalSkillLoader.Load(path)
           ↓  ② 加载时校验（8 条）
           ↓    ├─ Name 前缀白名单 / 黑名单查重
           ↓    ├─ Goal / Workflow / OutputFormat 非空
           ↓    ├─ ToolSuggestions 每个名字 ∈ tool.Registry
           ↓    ├─ 无 WriteCapable 工具
           ↓    ├─ 显式 MCP 工具名（空 ToolSuggestions 不扩展）
           ↓    ├─ 总字节上限 (T7 缓解)
           ↓    └─ glossary 延迟到 register 后由 glossary_test.go 覆盖
           ↓
   skillRegistry.RegisterExternal(cfg)  ← 新方法；内部最终也调 Register，但在这一层记录 origin=external
           ↓
   启动 pre-flight：所有 pipelineTopology.Skill 名字 ∈ Registry（③ T8 缓解）
           ↓
   运行时：agent Execute（完全不变；外部 skill 走同一 BaseAgent）
```

### 4.1 接入点 1 —— `codrax.yaml` 新键

```yaml
# Directory of external *.skill.yaml files. Nil / empty (default)
# disables external skill loading entirely. Relative paths resolve to
# <runtime-anchor>/skills/ (i.e. <CWD>/.codrax/skills/).
# external_skills_dir: ""
```

仅一个键。默认空 = 完全走现有路径，字节级不变（符合 L1 红线精神）。

### 4.2 接入点 2 —— 外部 yaml 文件格式

跟 `skill.Config` 对齐，**加两个外部专用字段**：

```yaml
# eval/external-example.skill.yaml
name: external:refactor-advisor         # 必须 "user:" 前缀（见 §4.3）
goal: "..."                         # 必填
workflow:                           # 必填，非空
  - "..."
output_format: "..."                # 必填
prohibitions:                       # 可选
  - "..."
tool_suggestions:                   # 必须显式枚举；空即零工具
  - read_file
  - grep
  - "mcp:<server_name>.<tool_name>" # MCP 工具的显式引用

# 外部 skill 专用字段：
description: "..."                  # 说明文案；不进 LLM prompt，只为 /skills list 展示
source_file: "..."                  # 加载时自动填（操作员审计用）
```

### 4.3 Name 保留命名空间

**内置 skill 名称不得以以下前缀开头**：
- 无前缀的"短名"形式（`analysis-skill`, `explore-skill`, ...）

**外部 skill 必须以 `external:` 前缀开头**：`external:refactor-advisor`, `external:security-audit`, etc.

这样内置 vs 外部天然不冲突，加载器的第一道检查就是"Name 以 `external:` 开头吗？否则 reject"。

**替代方案（已拒绝）**：
- **命名空间分两个 Registry**（builtin + external）：会让 `skillRegistry.Get()` 的现有调用点复杂化（要查两个表），且外部 skill 无法被内置流程（如 explorer 的 sub_agent）调用
- **在 Config 加 `origin: external|builtin`**：origin 是外部自报，恶意 yaml 声明 `origin: builtin` 就绕过
- **MD5 / SHA 白名单**：过度工程，改一个字都要更新白名单

### 4.4 加载器 API 形态（**未实现**）

新文件：`internal/skill/external.go`

```go
// LoadExternalSkillsDir 扫 dir 下所有 *.skill.yaml，对每份做 8 条校验，
// 通过的调 reg.Register(cfg) 注册。失败 fail-loud 返 error——启动阶段
// 解析失败等同于 invalid codrax.yaml（参考 LoadRuntimeSettings 的
// posture）。返回 loaded / rejected 两个切片供 cmd/root.go 打 INFO 日志。
func LoadExternalSkillsDir(reg *Registry, toolReg *tool.Registry, mcpReg *mcp.Registry, dir string) (loaded []string, rejected []LoadError, err error) {
    ...
}

// LoadError 描述一份 yaml 文件被拒绝的具体原因 + 违反的校验编号。
type LoadError struct {
    Path      string
    RuleIndex int    // 1-8 对应 §4.5 的 8 条
    Reason    string
}
```

### 4.5 7 条加载校验（按顺序执行，首错即返）

| # | 规则 | 缓解威胁 |
|---|---|---|
| 1 | `strings.HasPrefix(cfg.Name, "external:")` 否则 reject | T1 |
| 2 | 已经注册了同名 skill → reject（防止两份外部 yaml 也冲突） | T1 |
| 3 | `len(cfg.Goal) >= 10 && len(cfg.Workflow) >= 1 && len(cfg.OutputFormat) >= 10` | T4 |
| 4 | 每个字段字符数 ≤ `external_skill_max_field_bytes`（yaml 可配置，默认 16 KB） | T7 |
| 5 | `ToolSuggestions` 每个名字解析：`read_file`/`grep`/... → `toolReg.Get(name)` 必须成功，且 `!t.IsWrite()`（阻止读 skill 携带写工具） | T2, T6 |
| 6 | `ToolSuggestions` 中 `mcp:<server>.<tool>` 形式的条目必须在 `mcpReg` 里真的存在 | T3, T6 |
| 7 | glossary lint 延迟到 register 之后由 `skill/glossary_test.go` 在 CI 环节执行。运行期不检查（内置 skill 也不运行期检查） | T5 |

**空 ToolSuggestions 是合法配置**（与初版草案相反）。外部 skill 可以真的只需要 zero tools（纯 chat-style 回答）。但这就引入 T3 风险：`agent/agent.go:1422` 对所有 skill 无条件注入 MCP 工具，空 ToolSuggestions 的外部 skill 会拿到**所有** MCP 工具——背离 allowlist 语义。

解决见 §4.5.1，在 tool-schema 构建层**分路径**。

### 4.5.1 T3 缓解 —— 外部 skill 的 strict tool-schema 构建

**现状**：`BaseAgent.buildToolSchemas` 对所有 skill 做三步：

1. 对 `ToolSuggestions` 逐个 `tool.Registry.Get()`，拼入 schema
2. **无条件**把 `mcp.Registry.ListAllTools()` 全加入（agent.go:1422）
3. 若 sub-agent 配置存在，拼入 `propose_sub_agents`

**新行为 —— 通过 `strings.HasPrefix(sk.Name, "external:")` 分支**：

**内置 skill（字节级不变）**：
- ToolSuggestions 过滤的内置工具
- 全量 MCP 工具
- 自动加 propose_sub_agents（如 sub-agent 已注册）

**外部 skill（strict allowlist）**：
- **仅**显式列入 `ToolSuggestions` 的工具：
  - 内置工具名 → `tool.Registry.Get(name)` 命中
  - `mcp:<server>.<tool>` 形式 → 精确指向某个 MCP 工具
- **不自动**加入全量 MCP
- **不自动**加入 propose_sub_agents（外部 skill 不参与 sub-agent 派生）
- 空 ToolSuggestions = zero tools（LLM 只能纯文本回答，ReAct 循环一轮后停）

**T3 根本防御**：外部 skill 的工具面**完全由 yaml 显式控制**。

此改动 ~10 行 + 两个测试（内置路径 vs 外部路径各一）。内置行为字节级不变是 `TestBuiltInSkills_ToolSchemaByteIdentical` 的断言目标。

### 4.6 启动时 pre-flight（新）

加载完外部 skill 后、`orch.Run()` 第一次触发前，`cmd/root.go` 跑一次：

```go
for _, s := range orchestrator.AllStages() {
    skName, ok := pipelineTopology[s]
    if !ok || skName == "" { continue }
    if _, err := skillRegistry.Get(skName); err != nil {
        return fmt.Errorf("pipeline topology references unregistered skill %q (from stage %s)", skName, s)
    }
}
```

这是**内置 skill** 也会受益的健壮性检查 —— 如果 defaults.go 漏注册某个 skill（比如将来 refactor 忘加），启动就炸，不会拖到运行时。

### 4.7 `Register()` 改造（非破坏性）

当前 `skill.Registry.Register()` 静默覆盖。改：

```go
func (r *Registry) Register(cfg *Config) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.skills[cfg.Name]; exists {
        logging.Warning("[skill] duplicate registration for %q — second wins; check for accidental re-registration", cfg.Name)
    }
    r.skills[cfg.Name] = cfg
}

// RegisterExclusive 在 cfg.Name 已注册时失败。外部 skill 加载器用这个。
func (r *Registry) RegisterExclusive(cfg *Config) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.skills[cfg.Name]; exists {
        return fmt.Errorf("skill %q already registered", cfg.Name)
    }
    r.skills[cfg.Name] = cfg
    return nil
}
```

内置 `RegisterDefaults` 继续用 `Register()`（幂等：多次调用同名盖写是 OK 的）。外部加载器用 `RegisterExclusive()`，重复名字 fail-loud。

---

## 5. 运行时行为 —— 什么不变、什么变

### 5.1 不变（L1 byte-identity 类红线）

- `pipelineTopology` 条目**绝对不变**。外部 skill 不能绑定到任何 built-in stage。
- `BaseAgent.Execute` 本身**不变**。它只知道"收到一个 `*skill.Config`，按协议构建 prompt + tool schemas"。
- `buildToolSchemas()` 逻辑**不变**。加载时已校验过的外部 skill 和内置 skill 走一样的过滤路径。
- `BuildPromptContext()` 的段渲染顺序、标题、内容**不变**。
- 现有所有测试**不变**。

### 5.2 变（唯一三处新增）

1. **`cmd/root.go:1349` 之后新增**：若 `runtime_settings.external_skills_dir != ""`，调 `LoadExternalSkillsDir`。失败 → 启动失败（fail-loud）。
2. **`cmd/root.go` orch.New 前新增**：pre-flight loop 校验 topology ↔ registry 完整性。
3. **`skill/skill.go`**：给 `Register()` 加 dup-warning；新增 `RegisterExclusive()`。

### 5.3 外部 skill 在哪里被**调用** —— REPL 命令

**本 PR scope**：加两个 REPL slash 命令 + 一个通用 "external agent" 分派路径。**不**允许外部 skill 进入 `pipelineTopology`，**不**允许被 explorer 的 `propose_sub_agents` 自动选取（那会让内置流程行为依赖外部文件的存在与否）。

**新命令**：

- **`/skills list`** — 列出所有已注册 skill，分类展示"内置"/"外部"。纯只读，对 agent 执行无副作用。
- **`/skills show <skill-name>`** — 打印指定 skill 的完整定义（Goal / Workflow / OutputFormat / Prohibitions / ToolSuggestions）。纯只读。
- **`/ask <external:skill-name> <question>`** — 用指定外部 skill 跑一次独立 ReAct 分派。**拒绝**非 `external:` 前缀（保护内置 skill 不被用户误调绕开 topology）。
  - Runner：一个新的 **`AgentExternal` agent type**（`internal/agent/external.go`）—— 嵌 `BaseAgent`，配一个极简 `externalEvaluator`：
    - `BuildInitialInstruction` 返回 `""`（skill 的 Workflow / OutputFormat 已经在 PromptContext 里）
    - `ShouldStop` = `iter >= AgentSettings.MaxIterations`（靠 context-pressure 监控兜底）
    - `ParseOutput` = 汇总所有 tool result + assistant content 进 `Mutable.SetResult(...)`；无任何 CGEC / 假设 / IR 副作用
    - **不**实现 `LoopController`（无 mid-loop 注入逻辑）
  - BusContext：Mode 固定 `ModeRead`（绝不允许外部 skill 触发写模式）；`AnalysisIR` 为 nil（分析管线不参与）；`StageReports` 空；`TaskState.Stage` = `StageExternal`（新枚举值）。
  - 输出：同读模式单 shot，`busCtx.Mutable.Result()` 渲染到 stdout。
  - Prior conversation：按 `agent_prior_conversation_policy`；默认 `analyzer` 政策对外部 skill 视同 analyzer（读 Prior）。

**新 Stage 枚举值**：`types.StageExternal`（`internal/types/enums.go`）。**不**进 `AllMainStages()` / `AllStages()`；仅 REPL 分派路径持有。

**`AgentExternal` 在 `pipelineTopology` 里不出现** —— 保持红线 L1"读模式 Run 字节级不变"。

### 5.3.1 调用路径隔离（防止意外污染）

- 外部 skill 的分派**只**经过 `/ask`，不穿过 `Orchestrator.Run()` 的 mode-routing 分支
- REPL 为 `/ask` 单独走 `runExternalSkill(ctx, skillName, question)`（新 REPL 内部方法）—— 不设置 `ctx.Mode`，不加载 ChangePlan，不 provision worktree
- `BusContext.Mutable` 是新 `NewMutableState(question)` 实例，**不**复用 REPL 上一次读模式 Run 的 Mutable（避免 Evidence / AnalysisIR 串扰）
- `memory` store 按正常 REPL 轮次写入（`kind=external`，新 `memory.KindExternal` 枚举）；memory 检索照常但 topic 标注加 `[external:<skill-name>]`

**未来扩展（不在本次 scope 内）**：
- 外部 skill 作为 custom sub_agent 模板（需要 explorer 工具扩展）
- 外部 skill 参与 write-mode（需要新 write_enabled 等级开关）
- 外部 skill 可以声明 prompt 模板字段（需要 context/builder.go 扩展；暴露 sk.Name 字面比较模式，风险最大）

---

## 6. 测试计划

**新增测试（本 PR 必须包含）**：

| 测试 | 位置 | 断言 |
|---|---|---|
| `TestExternalSkill_RequiresUserPrefix` | `skill/external_test.go` | 8 条加载规则的第 1 条 |
| `TestExternalSkill_DuplicateNameRejected` | 同上 | 第 2 条 |
| `TestExternalSkill_RequiresNonEmptyFields` | 同上 | 第 3 条 |
| `TestExternalSkill_FieldSizeCap` | 同上 | 第 4 条 |
| `TestExternalSkill_WriteToolRejected` | 同上 | 第 5 条（读 skill 不能携带写工具） |
| `TestExternalSkill_UnknownToolRejected` | 同上 | 第 5 / 6 条 |
| `TestExternalSkill_EmptyToolSuggestionsRejected` | 同上 | 第 7 条 |
| `TestExternalSkill_GlossaryLintCovers` | `skill/glossary_test.go` 新增 sub-test | 用 `external:test-bad-term` 注册带内部术语的外部 skill，断言 glossary lint 仍然失败 |
| `TestRegisterExclusive_RefusesDuplicate` | `skill/skill_test.go` 新 | `Register()` + `RegisterExclusive()` 行为区分 |
| `TestRegisterDefaults_DoesNotUseUserPrefix` | 同上 | 内置 skill 永不以 `external:` 开头（保护 §4.3 namespace） |
| `TestTopologyReferencesRegisteredSkills` | `orchestrator/orchestrator_test.go` | pre-flight 逻辑校验 |
| `TestExternalSkillsDirEmpty_NoRegistration` | `cmd/root.go` 测试或新 cmd_test | 空 dir 路径 → skillRegistry.List() 与不含 external_skills_dir 时完全相等（字节级回归保证） |

**不在本 scope 的测试**：
- glossary lint 对内置 skill 的现有覆盖保留不变
- 每个内置 skill 的结构测试保留不变

---

## 7. 回滚预案

**完全回滚**：删除新 `skill/external.go` + 回退 `skill/skill.go` 的 `RegisterExclusive` + 移除 `external_skills_dir` 的读取。`cmd/root.go` 的一处新增（loader call）是唯一非注释面。

**runtime 禁用**：`codrax.yaml :: external_skills_dir: ""`（默认值）= 从未加载。

**部分生效**：外部 skill 被正常加载但 topology 仍然硬编码绑定内置 skill，用户不 `/ask <user:...>` 就永不触发。

---

## 8. 明确拒绝的设计选项

| 选项 | 为什么拒绝 |
|---|---|
| 允许外部 skill 覆盖内置 | T1；破坏 topology 契约；覆盖 analysis-skill 意味着重定义整个分类器行为 |
| 外部 skill 自己声明"我是 analyze stage 的 skill"然后被 topology 自动拾取 | topology.go 的硬编码是 L1 红线的基石；运行时切换 = 把管线变成声明式 = 所有结构测试失效 |
| 加密签名 / 白名单 yaml | 供应链防护属 CI / 部署层职责；在 codrax 内做只是幻觉安全 |
| 外部 skill 里写 Go 代码 / WASM 插件 | 完全违背 codrax "全代码编译时安全"的设计精神；见 Tool registry 讨论也是同理 |
| 允许外部 skill 扩展 `canonicalSystemSectionOrder` / `canonicalUserSectionOrder` | prompt 段顺序是 LLM 行为稳定性的基石；外部不动 |
| 允许外部 skill 在 Workflow 里插 `{{placeholder}}` 模板变量 | 模板语言是另一个 attack vector；如果真需要请用 `--request` 本身传参 |

---

## 9. 开工顺序（经审查后执行）

**阶段 A — 核心加载器（低风险，完全可回滚）**

1. `skill/external.go` 加载器 + 7 条校验 + 完整 test file
2. `skill/skill.go` `Register()` 加 dup warning + 新增 `RegisterExclusive()`
3. `internal/config/runtime.go` 加 `ExternalSkillsDir *string` + `ExternalSkillMaxFieldBytes *int`
4. `cmd/root.go` 调用加载器 + pre-flight topology 校验 + INFO 日志（loaded N skills from dir）

**阶段 B — Tool schema 分路径（buildToolSchemas 改造）**

5. `agent/agent.go` `buildToolSchemas` 按 `strings.HasPrefix(sk.Name, "external:")` 分支；外部走 strict allowlist（§4.5.1）
6. 新增 `TestBuiltInSkills_ToolSchemaByteIdentical` 守护内置路径字节不变
7. 新增 `TestExternalSkill_ToolSchemaStrictAllowlist` 守护外部路径零自动注入

**阶段 C — AgentExternal + REPL 命令**

8. `types/enums.go` 加 `StageExternal`、`AgentExternal`
9. `agent/external.go` 新 `externalEvaluator` + 注册 `AgentExternal` 到 agent registry
10. `repl/` 三命令：`/skills list`、`/skills show <name>`、`/ask <external:name> <question>`；绑定 `runExternalSkill` 内部方法
11. `memory` 加 `KindExternal`；REPL 的 external 分派按此 kind 落盘 / recall

**阶段 D — 文档**

12. `codrax.yaml.example` 加 `external_skills_dir` + `external_skill_max_field_bytes` + 一段 yaml 示例（注释形式）
13. `docs/user_guide.md` §3.3.X "外部 skill 加载" + §5.2 REPL 命令表补 3 条
14. `docs/architecture.md` §3.3 skill 章节加外部来源说明 + §4 新增 §4.7 "外部 skill 分派路径"

每阶段独立 commit。阶段 A 回滚 = 删 `external.go` + 回退 3 处非测试行；阶段 B 回滚 = 还原 buildToolSchemas 的分支；阶段 C 回滚 = 摘除新命令 + 新 agent（pipelineTopology 本就未绑）。

---

## 10. 决策记录

| 决定 | 选项 | 选中 | 理由 |
|---|---|---|---|
| 命名空间前缀 | `user:` / `external:` / `custom:` / `x-` | **`external:`** | 语义直白，与"内部"一词清晰对位；`user:` 暗示多租户但实际无租户概念 |
| 调用路径 | 本 PR 集成 vs 下次 PR 再加 | **本 PR 集成 `/skills list`+`/skills show`+`/ask`** | 设计文档的说服力必须用真实调用路径验证；休眠态 registry 无法暴露隐藏耦合 |
| 空 ToolSuggestions | reject / 允许 | **允许** | 合法 use case（纯 chat 外部 skill）；T3 风险改由 §4.5.1 的 strict schema 构建解决 |
| 字段字节上限 | 固定 / 可配置 | **默认 16 KB，yaml 键 `external_skill_max_field_bytes` 可配置** | 16 KB 是常规 workflow 综合长度的 4-8 倍，绝对够用；大型 skill 的 edge case 靠 yaml 覆盖，不在代码里写死 |

### 10.1 未决项（实现中若发现再提 review）

- **外部 skill yaml 文件名约定**：`*.skill.yaml` vs `*.yml` vs 任意 `*.yaml`？**当前计划 `*.skill.yaml`**（明确后缀避免混淆普通配置文件）
- **REPL `/ask` 的 question 是否支持多行**？默认单行；多行走 `/paste` 后接 `/ask`
- **memory 对 external 调用的 recall 策略**：同普通 turn 一起 recall，还是隔离？**当前计划同普通 turn**（memory 是用户视图，外部 skill 对用户来说只是一种 agent）

---

**Ready for implementation。** 开工按 §9 的阶段 A → B → C → D 顺序，每阶段独立 commit + 全仓测试绿才进下一阶段。
