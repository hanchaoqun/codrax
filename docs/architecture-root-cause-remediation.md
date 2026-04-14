# 架构根因与根治方案 / Architecture Root-Cause & Remediation

> **目的 / Purpose** — 记录 2026-04-14 fake-green audit ship 后，对 codrax
> 五层多 Agent 系统做的一次架构级审查，指出多轮 patch 背后的共同根因，
> 并给出分 P0/P1/P2 的根治路线图。本文是 session 结束的资产，不是
> session 内的决策——下一次优化应以此为起点。
>
> Captures the architectural audit done after the 2026-04-14 fake-green
> ship. Identifies the common root causes behind many rounds of local
> patches, and proposes a staged P0/P1/P2 remediation roadmap. This is a
> cross-session asset — future optimization work should start here.

**状态 / Status:** 分析稿 / analysis draft — not yet a committed roadmap
**HEAD at write:** `133973d`
**作者 / Author:** Claude Opus 4.6 (1M), 2026-04-14 late session
**前置阅读 / Prereqs:** `docs/architecture.md`, `docs/repomap-v3-design.md`,
`memory/project_fake_green_audit_2026_04_14.md`,
`memory/project_evidence_as_tool_refactor_deferred.md`

---

## 0. 本文的位置 / Scope of this document

### 中文

本文不是 5 层架构的介绍——那是 `docs/architecture.md` 的任务。本文
回答的是一个更窄、更难、也更重要的问题：

> **在已经修了 5 层架构里可以局部修的所有东西之后，系统为什么还会有
> 假绿？根因的形状是什么？**

为此本文做三件事：
1. 综合三条正交数据通道的全库映射（见 §2 ~ §4）
2. 从"症状集合 → 根因集合"做归纳（§5）
3. 给出 P0 / P1 / P2 三个时间窗的根治路线（§6）

**不做什么**：本文不提供任何"再加一层过滤"、"再写一条 prompt 规则"、
"再跑一次 grid 验证"这类患处再贴药。那些已经够多了——今天 session
的 fake-green audit ship（5 个 commit 从 `951924f` 到 `133973d`）就是
最新一轮。本文反而是在问：**什么时候应该停止加过滤层**？

### English

This document is not an introduction to the 5-layer architecture —
`docs/architecture.md` owns that. This document answers a narrower,
harder, and more important question:

> **After fixing every local thing that can be locally fixed in a 5-layer
> architecture, why does the system still ship fake-greens, and what is
> the shape of the root cause?**

To answer it, the document does three things:

1. Synthesizes a whole-repo map across three orthogonal data channels
   (§2–§4).
2. Induces from the observed symptoms to a small set of root causes (§5).
3. Proposes a P0 / P1 / P2 remediation path over three time windows (§6).

**What this is not**: this is not another "add one more filter", "add
one more prompt rule", "re-run the grid" patch. There have already been
enough of those — today's fake-green ship (5 commits from `951924f` to
`133973d`) is the latest round. Instead this document asks the harder
question: **when should we stop piling on filter layers?**

---

## 1. 背景上下文 / Background

### 中文

codrax 的 5 层管道是：
`Orchestration(1) → Execution(Agents, 2) → Strategy(Skills, 3) → Capability(Tools/MCP, 4) → LLM(5)`

任务从用户请求进入，沿着 `analyze → explore → plan → design_review →
implement → code_review → verify → finalize` 流转。关键约束是
**所有对 Tool / LLM 的调用都必须经过 Layer 2 (Agent)**——
Orchestrator 不能直接 touch Tool 或 LLM。

Explorer agent 是全系统最复杂的一个 evaluator（`internal/agent/explorer.go`
~5600 行），它负责把"用户问题"变成一套结构化的证据
(`[]types.EvidenceItem`)、答案链 (`[]string`)、答案符号
(`[]types.AnswerSymbol`)、和一段给 finalizer 看的合成 prose
(`StageOutput.Data`)。Finalizer agent 读这些，产出最终对用户的回答。

2026-04-14 的 fake-green audit 发现：一个外观上 7/7 PASS 的 grid，
人工检查只有 2-3/7 真的对。四个 pattern（行号幻觉、兄弟方法漂移、
enumeration 坍缩、prose→fact 漂移）跨 3 个 case 反复出现。本 session
做了四层 A/B/C/D + 1 个 parser 修复 `133973d`，post-ship 验证变成
6/7 真绿。

**但这是对症的**。

### English

codrax's 5-layer pipeline is:
`Orchestration(1) → Execution(Agents, 2) → Strategy(Skills, 3) → Capability(Tools/MCP, 4) → LLM(5)`

User requests flow through
`analyze → explore → plan → design_review → implement → code_review →
verify → finalize`. The key invariant is that **every tool / LLM call
must go through Layer 2 (Agent)** — the Orchestrator never touches tool
or LLM directly.

The explorer agent is the most complex evaluator in the system
(`internal/agent/explorer.go`, ~5600 lines). It turns a user question
into a structured evidence list (`[]types.EvidenceItem`), answer chains
(`[]string`), answer symbols (`[]types.AnswerSymbol`), and a free-text
synthesis prose (`StageOutput.Data`) for the finalizer. The finalizer
agent consumes these and produces the final user-facing answer.

The 2026-04-14 fake-green audit found that an outwardly 7/7 PASS grid
was only 2–3/7 actually correct under manual review. Four patterns
(line-number hallucination, sibling-method drift, enumeration collapse,
prose→fact drift) repeated across three cases. This session shipped
four layered patches A/B/C/D plus the `133973d` parser fix, and post-
ship verification improved to 6/7 real.

**But all of that is symptomatic.**

---

## 2. 数据流全图 / End-to-end data flow

### 中文

下图展示一条用户请求的完整数据流。每一跳标注了 **数据形态**：
`struct` = Go 类型，`JSON` = 序列化后的 JSON 字符串，`MD` = markdown
prose，`txt` = 任意自由文本。

