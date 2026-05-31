# 外部 Skill 接入能力分析 —— 能否泛化通用，以及支持/不支持的系统影响

**状态**：架构分析 / 决策论证。**结论：不应构建通用的"外部 skill 覆盖"接入；通用且安全的"外部 skill"语义已由 MCP prompts（见 [`mcp_integration_v2.md`](mcp_integration_v2.md)）覆盖。**
**用途**：(1) 为 MCP 设计 v1/v2 砍掉"外部 skill overlay（原 P2）"提供正式论证；(2) 作为未来再次提出"让客户外接 skill"时的挡板与裁决依据。
**方法**：基于对 skill 子系统的 file:line 级代码探索（2026-05-31）。

---

## 0. 一句话结论

codrax 的 "skill" **不是** Claude Code 那种自由 markdown playbook，而是**每个 pipeline stage 的 load-bearing 契约教学层**——它教 LLM 如何满足下游 fail-closed 硬门 + typed answer/evidence 契约。把它外部化（让客户替换/覆盖核心 skill），会**同时击穿**契约完整性、全部测试期红线、L1 读模式字节等价、eval 基线意义四道防线。真正通用且安全的"外部领域引导"需求，**MCP prompts 已经覆盖**，且更优。

---

## 1. 先厘清：codrax 的 skill ≠ Claude Code 的 markdown skill

最容易出错的前提。codrax 的 `skill.Config`（`internal/skill/skill.go:21-39`）是强结构 + 强契约对象，不是散文：

```go
type Config struct {
    Name            string   // 唯一标识，与 stage 硬绑定
    Goal            string   // 该 stage 目的
    Workflow        []string // 过程步骤
    ToolSuggestions []string // 工具 allowlist（含该 stage 的 emit 工具）
    OutputFormat    string   // 必须与 emit 工具 JSON schema 逐字段同步
    Prohibitions    []string
    WorkflowTierB     []TierBItem // 条件化分支（按 dispatch 上下文显隐）
    ProhibitionsTierB []TierBItem
}
```

它已带 `yaml` tag，但**今天没有任何外部加载路径**——见 §2。

---

## 2. 现状（全部 file:line 核实，2026-05-31）

| 事实 | 位置 |
|---|---|
| 11~12 个 skill **全部硬编码**在 Go 里，启动时 `RegisterDefaults` 注册 | `internal/skill/defaults.go`；`cmd/root.go:2909-2911` |
| **零外部加载**：无 yaml key、无 file/env 加载、无 plugin seam（与 providers.yaml / mcp_servers 不同——后两者有 loader） | grep 全仓无 skill 外部加载 |
| stage↔agent↔skill **硬绑定、一对一、固定** | `internal/types/stage_binding.go:14-25` |
| `skill.Registry.Register` **零校验**，blind trust（不查 Goal 非空、不查 ToolSuggestions 工具是否存在、不查 jargon） | `internal/skill/skill.go:189-193` |
| skill 在 dispatch 时由 topology 静态查名取得，传给 `agent.Execute(ctx, sk)` | `orchestrator.go:7326-7350`；`agent.go:1660` |
| `ToolSuggestions` 是 allowlist，但**未注册工具静默跳过**（fail-open at schema build） | `agent.go:2686-2689` |
| 运行时唯一对 skill 的改动：注册 emit 工具后给 explore-skill 追加 `emit_evidence`/`emit_investigation_complete` | `cmd/root.go:2938-2954` |
| 无 skill 版本、无 override、无 codrax.yaml skill 配置项 | — |

**技术接入机器 ~90% 现成**（`Config` 有 yaml tag、Registry 是 map、loader 范式可照抄）。问题不在"能不能加载"，而在"加载进来安全吗"。

---

## 3. 核心论断：skill 不能"泛化通用地"外部覆盖

每个 skill 是它那个 stage 的**契约教学层**，深度耦合三样**编译态**的东西。外部化教学侧但保留这些耦合，就会出结构性故障：

### ① skill 是 load-bearing 契约，不是 advisory 文本

下游硬门（fail-closed）**假设 LLM 已被可信、测试保护的 skill 教过精确契约**：
- `contract_check_block.go` 的 required-block coverage / principal claim use / diagram edge support / claim_form 合规校验，拒答则 retry。
- 各 evaluator 的 `ShouldStop`/`ParseOutput` 直接 key 在 emit 名上——`log_triager.ShouldStop` 检查 `r.ToolName == "emit_log_triage"`（`log_triager.go:140`）。

外部 skill 一旦偏离：
- **教错** → 硬门永远拒 → retry storm → 超时（fail-closed 灾难）；
- **漏教某 typed 字段** → LLM 永不 emit → 特性静默死亡（fail-open）。

这正是架构红线 **`feedback_precise_signals_for_hard_gates` 的反向触发**：精确硬门 + 把教学侧外部化 = 门对"没被教过的 LLM"开火 = 结构上正常的问题也用户面失败。

### ② 所有红线保护都是编译/测试期，外部 skill 一律 0% 覆盖

红线 checklist（R3/R4/R6/R7/SST/R2'，`feedback_prompt_redline_checklist`）的强制点：
- `TestPromptSnapshot_NoInternalTermsInRenderedOutput`（动态渲染扫 jargon）、`internal/skill/glossary_lint_test.go`、SST 标题测试（`builder_test.go` 用常量查节）、carve-out 谓词测试——**全部只跑硬编码 skill**。
- **R2'（typed 字段六处同步）无任何自动化强制**，纯人工 review。

运行时加载的外部 skill **完全绕过**这些测试。于是 R4/R6（无内部术语）、SST（section 标题）、R2'（字段同步）对外部 skill 统统失效。

