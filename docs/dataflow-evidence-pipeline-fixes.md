# 跨语言数据流证据管道修复全记录

## 一、问题现象

### 测试场景

对 codrax 项目自身运行真实查询：

```
有多少个agent可以调用subagent? 请列出每个agent的名称和它能调用的subagent类型
```

**正确答案**：只有 1 个 agent（Explorer）可以调用 subagent（SubExplorer）。

依据的三跳推理链：
1. `RegisterDefaultSubAgents`（subagent.go:63）只注册了 `NewSubExplorer`
2. `SubExplorer.Name()`（sub_explorer.go:32）返回 `"explorer"`
3. `agent.go:465` 的 `SubAgents.Get(string(b.name))` 只匹配 name 相同的 agent → 只有 name=="explorer" 的 AgentExplorer 能调用

### 实际表现

| 阶段 | 系统回答 | 关键缺陷 |
|------|---------|---------|
| 原始代码 | "共有 3 个 agent" / "8 个 agent" | 完全错误 |
| 修复后（提交 1-2） | "多种 agent" / "2 个 agent" | 接近但不精确 |
| 修复后（提交 3-4） | "2 个：Orchestrator + SubAgentRegistry" | 核心实体正确但推理链不完整 |
| 修复后（提交 5-6） | "AgentExplorer 能调用 SubExplorer，Name()返回 explorer" | **三跳链完整** |

---

## 二、一句话根因

> 确定性分析层产出的事实（concrete values、resolution chains）散落在 synthesis prompt 文本中，不作为结构化证据流入下游 finalizer；同时 evidence 生成受 markdown 表格 cap（25 条）截断，关键的多跳推理事实被淹没在噪声中。

---

## 三、系统完整流程与断裂点分析

### 3.1 系统架构流程

```
用户问题
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│ Analyzer Agent (Stage: analyze)                         │
│   分析问题复杂度，提取关键词，生成任务描述                     │
│   输出：task keywords, task description                   │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│ Explorer Agent (Stage: explore)                         │
│                                                         │
│  Phase 0: Keyword Search                                │
│    keyword_search → scored files + preScannedFiles(8)   │
│                                                         │
│  Phase 1: LLM Investigation (ReAct Loop)                │
│    LLM 自主选择 grep/read_file → investigation notes     │
│                                                         │
│  ContinuationPrompt: 催读 preScannedFiles，检测覆盖率       │
│                                                         │
│  ensureStructuredEvidence:                              │
│    ├─ parseEvidenceItems (从 LLM notes 提取)             │
│    ├─ needsDataflowAnalysis → dataflow.Analyze          │
│    └─ mergeEvidenceItems                                │
│                                                         │
│  SynthesisPrompt:                                       │
│    ├─ Evidence Catalog (LLM notes)                      │
│    ├─ buildConcreteValuesSection → markdown table ←──── │─── 确定性事实在此
│    ├─ Resolution Chains                                 │
│    ├─ Cross References                                  │
│    └─ Reasoning Instructions                            │
│                                                         │
│  Synthesis LLM Call → 综合回答                            │
│                                                         │
│  ParseOutput → StageOutput:                             │
│    ├─ Data (synthesis 结果)                              │
│    ├─ EvidenceItems ←───────────────────────────────── │─── 结构化证据
│    ├─ FlowFindings                                      │
│    └─ StageReport                                       │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│ Orchestrator: applyStageOutput                          │
│   BusContext.EvidenceItems += output.EvidenceItems       │
│   BusContext.FlowFindings += output.FlowFindings         │
│   BusContext.StageReports += output.StageReport          │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│ Context Builder: BuildPromptContext                      │
│   formatEvidenceItems(top 18) → "## Structured Evidence"│
│   formatFlowFindings(top 10) → "## Dataflow Findings"   │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│ Finalizer Agent (Stage: finalize)                       │
│   读取 Prior Stage Findings + Structured Evidence        │
│   + Dataflow Findings → 生成最终回答                      │
└─────────────────────────────────────────────────────────┘
```

### 3.2 发现的六个断裂点

在上述流程中，我们逐步发现了六个信息流断裂点：

#### 断裂点 ❶：中文问题不触发 dataflow 引擎

**位置**：`evidence.go:369` `needsDataflowAnalysis`

**现象**：用户问 "有多少个agent可以调用subagent?"，dataflow 引擎完全未被调用。

**原因**：`needsDataflowAnalysis` 的关键词列表只有英文（flow/path/trigger/registered 等），中文问题中的"调用""多少"不在匹配范围内。