```
┌─────────────────────────────────────────────────────────┐
│ USER REQUEST (txt)                                       │
│  ↓ CLI / REPL                                            │
│ BusContext.Request (txt)                                 │
│  ↓                                                        │
│ Analyzer Agent                                            │
│  ├─ LLM turn: classify → AnalyzerHints (struct)          │
│  │             question_kind, answer_shape, keywords,    │
│  │             entities, complexity, high_risk           │
│  └─ todo_write tool call (JSON → struct)                 │
│ BusContext.Mutable.Classification (struct)               │
│  ↓                                                        │
│ Explorer Agent                                            │
│  │ Phase 0: breadth scan                                  │
│  │  ├─ keywordSearch (repomap + grep IDF)                │
│  │  └─ LLM reads repo_map / list_files / grep           │
│  │ Phase 1: depth read                                    │
│  │  ├─ read_file returns `[banner]\n%6d│ <line>` (MD)    │
│  │  ├─ LLM writes investigation notes (MD prose):        │
│  │  │   ## Evidence from `path.go`                       │
│  │  │   - [DIRECT] `func` line 42: description            │
│  │  ├─ parseEvidenceItems (REGEX parser)                 │
│  │  │   → []EvidenceItem (struct)                        │
│  │  ├─ Deterministic producers also write EvidenceItem:  │
│  │  │   scanMechanismEvidence (tree-sitter)              │
│  │  │   extractConcreteValues (regex + parser)           │
│  │  │   dataflow.Analyze (CFG + graph walk)              │
│  │  ├─ groundEvidenceItems — validate LineStart          │
│  │  │   → /ungrounded tag on fail                        │
│  │  ├─ filterEvidenceByPrimaryFiles (struct)              │
│  │  ├─ mergeEvidenceItems + rankEvidenceByRelevance       │
│  │  ├─ identifyAnswerChains → []string                    │
│  │  ├─ extractAnswerSymbols → []AnswerSymbol (struct)     │
│  │  └─ SynthesisPrompt: LLM writes final prose            │
│  │     investigationNotes scrubbed (scrubSibling…Blocks)  │
│  │     → StageOutput.Data = {"result": <prose>} (JSON)    │
│  └─ StageOutput (struct)                                   │
│      EvidenceItems, FlowFindings, AnswerChains,           │
│      AnswerSymbols, NewFacts, SignalUpdates, Data         │
│  ↓                                                         │
│ Orchestrator.applyStageOutput                              │
│  ├─ Merge-dedup into BusContext (all via StableEvidenceID) │
│  ├─ Append StageReport (raw prose from StageOutput.Data)   │
│  └─ BusContext.StageReports[] (MD prose)                  │
│  ↓                                                         │
│ Finalizer Agent                                            │
│  ├─ BuildAgentContext → copies structured fields           │
│  ├─ formatEvidenceItems → rebuilds MD prose from struct    │
│  ├─ Prior Stage Findings = raw prose from StageReports     │
│  ├─ BuildInitialPrompt:                                    │
│  │  if AnswerSymbols != nil: L0-2 translation mode        │
│  │  else: shape-switch soft constraints (prompt-only)     │
│  ├─ LLM produces answer (MD prose)                         │
│  ├─ outOfListSymbols validation (S3, for L0-2 only)        │
│  │  → ContinuationPrompt retry (max 2)                    │
│  └─ StageOutput.FinalAnswer = lastContent (txt)            │
│  ↓                                                         │
│ CLI render → USER                                          │
└─────────────────────────────────────────────────────────────┘
```

重点：**有两条并行的数据通道从 Explorer 流向 Finalizer**：
1. **结构化通道**：`EvidenceItems` / `AnswerChains` / `AnswerSymbols`
   → 走 struct merge-dedup → `formatEvidenceItems` 渲染为 MD
2. **自由文本通道**：`StageOutput.Data` → `StageReport` → 原样作为
   "Prior Stage Findings" 段出现在 finalizer prompt

两条通道的内容**高度重叠但不同步**：LLM 合成 prose 里可以包含
结构化通道已经过滤掉的 sibling cite、幻觉行号、prose→fact 漂移。
过去 session 的修复大多在试图让两条通道"表现一致"，但它们**根本
没有一个公共 schema** 来保证一致。这是系统级漂移的温床。

### English

The diagram above shows the full data flow of a single user request.
Every hop is tagged with its **data shape**: `struct` = Go type, `JSON`
= serialized JSON, `MD` = markdown prose, `txt` = arbitrary free text.

Key insight: **two parallel channels run from Explorer to Finalizer**:

1. **Structured channel**: `EvidenceItems` / `AnswerChains` /
   `AnswerSymbols` flow through struct-level merge-dedup, then get
   rendered back into markdown by `formatEvidenceItems`.
2. **Free-text channel**: `StageOutput.Data` passes through into
   `StageReport` unchanged, reappearing in the finalizer's prompt as a
   "Prior Stage Findings" section.

The two channels carry **heavily overlapping but unsynchronized**
content: the LLM's synthesis prose can contain sibling cites,
hallucinated line numbers, and prose→fact drift that the structured
channel already filtered out. Most prior-session fixes tried to make
the two channels "behave consistently", but **there is no shared
schema forcing that consistency**. This is the breeding ground for
system-level drift.

---

## 3. 结构化进度矩阵 / Structured-progress matrix

### 中文

系统里"结构化端到端"这件事做到哪一步了？下表是一次硬审：

| 子系统 | 结构化程度 | 关键发现 |
|---|:-:|---|
| L0-2 Symbol Translation Mode | ✓ | `finalizer.go:32-67` 已建成；S3 `outOfListSymbols` 硬验证；2 次重试兜底。**这是整个系统里唯一一处端到端带 schema 强制的路径**。 |
| Repo Map / Graph / SymbolID | ✓ | `internal/tool/repomap/types/types.go:27-45` 的 SymbolID 是 canonical drift-proof。Graph 从 tree-sitter 生成，**从不回环给 LLM 让其重构**。 |
| Evidence 确定性生产者 | ✓ | `scanMechanismEvidence` / `extractConcreteValues` / `dataflow.Analyze` / `resolveDefWithReceiver` —— 四条确定性路径，全走 struct。 |
| AnswerSymbol 抽取 | ✓ | `extractAnswerSymbols`（erm.go）读 `EvidenceItem.Subject/Object/Source/LineStart` 字段，不读 Summary 的 prose 文本。 |
| EvidenceItem 去重 | ✓ | `StableEvidenceID`（FNV64a 哈希语义字段）+ `mergeEvidenceItems`。 |
| AnalysisIR v3 | ⚠️ 部分 | `types/analysis_ir.go:27-28` 明确写 **"pure data-model scaffolding… not yet wired into the runtime"**。AnswerContract / AnalyzerHints 已被 explorer/finalizer 消费；TaskGraph / EvidencePlan / RiskMatrix 仍是 free-text `Expr` 字段。 |
| Evidence 主写路径 (LLM notes) | ✗ | `parseEvidenceItems` 正则扫 `investigationNotes` 里的 markdown。无 schema 强制。 |
| Finalizer prose synthesis (legacy shapes) | ✗ | `step_list / value / boolean / config_value` 走 `finalizer.go:86-136` 的 prompt soft constraint，**没有运行时验证器**。只有 `list_of_symbols` 有 L0-2 硬验证。 |
| StageOutput.Data / StageReport prose | ✗ | `fmt.Sprintf(\`{"result": %q}\`, lastContent)`，纯 prose 逃生舱。finalizer 仍然能看到。 |
| Final answer composition | ✗ | LLM 自己写 prose；没有独立的 renderer 层。即使 AnswerSymbol 列表已确定，措辞、排序、citation 格式仍是 LLM 的自由写作。 |

**这个矩阵的形状本身就是诊断**：结构化做得最好的部分（Graph / SymbolID）
**从不把数据回送给 LLM**；结构化做得最差的部分（investigation notes
parsing / StageReport / finalizer prose）**正是 LLM 既写又读的两段**。
这不是巧合——**只要 LLM 既是写者又是读者，就没有 schema 能站得住脚**。

### English

How far has "structured end-to-end" actually progressed in the system?
The table above is a hard audit.

**The shape of the matrix is itself the diagnosis**: the subsystems
that are furthest along in structuralization (Graph / SymbolID) **never
round-trip data back to the LLM**. The subsystems that lag hardest
(investigation notes parsing, StageReport, finalizer prose) are
**precisely the places where the LLM is both the writer and the
reader**. This is not a coincidence — **wherever the LLM is both writer
and reader, no schema can hold.**

---

## 4. 后置过滤层清单 / Post-hoc filter inventory

### 中文

我数了一遍全仓里的后置过滤器 / 验证器 / 清洗器 / 修正器，共
**11 层**：