### ③ emit-tool↔evaluator 与 stage 绑定是 Go 代码，不可声明式替换

- 外部 skill **带不来自己的 evaluator**（evaluator 是编译态逻辑，决定"何时停、如何解析"）。声明式 skill 只能配既有 evaluator，而 evaluator 写死了它要的 emit 工具。
- 替换核心 stage 的 skill 还会直接撞 **L1 读模式字节等价红线**（read mode 不再字节稳定）。

---

## 4. "外部 skill"其实有三种语义，只有一种安全

| 语义 | 含义 | 评价 |
|---|---|---|
| **Scope A：覆盖/替换核心 skill** | 客户改写 analyze/explore/extract/finalize 的 prompt | ❌ **不可泛化**。攻击契约教学层 + 废掉全部测试期红线 + 破 L1。**正是 v1 明智砍掉的"自造 skill overlay (P2)"** |
| **Scope B：附加领域引导** | 客户注入领域 SOP，advisory、LLM 自筛、消毒、非 load-bearing | ✅ **安全且通用——但 MCP prompts（v2 的 External Guidance）已覆盖**。无需新机制 |
| **Scope C：新增可选 stage+skill** | 不碰核心，加新阶段 | ⚠️ **退化回 MCP**。声明式 skill 带不来 evaluator，只能配通用"无契约"evaluator，产出无法进 typed answer 契约/citation pool（受 L-MCP-1 同类约束）→ 等价于"产生一条 advisory 观测" → 又落回 MCP tools/prompts 地盘 |

**三种里能"泛化通用"且安全的那一种（B），codrax 已经有了（MCP prompts）。** 所以正确结论：**不应再造通用外部 skill 接入；领域定制走 MCP（prompts + tools + tool_metadata）即最优。**

为什么 MCP prompts 是 Scope B 的更优载体（对比"自造 skill overlay"）：
- 标准协议，客户 MCP server 投入跨 Claude Desktop / Zed / codrax 复用，无 codrax 专有格式绑定；
- 关在 `> [mcp: ...]` blockquote advisory 段 + TypedDenials 消毒 + 字节 cap + 不进 finalize（v2 §6.1），**影响不到 load-bearing 契约**；
- LLM 按 name/description 自筛相关性，无需 server 作者学 codrax stage 分类法。

---

## 5. 支持 vs 不支持 的系统影响对比

| 维度 | 支持 Scope A（通用外部 skill 覆盖） | 不支持（仅 MCP prompts = Scope B） |
|---|---|---|
| **契约完整性** | ❌ 破。教学层外部化，硬门 fail-closed 拒答或 fail-open 死特性 | ✅ 核心契约不可变，answer 永远 grounded/citable |
| **红线保护** | ❌ 全部测试期红线对外部 skill 失效（jargon/SST/R2'） | ✅ 编译态测试始终有意义 |
| **L1 读模式字节等价** | ❌ 破（核心 skill 可变） | ✅ 保持 |
| **eval 基线意义（~103/111）** | ❌ 失效——每部署跑不同 prompt，基线不可比，bug 无法归因 | ✅ 核心从不随部署变，基线恒有效 |
| **支持成本** | ❌ 每个 bug 先问"你改过 skill 吗"，排障地狱 | ✅ 行为可复现 |
| **安全（prompt injection）** | ❌ skill 是最高信任的 LLM 面，外部文本能影响 load-bearing 行为 | ✅ MCP prompts 隔离在 advisory 段 + 消毒，影响不到契约 |
| **泛化哲学** | ❌ 反向——把过拟合推给客户，放弃"单一通用 analyzer core"北极星 | ✅ 符合 `feedback_generalization_over_project_success` |
| **客户能拿到的定制力** | 全量（代价是上面全部） | 领域引导（MCP prompts）+ 新能力（MCP tools）+ 路由元数据（tool_metadata）——**覆盖真实需求** |
| **客户拿不到的** | — | 无法根本改写 stage 契约（如改 answer-document 契约）。**这是特性不是缺陷**——契约正是 grounded 答案的来源 |

---

## 6. 结论与建议

1. **当前系统不能、也不应"泛化通用地"提供外部 skill 覆盖接入（Scope A）。** skill 是契约教学层而非可插拔配置；外部化它会同时击穿契约完整性、全部测试期红线、L1、eval 基线四道防线。

2. **真正通用且安全的"外部 skill"语义（领域引导），MCP prompts 已覆盖**且更优。这是 v1/v2 砍 P2 的正确性证明。

3. **若客户确有"微调既有 skill"诉求**，唯一可辩护的窄口子：yaml **追加**（非替换）既有 skill 的 `ProhibitionsTierB` / 额外 `Prohibitions`——纯 additive、advisory、不动 emit 契约，且**必须在加载期校验 + glossary 消毒**（补上外部 skill 的 0% 保护）。但其价值低于、支持成本高于 MCP prompts，**不推荐**。

**一句话定调**：codrax 的扩展性北极星是"**单一通用核心 + 通过 MCP 接外部能力/引导**"，而不是"让外部改写核心"。skill 属于核心，MCP prompts 属于外部——这条线不该模糊。

---

## 7. 关联文档与红线

- [`mcp_integration_v2.md`](mcp_integration_v2.md) —— MCP prompts（External Guidance）= Scope B 的实现载体；§6.1 注入安全三道（stage 门 / TypedDenials 消毒 / blockquote+字节 cap）。
- 红线：`feedback_precise_signals_for_hard_gates`（本分析 §3① 的反向触发）、`feedback_prompt_redline_checklist`（§3② 的测试期强制点）、`feedback_generalization_over_project_success`（§5 泛化哲学）、L1 读模式字节等价（CLAUDE.md Red lines）。