**日志佐证**：
```
(无 dataflow.Analyze 相关日志)
```

#### 断裂点 ❷：Synthesis prompt 超出 LLM context window

**位置**：`explorer.go` `SynthesisPrompt` → `agent.go:378` LLM.Chat

**现象**：Synthesis prompt 达到 708KB，超出 gpt-4o 的 128K context window。

**原因**：`buildConcreteValuesSection` 扫描 32 个文件提取了 1156 个 concrete values（过滤后 619 个），加上 investigation notes、cross-references 等各 section 独立 cap 但无全局预算，总计 708KB。

**日志佐证**：
```
SYNTHESIS prompt len=708176
SYNTHESIS failed: context_length_exceeded
```

#### 断裂点 ❸：Evidence/Findings 不按问题相关性排序

**位置**：`engine.go:416` `mergeEvidenceItems` 按 `(source, lineStart, ID)` 字典序排序；`engine.go:344` `digestFindings` 按硬编码 confidence 排序

**现象**：9434 条 evidence 取 top-18 显示给 finalizer，但 top-18 中全是无关项（Logger、ListFiles 等）。1316 个 findings 取 top-10，全是无关跨函数调用链。

**原因**：排序不看问题实体。evidence 按文件路径字典序，findings 按硬编码 confidence（0.81/0.84）——所有 finding 的 confidence 几乎相同，无区分度。

**日志佐证**：
```
## Structured Evidence
- dataflow candidate set exceeded budget and was truncated to 40 files
- `BaseAgent` → `SubAgentRegistry`...  (无关)

## Dataflow Findings  
- cmd/root.go:runSingleShot -> internal/render/renderer.go:...  (无关)
- internal/agent/keyword_search.go:expandKeywords -> ...  (无关)
```

#### 断裂点 ❹：LLM 文件选择不受问题语义驱动

**位置**：`explorer.go` Phase 1 LLM Investigation

**现象**：`sub_explorer.go`（包含 `SubExplorer.Name() returns "explorer"`）在 keyword search 中排名 #6（repo_map=566），在 preScannedFiles 中，但 LLM 反复选择读 `explorer.go`（3408 行，读了 3 次）而不读 66 行的 `sub_explorer.go`。

**原因**：LLM 的文件选择完全自主，不受 keyword search ranking 约束。preScannedFiles 的 3 级催读机制依赖 idle streak 触发，但 LLM 持续调用工具时 idle streak 不增长，催读不触发。

**日志佐证**：
```
iter=8  read_file internal/agent/explorer.go offset=0
iter=9  read_file internal/agent/explorer.go offset=50
iter=10 read_file internal/agent/explorer.go offset=100
(sub_explorer.go 从未被 read_file)
```

#### 断裂点 ❺：确定性事实困在 Synthesis 文本中不进 StageOutput

**位置**：`explorer.go` `buildConcreteValuesSection` 返回 `string`，只注入 synthesis prompt

**现象**：`RegisterDefaultSubAgents binds NewSubExplorer` 和 `SubExplorer.Name returns "explorer"` 在 synthesis prompt 的 Concrete Values 表格中存在，但不在 `StageOutput.EvidenceItems` 中。当 synthesis LLM 不利用或 synthesis 失败时，这些确定性事实丢失。

**原因**：`buildConcreteValuesSection` 只返回 markdown 字符串给 synthesis prompt，不生成 `[]types.EvidenceItem`。确定性分析与结构化证据管道之间没有数据通路。

**日志佐证**：
```
(finalizer 的 ## Structured Evidence 中无 RegisterDefaultSubAgents 相关条目)
(finalizer 的 ## Structured Evidence 中无 SubExplorer.Name 相关条目)
```

#### 断裂点 ❻：Evidence 生成受 markdown cap 截断

**位置**：`explorer.go` `buildConcreteValuesSection` 中 `valueCap=25` 在 evidence 生成之前

**现象**：891 个 relevant concrete values 排序后 cap 到 25 条。所有 bindings（score=100）占满 25 个名额，short string returns 如 `SubExplorer.Name returns "explorer"`（score=80）被完全截断。evidence 从 capped 的 25 条中生成，关键事实丢失。

**原因**：`valueCap` 是为控制 synthesis markdown 大小设计的，但 evidence 生成代码在 cap 之后执行，共享了同一个截断后的 `relevant` 切片。evidence pipeline 有自己的下游 ranking（`rankEvidenceByRelevance`）和 limit（`formatEvidenceItems` top-18），不需要被 markdown 预算限制。