| 层 | 函数 | 位置 | 捕获的问题 | 捕获后的动作 | 数据通道 |
|---|---|---|---|---|---|
| Request | `stripConversationPrefix` | explorer.go:65 | REPL 内存污染 | 字符串前缀剥离 | string/regex |
| Tool output | `pruneToolHistory` | agent.go:268 | 工具历史 >150KB | 老消息替换为 stub | struct/byte |
| Tool output | `StoreBlob` / `StoreBlobHeadOnly` | blob.go:93,128 | 工具输出 >32KB | 落盘 + 预览 | struct/size |
| Evidence | `groundEvidenceItems` | evidence.go:320 | LLM 行号 cite 错 | 清 LineStart + /ungrounded tag | struct (2-tier validator) |
| Evidence | `mergeEvidenceItems` | evidence.go:237 | 重复 evidence | 按 StableEvidenceID 去重 | struct (hash) |
| Evidence | `rankEvidenceByRelevance` | evidence.go:847 | 无关 evidence | 加权排序 | regex/string |
| Evidence | `scrubSiblingEvidenceBlocks` | evidence.go:633 | sibling 文件 evidence 块 | drop block | string/regex (markdown headers) |
| Evidence | `filterEvidenceByPrimaryFiles` | explorer.go:614 | 非 primary 文件 evidence | drop; 0 命中则 fail-open | struct (source matching) |
| Answer | `outOfListSymbols` + `ContinuationPrompt` | finalizer.go:230,171 | LLM 编造 symbol 名 | 重试最多 2 次，最后接受 | struct (allowed set) |
| Prompt | Explorer Phase 2 Rules bullets | explorer.go:1058-1076 | evidence 格式错 | 通过 prompt 软约束 | string/prompt |
| Prompt | Finalizer Hard constraints | finalizer.go:88-136 | answer shape 违规 | 通过 prompt 软约束 | string/prompt |

**统计**：
- **按通道**：6 条 string/regex/prompt + 5 条 struct/schema
- **按动作**：6 条 drop + 3 条 rewrite + 2 条 retry + 1 条 reorder
- **按所在层**：5 条 evidence 层 + 2 条 prompt 层 + 2 条 tool 层 + 1 条
  answer 层 + 1 条 request 层

**fail-open 风险清单**（默默放过 bug 的）：
- `filterEvidenceByPrimaryFiles`: 0 命中时返回全集（explorer.go:1576）
- `groundEvidenceItems`: /ungrounded tag 不强制 drop，下游 finalizer
  是否实际 honor 取决于 prompt 里的软约束（D commit `f0de900` 的
  提示词）
- `outOfListSymbols`: 2 次重试耗尽后**直接接受**违规响应
  （finalizer.go:166-168）
- LLM 的 investigation notes 里如果写了 prose→fact 漂移（t5 pre-ship
  的 `slice/total` 场景），没有任何 filter 能原生察觉——只能靠
  grounder 的 line 校验间接捕获

### English

I counted every post-hoc filter, validator, scrubber, and corrector in
the repository. **Eleven layers**.

**Stats**:
- By channel: 6 string/regex/prompt + 5 struct/schema
- By action: 6 drop + 3 rewrite + 2 retry + 1 reorder
- By layer: 5 evidence + 2 prompt + 2 tool + 1 answer + 1 request

**Fail-open list** (filters that silently let bugs through):
- `filterEvidenceByPrimaryFiles` returns the full set on 0 hits
- `groundEvidenceItems` tags but does NOT drop; downstream honoring
  is a prompt soft-constraint that the LLM can ignore
- `outOfListSymbols` accepts violations after 2 retries are exhausted
- Pure prose→fact drift inside investigation notes has no native filter
  — the grounder only catches it indirectly via line-cite validation

**Note on direction**: these 11 filters are not a design — they are
accretion. Each one was added to patch a specific failure. None share a
type system, a schema contract, or even a common vocabulary. Two of
them (`filterEvidenceByPrimaryFiles` and `scrubSiblingEvidenceBlocks`)
solve literally the same problem on adjacent data channels because the
first filter can't reach the second channel.

---

## 5. 根因分析 / Root-cause analysis

### 中文

把前四节的证据综合起来，得出 **6 条相互关联的根因**。这些不是 bug
类别，而是结构性选择——每一条都是过去某个决定在今天的代价。

#### 根因 R1：双通道数据流，没有单一真相源

Explorer 往 finalizer 送的是**两条并行通道**：
- 结构化：`StageOutput.{EvidenceItems, AnswerChains, AnswerSymbols}`
- 自由文本：`StageOutput.Data` → `StageReport` 段

`formatEvidenceItems` 会把结构化通道重新渲染为 markdown 插入 finalizer
prompt；自由文本通道原样插入。finalizer LLM 同时看到两份——**高度
重叠但内容可以冲突**。本 session ship 的 **C (`scrubSiblingEvidenceBlocks`)
就是在堵自由文本通道的一个泄漏点**。堵得住这一个，堵不住下一个，
因为两条通道从来没有被做成 schema 对等。

**症状**：t3 pre-ship 同时列 `explorer.go` 的真 evidence 和 `finalizer.go:100,132`
的 sibling drift——结构化通道已过滤但 synthesis prose 没过滤。

**根因**：`StageOutput.Data` 是 free-text 逃生舱，没有任何 schema 说它
必须由结构化字段 deterministic 渲染。只要这个逃生舱还在，所有下游
都得为"prose 通道内容 ≠ struct 通道内容"做兜底。

#### 根因 R2：LLM 既是写者又是读者的段没有 schema

Investigation notes (`e.investigationNotes`) 是 LLM 自己在 ReAct 循环
里写的 markdown，`parseEvidenceItems` 通过正则扫成 `EvidenceItem`。
下一个 turn 里这些 items 又会以 `formatEvidenceItems` 渲染的 markdown
形式**喂回给 LLM**。

这条路径上 **LLM 既是写者又是读者**。它写的时候不知道 parser 的正则
是什么，也不受编译器/schema 约束；它读的时候也不受同样的约束。每次
LLM 的 markdown 风格飘一点（例如本次的"给文件名加反引号"），parser 就
得跟一次 patch——这就是本 session 末尾 `133973d` 的反引号 bug。

**症状**：从 2026-04-13 `c04298f→ba081db` 的 ERM entity 污染，到
2026-04-14 `133973d` 的反引号，parser-class bug 已经两次了。

**根因**：prose 是载流介质。只要 LLM 还在以 markdown 写 evidence
tags，就一定会有第三次、第四次 parser bug。这是结构性的。

#### 根因 R3：Finalizer 约束是 prompt 级的，不是 schema 级的

`finalizer.go:86-136` 的 shape-switch 块是自然语言软约束：
`"The answer is a SET OF IDENTIFIER NAMES. Rules: 1. Every symbol..."`

只有 **`list_of_symbols` 这一个 shape 有 S3 (`outOfListSymbols`) 硬验证**。
`step_list`、`value`、`boolean`、`config_value` 都只有 prompt 里的英文
说明，LLM 可以理解错、可以违反、可以装作没看见。

**症状**：df3/t2 pre-ship 的 "5/9 / 2/9 enumeration 坍缩"——prompt 明确
说"enumerate each branch"，LLM 还是 narrative 坍缩。`D` (f0de900) 加了更
精确的 prompt 措辞让情况改善了，但仍然是**在同一层继续加 prompt**，
没有让 finalizer 运行时实际验证"N 个分支 → N 步"这件事。

**根因**：相信 LLM 会"理解并遵守"自然语言规则，就是系统性信任一个
**demonstrably 不可靠** 的行为。正确的做法是把规则 compile 成验证
函数，在 ReAct 循环里以 `ContinuationPrompt` 强制重试——这正是 L0-2
对 `list_of_symbols` 做的事。其它 shape 还没到这一层。

#### 根因 R4：AnalysisIR 搭了架子没接线

`types/analysis_ir.go:27-28` 明确写：

> // This file is pure data-model scaffolding for batch B1... not yet
> // wired into the runtime.

AnalysisIR v3 设计时的意图是成为 analyze→explore→finalize 三阶段间
的**单一结构化契约**。但实际上：
- `AnswerContract` 里的 `RequiredAnswerShape` 已经用起来（`irAnswerShape`）
- `AnalyzerHints` 里的 `Shape/Kind/Keywords/Entities` 也用起来
- `TaskGraph` / `EvidencePlan` / `RiskMatrix` 仍是 free-text `Expr`
  字段，没有被 downstream 消费
- `HypothesisSet` / `QualityGate` 框架还未绑定到 execution

**症状**：Finalizer prompt 里的 "Prior Stage Findings" 段是 `StageReport`
prose，而不是"IR.EvidencePlan 映射到 actual evidence"的结构化结果。
本该是 IR-contract 的地方，变成了 prose channel。

**根因**：一次 refactor 只做到一半是最坏的结果。现在 IR 既不是
scaffold（因为有些字段在用）也不是 contract（因为有些字段没用）。
这种 "half-structure" 是系统里最难维护的状态。

#### 根因 R5：同一语义事实多处持有

下面列出同一事实出现在多个位置的情况：

| 语义 | 真相源位置 | 二次持有位置 |
|---|---|---|
| Answer shape | `Classification.AnswerShape`（`context.go:98-105`） | `AnalyzerHints.Shape`（`analysis_ir.go:73-78`）、finalizer 读 `irAnswerShape(ctx)`（一个 accessor，OK） |
| Source path | `EvidenceItem.Source`（canonical） | `## Evidence from <path>` markdown 头（被 parser 扫出来）、`logicalFactSource`（从 tool summary 扫）、`allScoredFiles`、`preScannedFiles` |
| Answer symbols | `BusContext.AnswerSymbols`（struct） | `formatEvidenceSymbols` 渲染到 finalizer prompt、 finalizer `BuildInitialPrompt` 又把约束复述一遍 |
| Investigation notes content | `e.investigationNotes []string` | parseEvidenceItems 解出的结构化 EvidenceItems、synthesis prompt 又把 notes 原样插入 |

**症状**：一个 bug 改了一处，另一处没跟上就产生漂移。历史上已经发生
过多次（`c04298f`→`ba081db` 的 `Objective + CurrentTask` 拼接问题就是
这类）。

**根因**：没有 single source of truth 纪律。每次加字段都是 expedient，
没有"这一事实由谁拥有"的答案。

#### 根因 R6：过滤器堆叠顺序 load-bearing 但未显式

11 条 filter 之间有**隐式顺序**：`filterEvidenceByPrimaryFiles` 必须在
`scrubSiblingEvidenceBlocks` 之前，因为前者只管结构化通道，后者在 synthesis
时额外补;`groundEvidenceItems` 必须在 `mergeEvidenceItems` 之前
（否则 /ungrounded 标记影响 ID 哈希）；`rankEvidenceByRelevance` 必须
在 top-18 截断之前。

这些顺序**没有一处文档化**，也没有 graph-level 的依赖检查。新加一层
filter 放错位置可能悄悄 break 已有的隐式依赖。

**症状**：本 session 的 `133973d` 反引号 parser 修复本身没影响顺序，
但如果未来有人在 `parseEvidenceLine` 层加新 filter，很容易破坏
`groundEvidenceItems` 的输入形态。

**根因**：filter 是增量堆叠的，没有统一的 pipeline DAG 描述。

### English (root-cause summaries only; full text in CN above)

**R1 — Two parallel channels, no single source of truth.** Explorer
ships both structured fields and a free-text prose channel
(`StageOutput.Data` → `StageReport`). The finalizer reads both; they
can disagree. Every session's filter additions try to bridge the gap
but never unify the channels. The C patch shipped this session
(`scrubSiblingEvidenceBlocks`) is a band-aid on exactly this gap.

**R2 — No schema where the LLM is both writer and reader.**
Investigation notes are LLM-authored markdown parsed by regex back into
`EvidenceItem` structs. The LLM is both writer (no constraint) and
reader (via `formatEvidenceItems` round-trip). Parser-class bugs are
structurally inevitable on this path — `133973d`'s backtick fix is the
second instance tracked; the deferred `emit_evidence` memo is
counting.

**R3 — Finalizer constraints are prompt-level, not schema-level.**
Only `list_of_symbols` gets S3 validation
(`outOfListSymbols` + `ContinuationPrompt` retry). `step_list`,
`value`, `boolean`, `config_value` are English-prose soft constraints
in `finalizer.go:86-136`. Trusting the LLM to "understand and obey"
natural-language rules is trusting a demonstrably unreliable behaviour.

**R4 — AnalysisIR is a half-wired scaffold.** `types/analysis_ir.go:27-28`
explicitly notes "pure data-model scaffolding… not yet wired into the
runtime." `AnswerContract`/`AnalyzerHints` are wired, but `TaskGraph`,
`EvidencePlan`, `RiskMatrix`, `HypothesisSet`, `QualityGate` are not.
Half-structuralization is worse than either a plain prose channel OR a
committed IR contract.

**R5 — Same semantic fact held in multiple places.** AnswerShape,
Source paths, answer symbol lists, and investigation-notes content all
live in 2–4 locations simultaneously. No explicit ownership → periodic
drift bugs like `c04298f`→`ba081db`.

**R6 — Implicit load-bearing ordering of filters.** 11 filters have
undocumented dependencies between each other (grounder must precede
merge; rank must precede truncate). No pipeline DAG describes this.

---

## 6. 根治方案 / Remediation roadmap

### 中文

下面按时间窗分 P0 / P1 / P2。每一条都回答：**它消灭了哪条根因？**

⚠️ **P0 是本 session 后立即可以做的**——低风险、低成本、高杠杆。
⚠️ **P1 是下一次大 session 的工作**——要协调几个子系统。
⚠️ **P2 是跨 session 的架构路线**——写在 memory 里让后续决策以此为
准。

---

#### P0 — 立即可做（低风险 · 修 R3 / R6 的部分）

**P0.1 - 把 /ungrounded 契约从 prompt 搬到渲染层**
- **目标根因**：R3
- **现状**：本 session D (`f0de900`) 在 finalizer prompt 里加了
  "/ungrounded 条目不报 line number"的软约束；但 `formatEvidenceItems`
  在 `context/builder.go` 渲染 EvidenceItem 到 prompt 时，**不会显式
  标出哪些 item 是 /ungrounded**。LLM 能不能看到 Producer 字段取决于
  渲染逻辑。
- **动作**：`formatEvidenceItems` 在输出行尾追加 `[UNGROUNDED: cite
  without line number]` 标签；LLM 看得见的 prompt 里直接带视觉标记。
- **工作量**：~20 行 + 1 单测
- **杠杆**：D 的契约从 "prompt 说有但渲染不标" 变成 "每一条 evidence
  的 tag 在 prompt 里可见" —— 从软约束升级到可验证