**日志佐证**：
```
concrete values: 891 relevant after multi-pass tracing
(cap 后只剩 25，其中全是 bindings)
FILTERED-OUT SubExplorer.Name returns "explorer" (sub_explorer.go:31) preScanned=false read=false
RELEVANT RegisterDefaultSubAgents binds ONLY NewSubExplorer(deps) (subagent.go:63) preScanned=true
```

---

## 四、逐个修复的解决思路与方案

### 提交 1：`e3168f9` — 修复 dataflow lowerer 的模式覆盖

**思路**：dataflow engine 的 lowering 层对某些代码模式识别不全（Java `config.getString()` 不匹配、字段读写不产出证据），需要扩展模式覆盖。

**方案**：
- `lower.go:284`：config 检测正则从 `config(?:\.get)?` 扩展为 `config(?:\.get\w*)?`，覆盖 `getString`/`getInt`/`getBoolean` 等 Java 变体
- `lower.go:152-176`：字段写入生成 `EvidenceRelationship`（predicate=`writes_field`），字段读取生成 `EvidenceRelationship`（predicate=`reads_field`）
- 新增 16 个 e2e 测试场景覆盖 Go/Python/JS/TS/Rust/Java/C + 配置文件 + 真实仓库自分析

**为什么能解决**：之前只有 return/call/guard/config-read 四种模式产出 evidence，字段操作和 Java 风格的 config 读取被遗漏。扩展后 lowerer 的模式覆盖更完整，减少了确定性分析层的盲区。

---

### 提交 2：`03c0861` — 中文触发 + synthesis 预算 + 证据排名

**思路**：三个独立的断裂点（❶❷❸）需要同时修复，因为任何一个都会阻断数据流。

**方案 A — 中文触发**（修复断裂点 ❶）：

`evidence.go:369` `needsDataflowAnalysis` 新增 18 个中文关键词和 3 个英文关键词：
```go
// Chinese — equivalent dataflow / registration / invocation concepts
"调用", "注册", "触发", "配置", "流向", "传播", "条件",
"路由", "处理器", "绑定", "分发", "哪些", "多少", "列出",
"哪个", "谁会", "谁能", "怎么",
```

**为什么能解决**：中文问题现在能触发 dataflow 引擎。"有多少个agent可以调用subagent?" 中的"调用"和"多少"命中关键词。

**方案 B — Synthesis 预算**（修复断裂点 ❷）：

`explorer.go` `SynthesisPrompt` 末尾新增全局 120KB 预算控制：
```go
const synthBudgetBytes = 120_000
if len(result) > synthBudgetBytes {
    result = truncateSynthesisPrompt(result, synthBudgetBytes)
}
```

`truncateSynthesisPrompt` 按优先级从低到高逐步移除 section：
1. Unread Important Files
2. Unresolved Conditions
3. Negative Evidence
4. Focus Misalignment Warning
5. Enumeration Completeness
6. Cross-References
7. Type Hierarchy Chains
8. Resolution Chains

通过 `removeMarkdownSection` 按 markdown heading 级别精确移除整个 section。如果仍超预算，硬截断并标注。

**为什么能解决**：708KB prompt 被截断到 ~22KB，gpt-4o 能正常处理。优先保留高价值 section（Concrete Values、Evidence Catalog、Reasoning Instructions），牺牲低价值 section。

**方案 C — 证据排名**（修复断裂点 ❸）：

新增两个排名函数：

`rankEvidenceByRelevance(question, items, readFiles)`:
```
relevance = entityOverlap × kindWeight × sourceWeight × bridgeBonus
```
- **entityOverlap**：从问题中提取实体（`extractRankingEntities`），计算 item 的 subject/object/summary 中命中比例
- **kindWeight**：concrete(1.0) > registration(0.95) > relationship(0.8) > mechanism(0.7) > conditional(0.6)
- **sourceWeight**：explorer 读过的文件 1.0，其他 0.5
- **bridgeBonus**：subject 和 object 命中不同问题实体时 ×2（桥接证据）
- **Diversity 约束**：同一 (source, subject) 最多 2 条

`rankFindingsByRelevance(question, findings)`:
```
relevance = pathEntityOverlap × chainBrevity × confidence
```
- **pathEntityOverlap**：path 节点命中问题实体的比例
- **chainBrevity**：`1/len(path)` — 短链比长链更精准
- **confidence**：保留原有值