**P0.2 - S3 validator 家族化：为 step_list / value / boolean 加运行时验证**
- **目标根因**：R3
- **现状**：`outOfListSymbols` 是 `list_of_symbols` 的 S3 硬验证；
  其它 shape 没有对应物。
- **动作**：在 `finalizer.go` 增加一套 `validate<Shape>(resp, ctx)`
  函数。例如 `validateStepList` 检查响应里的步数 ≥ evidence 中
  `[CONDITIONAL]` 分支数；`validateValue` 检查响应内含一个且仅一个
  值字面量且来自 `EvidenceConcrete` 集合；`validateBoolean` 检查响应
  开头是 YES/NO。`ContinuationPrompt` 现在只为 L0-2 走，改为**每个
  shape 都走对应 validator**，命中违规走重试。
- **工作量**：每个 shape ~50-80 行 + 每个 shape 2-3 单测；4 个 shape
  大约 300 行 + 10 单测
- **杠杆**：让 Pattern 1（enumeration collapse）**不再依赖 LLM 理解
  英文**。validate 层捕获 → 强制 retry → 有限次后 fail loud（不 silent
  accept）
- **红线**：retry 次数 > 2 时不应 silent accept，应直接 fail/log
  并在 finalizer 输出里打"answer-shape validation exhausted"的诚实
  声明（遵循 `feedback_honesty_over_cleverness.md`）

**P0.3 - 把 11 条 filter 的执行顺序写进一份 FILTERING_ORDER.md**
- **目标根因**：R6
- **现状**：11 条 filter 的相互依赖隐式。
- **动作**：在 `docs/` 下产一份 `filtering-pipeline.md`，画一张 DAG
  描述"谁是谁的输入"，把 fail-open 行为显式列出；代码注释同步
  指向这份文档。不改代码结构，先把现状说清楚。
- **工作量**：~1 小时文档
- **杠杆**：下一次新 filter 加进来时有参照；也让 P1 的"filter → 数据
  流改造"有一个明确的起点快照

**P0.4 - Parser-class bug 第二次计数**
- **目标根因**：R2（做计数，为 P1 的决策做准备）
- **动作**：本 session 的 `133973d` 反引号修复是第 1 次计数；
  `memory/project_evidence_as_tool_refactor_deferred.md` 里明确了
  "第三次 parser-class bug → 重新考虑 refactor"的阈值。**更新那份
  memo 将此计数从 1 变成 2**（把 `c04298f→ba081db` 的 ERM entity 污染
  作为第一次计数；`133973d` 是第二次）。
- **工作量**：5 分钟
- **杠杆**：让 P1 的 emit_evidence refactor 决策变得 data-driven——
  离阈值只剩 1 次 parser bug

---

#### P1 — 中期（1-3 session · 重构小部件 · 修 R1 / R2 / R4）

**P1.1 - emit_evidence 结构化 tool 替换 parseEvidenceItems**
- **目标根因**：R2（彻底消灭）
- **现状**：已在 `memory/project_evidence_as_tool_refactor_deferred.md`
  里写了理由和门槛。
- **动作**：
  1. 新建 `internal/tool/emit_evidence.go`，定义 tool schema 为
     `types.EvidenceItem` 的外部字段（kind/subject/predicate/object/
     source/line_start/line_end/condition/summary）
  2. 修改 explorer phase-2 prompt，改为指示 LLM 在分析完每个文件后
     发一次 `emit_evidence` tool call（batch 版 `emit_evidence_batch`
     也可以，一次 call 含 JSON array）
  3. `parseEvidenceItems` 保留作为 fallback 但打"deprecated"注释；
     新代码不走它
  4. 打开 feature flag 先跑一组 grid 验证 recall 是否下降；
     确认后切默认值
- **工作量**：~300 行代码 + ~20 单测 + 跨 session 的 grid 验证
- **杠杆**：**消灭整个 parser-class bug 家族**。从今天起不会有第 3 次
  `133973d`
- **风险**：
  1. 每条 evidence 一次 tool-call → latency 上涨。解决办法：batch
     版本，一次 tool call 带 array
  2. LLM 可能"一次 call 少写几条" → recall 下降。解决办法：在
     `finalizer.go` 的 retry loop 层加"evidence 数量不足时发送补写
     prompt"的逻辑
- **不做的事**：不要**同时**废掉 markdown 格式；保留 fallback，走
  灰度

**P1.2 - 消灭 StageOutput.Data 自由文本逃生舱**
- **目标根因**：R1
- **现状**：`explorer.go:1654` 塞 `{"result": lastContent}`。finalizer
  通过 `StageReport` 看到原 prose。
- **动作**：
  1. `StageOutput.Data` 改为**由结构化字段 deterministic 渲染**（从
     EvidenceItems + AnswerChains + AnswerSymbols 生成一份 canonical
     markdown，不是 LLM 的 synthesis 结果）
  2. Explorer 的 `SynthesisPrompt` step 是否还需要：如果只是为了让
     explorer 自己做一次 sanity check，可以保留但**不再把 prose 送
     给 finalizer**——explorer 内部用来校准自己的 hasEnoughFacts
  3. Finalizer prompt 的 "Prior Stage Findings" 段改为读 canonical
     rendered content，不再读 `StageReport` prose
- **工作量**：~150 行 + 10 单测 + 整个 grid 跑一遍 regression
- **杠杆**：**R1 被根治**。C commit 的 `scrubSiblingEvidenceBlocks`
  可以被**删除**（不是放宽，是彻底不需要）——因为 synthesis prose
  不再流到 finalizer
- **风险**：Explorer synthesis prose 可能承担了一些"把 evidence
  narrative 化"的价值，丢失后 finalizer 可能 recall 下降。解决办法：
  在 canonical renderer 里加入一些 LLM 的 narrative rationale 字段，
  但**作为结构化字段** (`EvidenceItem.Rationale`)，不是自由 prose

**P1.3 - AnalysisIR 全线路接线**
- **目标根因**：R4
- **现状**：IR 存在但半接线。
- **动作**：
  1. `TaskGraph` / `EvidencePlan` / `RiskMatrix` / `HypothesisSet` /
     `QualityGate` 每一个都要找到 downstream consumer，要么接通，要么
     删字段
  2. Analyzer agent 只写 IR，不再写 `Classification.*` 直接字段；
     后者改成 `irGet*` accessor pattern
  3. Explorer / Finalizer 的**所有**结构化上下文从 IR 读，不从旧字段
     读；旧字段标 deprecated 然后删除
- **工作量**：这是个大活，~1000+ 行 refactor，涉及 8+ 文件，需要一
  整个 session
- **杠杆**：**R5 被根治**（单一真相源纪律强制）；R4 被根治；analyzer
  v4 的路由算法有了干净 input
- **风险**：AnalysisIR 的现有部分（`AnswerContract`、`AnalyzerHints`）
  有 session 内的记忆依赖；迁移时必须遵照
  `memory/project_analyzer_v3_b5_baseline.md` 的 physical-delete 教训

---

#### P2 — 长期（跨 session · 架构级 · 修 R1 全部 / 为 R2 画下限）

**P2.1 - Two-turn explorer：investigation / extraction 分离**
- **目标根因**：R2 彻底解决剩余 LLM-write-read 环
- **现状**：Explorer 的 ReAct 循环里 LLM 既要"读文件"又要"写 evidence
  tag"，两个任务耦合在一个 turn 里。
- **动作**：
  1. **Turn A (Investigation)**: LLM 只调读取工具（grep / read_file /
     repo_map），产出 assistant content 是"这是我读到的内容，结论是
     X"的 narrative（不要求 tag 格式）。
  2. **Turn B (Extraction)**: 新 evaluator，输入是 Turn A 的所有内容
     快照 + 全部读过的文件内容；**LLM 只能调 `emit_evidence` 和
     `emit_answer_symbol` tool，不能再 read_file 或 grep**。
  3. Explorer stage 的 `StageOutput` 完全由 Turn B 的 tool call 填；
     Turn A 的 narrative 只作为 Turn B 的 context，不直接进 finalizer
- **工作量**：架构级，~1500 行 refactor + 一套 evaluator 测试体系
- **杠杆**：
  1. **完全根治 R2**：LLM 作为"写者"的数据只能经 tool schema 进结构；
     不再存在 markdown→regex 的 bridge
  2. 解决 explorer 的 ReAct 循环失控问题（memory 里已经跟踪过多次
     explorer 的早停 / 过读 bug）：两个 turn 的职责清晰分离意味着
     可以对每个 turn 用独立的 ShouldStop 策略
  3. Finalizer 变成真正意义上的 "take structured answer, render to
     user" 函数——它的 prompt 可以从 ~800 行变成 ~100 行
- **风险**：这是一个大 refactor；evaluator 测试体系必须先建好，
  不能边改边调
- **与 P1.1 的关系**：P1.1 是 P2.1 的前置——先把 emit_evidence tool
  建好，再把"Turn B 强制只能用它"加上去

**P2.2 - Answer struct + renderer 层**
- **目标根因**：R1 剩余部分（finalizer 的 prose composition）
- **现状**：Finalizer 的输出是 LLM 的自由 prose；即使 `AnswerSymbols`
  列表已确定，措辞/排序/citation 格式/步数切分仍是 LLM 自由写作。
- **动作**：
  1. 定义 `types.AnswerDocument` 类型，字段可能包括 `Summary` string、
     `Steps []AnswerStep`、`Symbols []AnswerSymbol`、`Citations []Citation`、
     `Caveats []string` 等
  2. Finalizer 的 LLM 只 emit `AnswerDocument` 结构（通过 tool call）
  3. 一个独立的 `internal/render/answer.go` 把 `AnswerDocument` 渲染
     成用户 prose，按语言（zh / en / off）选择措辞
  4. `outOfListSymbols` 等验证器改成 struct-level 验证——直接检查
     `AnswerDocument.Symbols` 是 `ctx.AnswerSymbols` 的子集
- **工作量**：架构级，~1200 行
- **杠杆**：
  1. Pattern 1（enumeration collapse）在结构层**不可能发生**：`Steps`
     是一个 slice，有几个分支就有几个元素
  2. Pattern 2/4（line 幻觉、prose→fact）在结构层**不可能发生**：
     `Citation.Line` 是 int 且经 P1.1 grounder 验证过
  3. Pattern 3（sibling drift）在结构层**不可能发生**：`Citation.File`
     必须在 `ctx.AllowedFiles` 列表里
  4. 多语言 finalizer 不再需要 prompt 里写 "respond in Chinese"——
     renderer 层按 `ctx.Lang` 切换
- **风险**：自然语言措辞的表达力损失。LLM 自由写 prose 可以有风格、
  连贯性、解释——renderer 模板化的 prose 可能比较机械。解决办法：
  `AnswerStep.Rationale` 是 LLM-authored prose 字段，但**局限在这一
  个字段里**；其它字段都是结构化

**P2.3 - Orchestrator state 类型化**
- **目标根因**：R5 + R6 的剩余部分
- **现状**：`BusContext.Mutable` 是一个混杂的 struct + map 状态
  容器；state-machine transition 是基于 signal 的 priority 权重。
- **动作**：把 orchestrator 的 state 做成明确的 Go state-machine 类
  型（state / event / transition），用 compiler 强制检查所有 transition
  都 exhaustive；signal-based transition 改成 explicit guards。
- **工作量**：中-大，~800 行
- **杠杆**：过滤器堆叠顺序问题（R6）从"读 11 个函数的注释"变成
  "看一张 state diagram"。新 filter 加进来必须声明自己的输入状态；
  compiler 会在不声明时报错
- **优先级**：P2 里最低优先——orchestrator 的漂移类 bug 不多，
  P2.1 + P2.2 已经解决了大部分可见症状

---

### English (summary only; full CN text above)

**P0 — Immediate, low-risk, high-leverage** — each ≤1 session:
- P0.1 Move the `/ungrounded` contract from prompt to renderer
  (`formatEvidenceItems` tags items visibly). Target: R3.
- P0.2 Extend S3-style structural validators to `step_list` / `value`
  / `boolean` / `config_value` shapes. Target: R3.
- P0.3 Document the 11 filters' implicit execution order in
  `docs/filtering-pipeline.md`. Target: R6.
- P0.4 Update the parser-class bug counter in
  `memory/project_evidence_as_tool_refactor_deferred.md` from 1 to 2
  (counting `c04298f→ba081db` as the first instance and `133973d` as
  the second). Target: R2 (decision data).

**P1 — Medium-term, 1–3 sessions, component-level refactors**:
- P1.1 Ship `emit_evidence` structured tool, deprecate
  `parseEvidenceItems`. Target: R2 (closed).
- P1.2 Replace `StageOutput.Data` free-text escape hatch with a
  deterministic rendering of structured fields; C's
  `scrubSiblingEvidenceBlocks` becomes unnecessary. Target: R1.
- P1.3 Wire AnalysisIR fully end-to-end; remove all legacy
  `Classification.*` direct reads. Target: R4 and R5.

**P2 — Long-term, cross-session, architectural**:
- P2.1 Two-turn explorer: Turn A (investigation with read tools), Turn
  B (extraction with only `emit_evidence` / `emit_answer_symbol`).
  Target: R2 fully closed; explorer ReAct discipline.
- P2.2 `AnswerDocument` struct + `internal/render/answer.go` renderer;
  finalizer LLM emits struct only; Patterns 1/2/3/4 become structurally
  impossible. Target: R1 closed at the answer layer.
- P2.3 Orchestrator state machine typed as explicit Go state/event
  types; filter pipeline as a DAG with compiler-checked dependencies.
  Target: R6 fully closed.

---

## 7. 实施红线 / Implementation guardrails

### 中文

执行 P0 → P1 → P2 路线时必须严格遵守以下红线。这些都是本 repo 的
记忆里反复强调过的原则——违反任何一条都会让根治变成另一个"对症
patch"。

1. **过拟合审计 gate**（`memory/feedback_no_overfitted_solutions.md`）
   每个 P 项 **在设计前 + 在编码前** 都要跑一遍 5 问审计。任何
   "只为了让某个 failing case 过"的写法直接 fail。
2. **诚实 > 聪明**（`memory/feedback_honesty_over_cleverness.md`）
   P0.2 的 validator 在 retry 耗尽后必须 fail loud，不能 silent accept。
   P1.1 的 emit_evidence schema 遇到未知字段必须拒绝，不能"尽力
   tolerance"。
3. **没有硬编码词表**（`memory/feedback_overfitting_audit_stopwords.md`）
   P0 / P1 / P2 里任何新增的过滤 / 验证 / 排序逻辑都不得用硬编码
   英文词表；只能用结构化判据（CamelCase / 含下划线 / 在 graph 里）。