排名在 explorer `ParseOutput` 和 `SynthesisPrompt` 中调用，在存入 StageOutput 前排序。

**为什么能解决**：9434 条 evidence 中与 "agent/subagent" 相关的条目被推到前面。无关的 Logger/ListFiles 条目因 entityOverlap=0 沉底。Finalizer 看到的 top-18 条目中关键事实占比大幅提升。

---

### 提交 3：`4cc1f2d` — Evidence Requirement Model (ERM)

**思路**（修复断裂点 ❹）：系统各组件各自为政——keyword search 知道哪些文件相关但不知道需要什么证据；LLM 知道需要什么但不被 ranking 约束；确定性层知道代码的确切值但不知道哪些对问题重要。需要一个贯穿全流程的"问题-证据对齐模型"。

**方案**：

新文件 `erm.go`，定义 Evidence Requirement Model：

```go
type EvidenceRequirement struct {
    Kind     string   // "enumeration", "call_chain", "registration", 
                      // "return_value", "config_mapping", "conditional"
    Entities []string // 问题中的关键实体
    Reason   string   // 人类可读理由
    Status   string   // "unsatisfied", "partial", "satisfied"
}
```

**四层接入**：

1. **文件优先级 boost**（`BuildInitialPrompt`）：
   ```go
   scoreI := candidates[i].repoMapScore + ermFileScore(...)*200
   ```
   preScannedFiles 排序叠加 ERM 分数，使包含未满足需求实体的文件获得更高优先级。`subagent.go`（包含 `RegisterDefaultSubAgents`）因路径 "subagent" 匹配 registration 需求 + 函数名匹配 `isRegistrationLikeName`，获得高 ERM 分数。

2. **ContinuationPrompt gap 注入**：
   ```go
   if !ermAllSatisfied(e.ermRequirements) {
       suggestions := ermSuggestFiles(graph, reqs, readSet, 3)
       // 注入 "## Evidence Gaps" + 建议文件列表
   }
   ```
   每轮 ContinuationPrompt 检查需求满足状态，对未满足需求推荐具体文件并重置 idle streak。

3. **Quality gate**（`ParseOutput`）：
   ```go
   if hasEnough && !ermAllSatisfied(e.ermRequirements) {
       if unsatCount > 0 { hasEnough = false }
   }
   ```
   当存在完全未满足的需求时阻止过早结束探索。只阻止 "unsatisfied"，不阻止 "partial"，防止死循环。

4. **Dataflow 候选扩展**（`ensureStructuredEvidence`）：
   ```go
   for _, s := range ermSuggestFiles(graph, reqs, readSet, 5) {
       candidateSet[s.Path] = true
   }
   ```
   ERM 指向的文件加入 dataflow engine 的候选集。

**关键泛化设计**：
- `ermAutoSatisfyUnresolvable`：检查实体是否在 codebase 的 symbol table 中存在，不存在则自动标记为 satisfied。这是数据驱动的过滤，不是硬编码停用词列表。
- `normalizeForMatch`：统一 "sub-agent"/"sub_agent"/"subagent" 的匹配。

**为什么能解决**：ERM 从问题中提取 "需要找到哪些类型的证据"，然后在文件选择、ContinuationPrompt、quality gate、dataflow 候选四个层面引导探索。即使 LLM 自主选择不读 `subagent.go`，ERM 会通过 file boost 提升其优先级，通过 gap injection 显式建议读它，通过 quality gate 阻止在未满足注册需求时结束。

---

### 提交 4：`7fe8725` — Concrete values → 结构化证据管道

**思路**（修复断裂点 ❺）：`buildConcreteValuesSection` 确定性提取了 `RegisterDefaultSubAgents binds NewSubExplorer` 等事实，但只输出 markdown 字符串给 synthesis prompt，不进入 `StageOutput.EvidenceItems`。需要建立从确定性分析到结构化证据的数据通路。

**方案**：

修改 `buildConcreteValuesSection` 返回类型：
```go
type concreteValuesResult struct {
    markdown string              // 给 synthesis prompt
    evidence []types.EvidenceItem // 给 StageOutput
}
```

每个 concrete value 生成一个 `EvidenceConcrete` item：
```go
types.EvidenceItem{
    Kind:      types.EvidenceConcrete,
    Subject:   v.method,     // "SubExplorer.Name"
    Predicate: v.kind,       // "returns"
    Object:    v.value,      // "\"explorer\""
    Source:    v.file,        // "internal/agent/sub_explorer.go"
    LineStart: v.line,        // 32
    Confidence: 0.95,
    Producer:  "concrete_values",
}
```