4. **不用 grid PASS 当 ship gate**（本 session fake-green audit 的
   DO NOT 清单）
   P1.x / P2.x 的验证必须包含**手动抽查**一部分案例的答案质量；
   substring PASS 不足以判绿。
5. **全数据流审计再下结论**（`memory/feedback_trace_full_dataflow_before_fixing.md`）
   任何 P 项在宣布完成前要 trace 一次完整数据流——tool output →
   LLM → evidence → BusContext → finalizer → answer——确认新改动
   在每一跳的行为符合预期。
6. **不加新 CLAUDE.md 以外的配置渠道**（`CLAUDE.md` 现有的三文件配置
   边界要守住）
   P1.3 的 AnalysisIR 接线不得引入新的 yaml / flag 之外的配置通道。
7. **不用硬变通绕过类型系统**
   P2.1 / P2.2 的 refactor 过程中出现"暂时用 `interface{}` / `any` /
   断言"的诱惑时必须立刻停下来找根因。这类 patch 会让 P2 变成
   P3。

### English

When executing the P0 → P1 → P2 path, the following hard guardrails
apply. Every one is a principle this repository's memory has
repeatedly reinforced; violating any of them turns remediation into
yet another symptomatic patch.

1. **Over-fitting audit gate** (`feedback_no_overfitted_solutions.md`):
   every P-item runs the 5-question audit **before design AND before
   code**. Anything that only makes a specific failing case pass is
   rejected.
2. **Honesty over cleverness** (`feedback_honesty_over_cleverness.md`):
   P0.2 validators must fail loud on retry exhaustion; never silently
   accept. P1.1 `emit_evidence` schema must reject unknown fields;
   never "best-effort tolerate".
3. **No hardcoded word lists**
   (`feedback_overfitting_audit_stopwords.md`): all new filtering /
   validation / ranking logic must decide on structural predicates
   (CamelCase / underscore / graph symbol membership), never a curated
   English list.
4. **Don't use grid PASS as a ship gate** (this session's fake-green
   DO NOT list): every P1/P2 validation MUST include manual inspection
   of a sample of answers; substring PASS is insufficient.
5. **Full data-flow trace before declaring done**
   (`feedback_trace_full_dataflow_before_fixing.md`): tool → LLM →
   evidence → BusContext → finalizer → answer; verify every hop
   behaves as expected.
6. **No new config channels beyond CLAUDE.md's three files**: P1.3
   must not introduce yaml / flag paths outside the established set.
7. **No type-system escape hatches**: if during P2.1 / P2.2 there is a
   temptation to "temporarily use `interface{}` / `any` / a type
   assertion", STOP and find the root cause. That kind of patch turns
   P2 into P3.

---

## 8. 不做什么 / What NOT to do

### 中文

1. **不要再加第 12 条 filter**。即使当下看起来能修一个 bug，也会让
   P1.2 的 "R1 R2 根治" 更难做。今天 session 的 D (`scrubSiblingEvidenceBlocks`)
   本身也是一个 band-aid——P1.2 上线后应该被**删除**。
2. **不要把 P1.1 跳过 P0**。P0 的 validator 家族化是一个 feedback
   loop：它让系统在还没 emit_evidence refactor 的前提下就能**量化**
   parser-class bug 的影响；这个量化数据是 P1.1 决策的依据。跳过
   P0 直接做 P1.1 = 凭感觉做 refactor。
3. **不要一次性 rewrite 整个 explorer**。P2.1 的 two-turn 设计
   需要先在 P1.1 + P1.2 基础上跑一次 grid 有良好数据，再动 explorer
   core。explorer 是全系统最复杂的 evaluator（~5600 行），从头重写
   ≈ 重写整个 fake-green audit 的判断依据。
4. **不要回退 2026-04-14 的 5 个 ship commit**（`951924f` 到
   `133973d`）。它们是 P0/P1 做完之前对真实 bug 的**唯一防护**。
   只有 P1.2 上线后 C commit 才能删除，P2.1 上线后 A commit 的
   gutter 机制才可以重新评估。
5. **不要把本文当 roadmap commit**。本文是分析稿 + 路线图，不是一次
   session 的 plan。每个 P 项应该**独立** open 一次 session 去做，
   带独立的 design review、test plan、grid 验证。硬把 P0.1+P0.2+P0.3
   塞一次 session 做 ≈ 又一次"本周先把能修的都修了"——正是今天
   本 session 想要反对的模式。

### English

1. **Do not add a 12th filter**. Even if it fixes a bug today, it makes
   P1.2's root-cause remediation harder. D's `scrubSiblingEvidenceBlocks`
   shipped this session is itself a band-aid — it should be **deleted**
   after P1.2 lands.
2. **Do not skip P0 and jump to P1.1**. The P0 validator family acts as
   a feedback loop: it quantifies parser-class bug impact BEFORE the
   emit_evidence refactor, and that quantification is the data
   underpinning the P1.1 decision. Skipping P0 = refactoring on
   intuition.
3. **Do not one-shot rewrite the explorer**. P2.1's two-turn design
   needs good grid data from P1.1 + P1.2 first. The explorer is ~5600
   lines of the most complex evaluator in the system; rewriting from
   scratch is equivalent to rewriting the basis of every fake-green
   audit judgment.
4. **Do not revert the 5 shipped commits** from 2026-04-14 (`951924f`
   through `133973d`). They are the only protection against real bugs
   until P0/P1 land. C can only be deleted after P1.2; A's gutter
   mechanism can only be re-evaluated after P2.1.
5. **Do not treat this document as a single-session roadmap commit**.
   This is an analysis doc + roadmap, not a session plan. Each P-item
   should open its **own** session with its own design review, test
   plan, and grid verification. Cramming P0.1 + P0.2 + P0.3 into one
   session is another "fix everything fixable this week" iteration —
   exactly the pattern this session was supposed to argue against.

---

## 9. 验证策略 / Verification strategy

### 中文

每个 P 项完成时用哪些信号判断"真的做对了"？

| 项 | 可观测信号 | 阈值 |
|---|---|---|
| P0.1 | `formatEvidenceItems` 输出中 UNGROUNDED tag 数 / 总 item 数 | 单 case 中 UNGROUNDED tag 存在 ⇒ finalizer 答案里对应 cite 必须没有 line number（0 例外） |
| P0.2 | 每个 shape 的 validate_<shape>_retries 计数 | < 0.1 次 / case 的 retry（target），< 0.5 次 / case（ceiling），> 0.5 ⇒ validator 太严 |
| P0.3 | （文档项，无代码信号） | 所有 11 条 filter 在 DAG 里有 predecessor + successor 标记 |
| P0.4 | memory 文件内容 | parser-class bug 计数 = 2 |
| P1.1 | `emit_evidence` 调用次数 vs `parseEvidenceItems` 调用次数 | feature flag off → 全部走 parse；on → 全部走 emit；切换期间两条路径并行跑一周 |
| P1.2 | C commit 的 scrubSiblingEvidenceBlocks 被 delete | 代码 grep "scrubSiblingEvidenceBlocks" 返回 0 条 |
| P1.3 | `grep -r "Classification\." internal/` 返回数 | 降到只剩 accessor (`irGet*`) |
| P2.1 | Turn B 里 LLM 的 tool_call 分布 | 100% 是 emit_evidence / emit_answer_symbol；0% 是 read_file / grep |
| P2.2 | `StageOutput.FinalAnswer` 的类型 | 从 `string` 变成 `*types.AnswerDocument` |
| P2.3 | filter 依赖关系在 compiler 级有 check | 加一条未声明 predecessor 的 filter 应该编译失败 |

**对答案质量的横向测量**：
- 每个 P 项完成后跑一次 1×7 grid（**只作为回归保障**，不作为 ship
  gate）。
- 手动 inspect 每个 case 的答案质量，对照 2026-04-14 ship 后的
  baseline（6/7 real）。**P1.2 应该把 6/7 推到 7/7**；**P2.1 + P2.2
  应该把 fake-green 的结构可能性降到零**。

### English

For every P-item, the observable signal for "actually done":

**Cross-cutting answer-quality measurement**: after each P-item, run a
1×7 grid **as a regression safety net, not a ship gate**. Manually
inspect each case against the 2026-04-14 baseline (6/7 real). **P1.2
should push 6/7 → 7/7**. **P2.1 + P2.2 should reduce fake-green to a
structural impossibility.**

---

## 10. 总结 / Conclusion

### 中文

codrax 的 fake-green 问题**不是某一层的 bug**，而是 6 条结构性根因
叠加的结果：两条数据通道并行、LLM 既写又读的 prose 段没有 schema、
finalizer 约束停留在 prompt 层、AnalysisIR 半接线、同一事实多处持有、
filter 堆叠顺序隐式。

过去的修复几乎全是"给受影响的一层加一个过滤器 / prompt 规则 /
parser patch"——今天 session 的 fake-green ship 也不例外。这些修复
在**症状层**有效，但在**根因层**没有触及任何一条。

本文提出的 P0 / P1 / P2 路线通过**改变数据流拓扑**而不是**添加拦截
层**来根治问题：
- P0 让现有的软约束变成可观测信号，为 P1 决策提供数据
- P1 把"LLM 既写又读 prose"的关键两段换成结构化 tool 通道，删除自由
  文本逃生舱，完成 AnalysisIR 接线
- P2 把 explorer 拆成 investigation/extraction 两 turn，引入结构化
  Answer 层，把 orchestrator state 类型化

**上述每一项落地时** fake-green audit 里的 4 个 pattern 会按下表
消失：

| pattern | 今天的现状 | P0 后 | P1 后 | P2 后 |
|---|---|---|---|---|
| 1 (step_list 坍缩) | D prompt 软约束 | P0.2 validator 强制 | 不变 | 结构上不可能 |
| 2 (行号幻觉) | A gutter + B grounder | 可观测（UNGROUNDED 可见） | P1.1 parse 通道消失 | 结构上不可能 |
| 3 (sibling drift) | C scrub 兜底 | 不变 | P1.2 逃生舱删除 → C 可以删除 | 结构上不可能 |
| 4 (prose→fact) | A + B 间接覆盖 | 不变 | P1.1 直接消灭 prose 通道 | 结构上不可能 |

**期望终态**：P2 完成时，fake-green 不是"通过多层拦截降到极低
频"，而是"在数据模型层面不存在可能性"。

从加过滤层到修拓扑——这是今天 session 的交付不包含、但整个系统
未来几 session 应该走的路。

### English

codrax's fake-green problem is **not a bug in any single layer**. It
is six structural root causes layered on top of each other: two
parallel data channels, no schema for the LLM's write+read prose
section, prompt-level finalizer constraints, half-wired AnalysisIR,
multiple ownership of the same semantic fact, implicit filter-stack
ordering.

Nearly every prior fix has been "add one more filter / prompt rule /
parser patch to the affected layer" — today's fake-green ship is no
exception. These fixes are effective at the **symptom** layer but
don't touch any of the **root** causes.

This document's P0 / P1 / P2 path fixes the problem by **changing the
data-flow topology**, not by **adding interception layers**:

- **P0** turns the soft constraints into observable signals and
  provides the data underpinning the P1 decision.
- **P1** replaces the critical "LLM writes and reads prose" segments
  with structured tool channels, deletes the free-text escape hatch,
  and completes the AnalysisIR wiring.
- **P2** splits the explorer into investigation / extraction turns,
  introduces a structured Answer layer, and types the orchestrator
  state.

**When each stage lands**, the four fake-green audit patterns will
disappear on the following schedule:

| pattern | today | after P0 | after P1 | after P2 |
|---|---|---|---|---|
| 1 (step_list collapse) | D prompt-level | P0.2 validator-enforced | unchanged | structurally impossible |
| 2 (line hallucination) | A gutter + B grounder | observable (UNGROUNDED visible) | P1.1 parse channel gone | structurally impossible |
| 3 (sibling drift) | C scrub net | unchanged | P1.2 escape hatch deleted → C removable | structurally impossible |
| 4 (prose→fact) | A + B indirect cover | unchanged | P1.1 prose channel killed | structurally impossible |

**Desired end state**: when P2 is complete, fake-greens are not "very
low frequency due to multi-layer interception" but "structurally
impossible at the data-model layer".

From adding filter layers to fixing the topology — this is the path
that today's session does not deliver, but that the next several
sessions should walk.

---

## 附录 / Appendices

### A. 今天 session 的 ship 清单 / Today's ship list

5 commits from `f8bea7c` to `133973d`, in order:

| commit | layer | summary |
|---|---|---|
| `951924f` | A — tool | `read_file` emits `%6d│ ` per-line gutter |
| `14e2c07` | B — evidence | `groundEvidenceItems` 2-tier validation with /ungrounded tag |
| `d5daa27` | C — explorer | `scrubSiblingEvidenceBlocks` in SynthesisPrompt |
| `f0de900` | D — finalizer | step_list prompt: ALTERNATIVE BRANCHES + /ungrounded contract |
| `133973d` | fix — parser | strip markdown backticks in `parseEvidenceHeaderSource` |

### B. 关键记忆文件索引 / Key memory file index

| memo | purpose |
|---|---|
| `project_fake_green_audit_2026_04_14.md` | Full ship record, patterns 1-4, 6/7 real verification |
| `project_evidence_as_tool_refactor_deferred.md` | P1.1 precondition + parser-class bug counter |
| `feedback_honesty_over_cleverness.md` | P0.2 / P1.1 guardrail |
| `feedback_no_overfitted_solutions.md` | Pre-design + pre-code audit gate |
| `feedback_overfitting_audit_stopwords.md` | No hardcoded word lists |
| `feedback_trace_full_dataflow_before_fixing.md` | P-item completion criterion |

### C. 为什么不直接跳到 P2 / Why not jump straight to P2

1. P2.1 / P2.2 的 refactor 需要 **数据** 支持它们的假设。没有 P0.2
   的 validator 计数和 P1.1 的 emit_evidence 统计，P2 的取舍会退化
   成 gut-feel engineering。
2. P2 的任何一项失败回退都需要**中间稳定态**。P0 / P1 的每一步都
   是一个可 ship 的稳定态，P2 的重写不是。跳过 P0/P1 等于没有回退
   路径。
3. P2 需要的测试体系比今天更重。evaluator 级的 golden test 需要在
   P1.1 上线后建起来，P2.1 才能安全地替换 evaluator。

---

**文档状态 / Document status**: 分析稿 / analysis draft. Next review
date: 当 P0 或 P1 的任一项进入实施时 / when any P0 or P1 item enters
implementation.

**反馈通道 / Feedback channel**: `docs/architecture-root-cause-remediation.md`
的后续 edit 应该走 PR review；session 内的 ad-hoc 修改请先在 session
笔记里记录理由再改正文。