每条 resolution chain 生成一个 `EvidenceDataflowPath` item。

在 SynthesisPrompt 中 merge 到 `e.structuredEvidence`：
```go
if len(cvResult.evidence) > 0 {
    e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, cvResult.evidence)
}
```

**为什么能解决**：确定性事实现在有两条独立通路——markdown 进 synthesis prompt（给 explorer LLM 看），evidence items 进 StageOutput → BusContext → finalizer prompt。即使 synthesis 失败或 LLM 不利用 synthesis 文本，确定性事实仍然通过 `## Structured Evidence` section 到达 finalizer。

---

### 提交 5：`90773be` — 解耦 evidence 生成与 markdown cap

**思路**（修复断裂点 ❻）：concrete values 的 `valueCap=25` 是为控制 synthesis markdown 大小设计的，但 evidence 生成代码在 cap 之后执行，导致 891 个 relevant 中排在 25 名之后的 short string returns（如 `SubExplorer.Name returns "explorer"`）被截断，永远不会成为 evidence item。

**方案**：

在 cap 前保存完整的 relevant 集合：
```go
// Save full set for evidence BEFORE capping
allRelevantForEvidence := relevant

// Cap only applies to markdown table
if len(relevant) > valueCap {
    relevant = relevant[:valueCap]
}
```

Evidence 从 uncapped 的 `allRelevantForEvidence` 生成：
```go
for _, v := range allRelevantForEvidence {
    cvEvidence = append(cvEvidence, types.EvidenceItem{...})
}
```

**为什么能解决**：markdown 表格仍然被 cap 到 25 条（控制 synthesis prompt 大小），但 evidence pipeline 获得完整的 891 条 relevant values。下游的 `rankEvidenceByRelevance`（按问题相关性排序）+ `formatEvidenceItems`（top-18）自带选择机制。`SubExplorer.Name returns "explorer"` 因其高 entity overlap（"subexplorer" 命中问题实体）+ 高 kind weight（EvidenceConcrete=1.0），在 ranking 中浮到前列，稳定出现在 finalizer 的 `## Structured Evidence` 中。

---

## 五、最终验证结果

修复后运行同一问题，系统回答：

```
1. AgentExplorer 能调用 SubExplorer 类型的subagent。
   Evidence: internal/agent/sub_explorer.go:32 显示了 SubExplorer.Name 方法返回 "explorer"。

2. 其他Agents 如 AgentAnalyzer, AgentPlanner 等，并没有明确的subagent绑定，
   但它们可以通过 propose_sub_agents 模块进行任务分解。
```

三跳推理链在 finalizer 的 Structured Evidence 中完整呈现：

```
- RegisterDefaultSubAgents line 58: Registers ONLY NewSubExplorer as the default SubAgent.
- RegisterDefaultSubAgents line 64 calls Register
- RegisterDefaultSubAgents line 64 calls NewSubExplorer
- SubExplorer.Name returns "explorer"  (通过 uncapped evidence pipeline)
```

### 全程对比

| 维度 | 原始 | 最终 |
|------|------|------|
| dataflow 引擎触发 | ✗（中文不匹配） | ✓ |
| synthesis 成功 | ✗（708KB 超限） | ✓（120KB 预算） |
| evidence 按相关性排序 | ✗（字典序） | ✓（entity×kind×bridge） |
| 文件选择受问题驱动 | ✗（LLM 自主） | ✓（ERM boost + gap injection） |
| 确定性事实进入 finalizer | ✗（困在 synthesis 文本中） | ✓（双通路 evidence pipeline） |
| 关键事实突破 cap | ✗（valueCap=25 截断） | ✓（evidence 从 uncapped 集合生成） |
| 最终回答 | "8 个 agent" | "AgentExplorer 调用 SubExplorer，Name()=explorer" |

### 提交清单

| 提交 | 修复的断裂点 | 改动文件 |
|------|-------------|---------|
| `e3168f9` | lowerer 模式覆盖 | lower.go, engine_test.go |
| `03c0861` | ❶❷❸ 触发+预算+排名 | evidence.go, explorer.go, builder.go, evidence_test.go |
| `4cc1f2d` | ❹ 文件覆盖精度 | erm.go, erm_test.go, explorer.go |
| `7fe8725` | ❺ 确定性事实管道 | explorer.go |
| `90773be` | ❻ evidence/markdown cap 解耦 | explorer.go |
