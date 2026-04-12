# openai 分支 vs main：系统性优化全景与分流分析

> **目的**：在投入下一轮优化之前，全面盘点 `openai` 分支相对 `main` 多出来的确定性流水线能力，明确哪些问题类型走新流程、哪些走旧流程、新旧流程能力边界在哪，以及当前分流策略是否最优、如果不是最优应该往哪个方向走。
>
> **基准**：本文对应的代码状态是 `openai` 分支 HEAD = `4a69e49`（Shape-based tie-breaking for answer chain ranking），与 `main` 的差异为 14 commits、~6700 行新增。
>
> **约定**：所有代码引用都写成 `file:line` 形式，便于直接跳转核对。

---

## 目录

1. [新增能力全景](#1-新增能力全景)
2. [触发分流矩阵](#2-触发分流矩阵)
3. [问题类型 → 流水线分流](#3-问题类型--流水线分流)
4. [新流程 vs 旧流程能力对比](#4-新流程-vs-旧流程能力对比)
5. [当前分流是否最优](#5-当前分流是否最优)
6. [分层建议](#6-分层建议)
7. [一句话总结](#7-一句话总结)

---

## 1. 新增能力全景

openai 分支在 main 基础上加了 **5 层确定性流水线**，串在 LLM 主导的 explorer 之前 / 之中 / 之后。每一层都可独立生产 `EvidenceItem`、**不依赖 LLM 成功**。

### 1.1 组件总表

| # | 组件 | 入口函数 | 位置 | 作用 | 何时触发 |
|---|---|---|---|---|---|
| 1 | **ERM** (Evidence Requirement Model) | `extractEvidenceRequirements` | `internal/agent/erm.go:27` | 从问题文本拆出 6 类需求 + 实体列表；驱动文件优先级、continuation 催读、质量门 | **无条件**（每次 `BuildInitialPrompt`） |
| 2 | **Concrete Values** | `buildConcreteValuesSection` | `internal/agent/explorer.go:1467` | 跨语言扫短函数 / 注册函数 / 中等函数，提取 returns / bindings / 配置 | **无条件**（`ParseOutput` 每次跑）|
| 3 | **Resolution Chains** | 内联于 `buildConcreteValuesSection` | `internal/agent/explorer.go:1876` | 把 concrete values 之间的 `Register→Name() returns "x"` 链起来，多跳最多 5 轮 | 有 concrete values 就跑 |
| 4 | **Type Hierarchy Chains** | 内联于 `buildConcreteValuesSection` | `internal/agent/explorer.go:~1990` | 沿 struct embedding / 类继承 传播短返回值（Go / Java / Python / JS / Rust） | 有 concrete values 且 graph 里有 embedding / inheritance 关系 |
| 5 | **Dataflow Engine** | `dataflow.Analyze` | `internal/analysis/dataflow/engine.go:22` | config → code 流、call graph、字段读写，跨语言 lowering，产出 `FlowFinding` + `EvidenceItem` | `needsDataflowAnalysis()` 关键词 OR evidence Kind 命中 |
| 6 | **Answer Chains → Ground Truth** | `identifyAnswerChains` → `builder.go` | `internal/agent/erm.go:524` + `internal/context/builder.go:211` | 从 `structuredEvidence` 里挑与问题 entity 重叠最高的 chain / registration / return，shape bonus 打破并列，top-5 作为专属 "Ground Truth" section 送到 finalizer | 有 `structuredEvidence` 就跑 |

### 1.2 支撑体系

| 功能 | 入口 | 位置 |
|---|---|---|
| Evidence 按问题排名 | `rankEvidenceByRelevance` | `internal/agent/evidence.go:374` |
| Flow Finding 按问题排名 | `rankFindingsByRelevance` | `internal/agent/evidence.go:480` |
| 问题实体抽取 | `extractRankingEntities` | `internal/agent/evidence.go:539` |
| 需求满足度检查 | `checkRequirementSatisfaction` | `internal/agent/erm.go:143` |
| ERM 驱动文件建议 | `ermSuggestFiles` / `ermFileScore` | `internal/agent/erm.go:393` |
| ERM 自动满足不可解析需求 | `ermAutoSatisfyUnresolvable` | `internal/agent/erm.go:464` |
| Ground Truth 段渲染 | 见 [1.3](#13-ground-truth-的渲染点) | `internal/context/builder.go:211` |
| Dataflow 触发器 | `needsDataflowAnalysis` | `internal/agent/evidence.go:595` |
| Answer chain 打破并列的 shape bonus | `endsWithShortLiteralReturn` / `firstSegmentIsBinds` | `internal/agent/erm.go` |

### 1.3 Ground Truth 的渲染点

`internal/context/builder.go:207-222`：

```go
// Answer chains get the highest priority — these are deterministic
// resolution chains that the system has identified as directly
// answering the user's question. The finalizer should use these
// as the answer skeleton, not re-derive from raw evidence.
if len(ac.AnswerChains) > 0 {
    var chainContent strings.Builder
    chainContent.WriteString("The following facts were extracted deterministically from source code and directly answer the question. " +
        "Use them as the primary basis for your answer — do NOT contradict or ignore them:\n\n")
    for _, chain := range ac.AnswerChains {
        chainContent.WriteString("- " + chain + "\n")
    }
    pc.UserSections = append(pc.UserSections, types.PromptSection{
        Title:   "Ground Truth (deterministic, verified from source code)",
        Content: chainContent.String(),
    })
}
```

关键点：Ground Truth section 永远排在 `Structured Evidence` 之前，并且用 "do NOT contradict or ignore them" 这种指令语言跟 finalizer 沟通。**但是它只含 `AnswerChains` 一个来源**——其他 evidence 只能进普通的 Structured Evidence 段。

---

## 2. 触发分流矩阵

### 2.1 ERM 6 个 Kind 的触发关键词与满足条件

`extractEvidenceRequirements` 在 `internal/agent/erm.go:27-137` 按关键词扫描问题文本，产出下面 6 类需求。满足度检查 `checkRequirementSatisfaction` 在 `erm.go:143-253`。

```go
// internal/agent/erm.go:47-59 (enumeration 片段)
for _, kw := range []string{"how many", "list all", "list each", "what are the"} {
    if strings.Contains(lower, kw) {
        add("enumeration", fmt.Sprintf("question asks to enumerate (%s)", kw), entities...)
        break
    }
}
for _, kw := range []string{"哪些", "多少", "列出", "哪几", "有几个", "分别"} {
    if strings.Contains(question, kw) {
        add("enumeration", fmt.Sprintf("question asks to enumerate (%s)", kw), entities...)
        break
    }
}
```

| Kind | 英文触发关键词 | 中文触发关键词 | 满足条件 |
|---|---|---|---|
| `enumeration` | `how many`、`list all`、`list each`、`what are the` | `哪些`、`多少`、`列出`、`哪几`、`有几个`、`分别` | ≥3 `[DIRECT]/[REGISTRATION]` tags 或 `EvidenceDirect/Registration` 条目 |
| `call_chain` | `call`、`invoke`、`dispatch`、`calls` | `调用`、`分发`、`触发` | 有 `[RELATIONSHIP]/[MECHANISM]` 且 entity 数 ≥2 共现 → satisfied；若仅其一 → partial |
| `registration` | `register`、`registered`、`registry`，**或 call_chain 自动蕴含** | `注册`、`绑定`，或 call_chain 自动蕴含 | `[REGISTRATION]` 行含具体 value（`new` / `"` / `only` / `default`）→ satisfied |
| `return_value` | `name`、`type`、`which`、`what` | `名称`、`类型`、`哪个`、`什么` | 存在 `EvidenceConcrete` 以 entity 为 Subject → satisfied；notes 含 entity + `return` → partial |
| `config_mapping` | `config`、`configured`、`configuration`、`yaml`、`json` | `配置` | ≥2 `EvidenceConcrete/Mechanism` 条目 |
| `conditional` | `when`、`condition`、`under what` | `条件`、`什么时候`、`何时` | ≥1 `[CONDITIONAL]` 或 `EvidenceConditional` |

### 2.2 Dataflow Engine 触发器 (`needsDataflowAnalysis`)

`internal/agent/evidence.go:595-619`：

```go
func needsDataflowAnalysis(question string, items []types.EvidenceItem) bool {
    lower := strings.ToLower(question)
    for _, needle := range []string{
        // English — dataflow / value tracing
        "flow", "flows", "path", "propagate", "through", "trigger",
        "which value", "what value", "where does", "who gets", "who is",
        "condition", "configured", "config", "registered", "route", "handler",
        "invoke", "dispatch", "call chain", "how many",
        // Chinese — equivalent dataflow / registration / invocation concepts
        "调用", "注册", "触发", "配置", "流向", "传播", "条件",
        "路由", "处理器", "绑定", "分发", "哪些", "多少", "列出",
        "哪个", "谁会", "谁能", "怎么",
    } {
        if strings.Contains(lower, needle) {
            return true
        }
    }
    for _, item := range items {
        switch item.Kind {
        case types.EvidenceConditional, types.EvidenceRelationship, types.EvidenceMechanism, types.EvidenceRegistration:
            return true
        }
    }
    return false
}
```

触发条件是三个 **OR** 之一：
- **OR 1** — 英文关键词命中：`flow / flows / path / propagate / through / trigger / which value / what value / where does / who gets / who is / condition / configured / config / registered / route / handler / invoke / dispatch / call chain / how many`
- **OR 2** — 中文关键词命中：`调用 / 注册 / 触发 / 配置 / 流向 / 传播 / 条件 / 路由 / 处理器 / 绑定 / 分发 / 哪些 / 多少 / 列出 / 哪个 / 谁会 / 谁能 / 怎么`
- **OR 3** — 已解析的 Evidence Kind 命中：`EvidenceConditional / EvidenceRelationship / EvidenceMechanism / EvidenceRegistration` 任一

**调用点**：`internal/agent/explorer.go:820`（主 explorer）、`internal/agent/sub_explorer.go:351`（sub-agent）。

### 2.3 Answer Chain 筛选（`identifyAnswerChains`）

`internal/agent/erm.go:536-543`：

```go
for _, ev := range evidence {
    // Only consider resolution chains and concrete registrations
    isChain := ev.Kind == types.EvidenceDataflowPath && ev.Predicate == "resolution_chain"
    isRegistration := ev.Kind == types.EvidenceConcrete && strings.Contains(ev.Predicate, "binds")
    isConcreteReturn := ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns"
    if !isChain && !isRegistration && !isConcreteReturn {
        continue
    }
    ...
}
```

只考虑 **三种 predicate**：
- `EvidenceDataflowPath` + predicate `resolution_chain`
- `EvidenceConcrete` + predicate 含 `binds`
- `EvidenceConcrete` + predicate == `returns`

**关键含义**：`[MECHANISM] / [CONDITIONAL] / [RELATIONSHIP]` **永远不进 Ground Truth**，即便它们才是机制类问题的答案。

打分公式（`erm.go:568-598`）：

```go
bonus := 1.0
if isChain {
    bonus = 2.0
}
if isChain && endsWithShortLiteralReturn(ev.Summary) {
    bonus *= 1.5
}
if isChain && firstSegmentIsBinds(ev.Summary) {
    bonus *= 1.3
}
candidates = append(candidates, scored{
    text:  display,
    score: float64(overlap) / float64(len(entities)) * bonus,
})
```

可达 bonus 层级：`1.0` (single item) → `2.0` (chain) → `3.0` (chain + 短字面值收尾) → `3.9` (chain + 短字面值 + binds 起头)。最终 top-5 进 Ground Truth（`maxChains = 5`，死数字）。

### 2.4 Concrete Values 扫描范围

`internal/agent/explorer.go:1486-1502` 构造 `filesToScan`：

```go
filesToScan := make(map[string]bool)
for file := range readSet {
    filesToScan[file] = true
}
for _, f := range e.allScoredFiles {
    filesToScan[f] = true
}
for _, f := range e.preScannedFiles {
    filesToScan[f] = true
}
for symName, defs := range graph.SymbolDefs {
    if len(symName) >= 6 && strings.Contains(notesJoined, symName) {
        for _, def := range defs {
            filesToScan[def.File] = true
        }
    }
}
```

四个来源的并集：**`readSet ∪ allScoredFiles ∪ preScannedFiles ∪ {notes 中提到的 symbol 所在文件}`**。

符号筛选（`explorer.go:1559-1611`）：

```go
bodyLines := sym.EndLine - sym.Line
isShort := bodyLines <= 3
isRegistrationFunc := !isShort && bodyLines <= 30 &&
    (strings.Contains(nameLower, "register") ||
     strings.Contains(nameLower, "defaults") ||
     strings.Contains(nameLower, "route") ||
     strings.Contains(nameLower, "handler") ||
     strings.Contains(nameLower, "config") ||
     strings.Contains(nameLower, "setup") ||
     strings.Contains(nameLower, "init") ||
     strings.Contains(nameLower, "bind") ||
     strings.Contains(nameLower, "subscribe") ||
     strings.Contains(nameLower, "provide") ||
     strings.Contains(nameLower, "module") ||
     strings.Contains(sym.Name, "Map"))
isMediumFunc := !isShort && !isRegistrationFunc && bodyLines <= 100
```

三档扫描策略：
- **≤3 行**：完整 `extractConcreteValues` 提取所有模式
- **≤30 行 且函数名含注册关键词**：完整提取，但只保留 `binds*` / `maps` 类值
- **4-100 行 且非注册**：行级扫描，只抓命中 `isEvidenceLine`（`return` / `register` / map 条目）的行，并取 ±1 行上下文
- **>100 行**：**完全跳过**

### 2.5 Concrete Values 过滤（P1 修复后）

`internal/agent/explorer.go:1718-1779`（dd7151e 后）：

```go
// Filter to keep only values relevant to the investigation:
// 1. Registrations — always kept (rule A)
// 2. Short string returns from pre-scanned/read/scored files — always kept (rule B1)
// 3. Short string returns from other files — only if receiver is in notes (rule B2)
// 4. Values referencing symbols from the investigation notes (rule C)
var relevant []concreteValue
var cntA, cntB1Read, cntB1PreScan, cntB1Scored, cntB2, cntC, cntLongSkip int
for _, v := range allValues {
    if strings.Contains(v.kind, "binds") || v.kind == "maps" || v.kind == "config" || v.kind == "decorates" || v.kind == "assigns" {
        relevant = append(relevant, v)
        cntA++
        continue
    }
    if v.kind == "returns" {
        isStringLit := len(v.value) >= 2 && (v.value[0] == '"' || v.value[0] == '\'')
        isBoolOrNil := v.value == "true" || v.value == "false" || v.value == "nil" || v.value == "null"
        if isStringLit && len(v.value) > 80 {
            cntLongSkip++
            continue
        }
        if isStringLit || isBoolOrNil {
            if readSet[v.file] { relevant = append(relevant, v); cntB1Read++; continue }
            if preScannedSet[v.file] { relevant = append(relevant, v); cntB1PreScan++; continue }
            if allScoredSet[v.file] { relevant = append(relevant, v); cntB1Scored++; continue }
            if strings.Contains(notesJoined, v.receiver) ||
               strings.Contains(notesJoined, v.method) {
                relevant = append(relevant, v); cntB2++; continue
            }
        }
    }
    for _, word := range strings.Fields(v.value) {
        cleaned := strings.Trim(word, "(){}[]&*,;")
        if len(cleaned) >= 6 && strings.Contains(notesJoined, cleaned) {
            relevant = append(relevant, v); cntC++; break
        }
    }
}
```

**P1 新增的 `allScoredSet` 路径（`cntB1Scored`）**是本次稳定性核心：让所有 keyword-search 命中的文件都享有无条件保留短返回值的特权，消除了对 LLM notes 的依赖。

### 2.6 Dataflow 候选文件选择

`internal/analysis/dataflow/engine.go:22-60` 的 `Analyze` 入口：

```go
func Analyze(graph *repomap.Graph, opts Options) Result {
    opts = defaultOptions(opts)
    if graph == nil {
        return Result{}
    }

    candidateFiles, truncated := selectCandidateFiles(graph, opts)
    if len(candidateFiles) == 0 {
        return Result{}
    }

    lowerers := newLowererRegistry()
    lowered := make([]LoweredFile, 0, len(candidateFiles))
    var evidence []types.EvidenceItem
    for _, filePath := range candidateFiles {
        fi := graph.FileIndex[filePath]
        if fi == nil {
            continue
        }
        if fi.Language == "" && !isConfigLikeFile(fi.RelPath) {
            continue
        }
        if isConfigLikeFile(fi.RelPath) {
            cfgEvidence := lowerConfigFile(opts.RepoRoot, fi.RelPath)
            evidence = append(evidence, cfgEvidence...)
            continue
        }
        ...
    }
```

候选文件来自 `selectCandidateFiles` 的图结构扩展（import/imported-by + 符号关系），**没有 ERM entity 过滤**。

---

## 3. 问题类型 → 流水线分流

下面 11 个典型问题类型，展示每个类型打开/关闭了哪些新能力，以及是否最终有 Ground Truth 送达。

| # | 问题类型 | 示例 | ERM Kind | needsDataflow? | Concrete Values | Chains | Answer Chains | Ground Truth | 裁决 |
|---|---|---|---|:-:|:-:|:-:|:-:|:-:|---|
| 1 | **Enumeration** | "how many X / 有多少 X" | `enumeration` (+ `return_value`) | ✅（`how many`/`多少`）| ✅ | ✅ | ✅ | ✅ | **新流程主场**，但 dataflow 其实多余 |
| 2 | **Registration** | "which X does Y / Y 注册了哪个 X" | `registration` + `call_chain` | ✅ | ✅ | ✅ 注册链 | ✅ | ✅ | **新流程主场** |
| 3 | **Identity** | "what is X's name/type / X 叫什么" | `return_value` | ⚠️（只有 `type`/`what` 部分命中）| ✅ | ✅ 单跳 | ✅ shape bonus 命中 | ✅ | **新流程主场**（Bug C 后稳定）|
| 4 | **Type relationship** | "Is X a Y / X 是 Y 吗" (interface/继承) | `return_value` + 可能 `conditional` | ⚠️ | ⚠️（只 bool/string 返回）| ⚠️ hierarchy chains 可能覆盖 | ❌（inheritance predicate 不在白名单）| ❌ | **部分新流程**：继承链构建但不进 GT |
| 5 | **Config mapping** | "config key X 控制什么" | `config_mapping` + `conditional` | ✅ | ⚠️（YAML/JSON 叶子进 concrete）| ❌（配置事实不构链）| ❌（无 chain predicate）| ❌ | **dataflow 独跑**，无 GT，finalizer 自己组合 |
| 6 | **Mechanism** | "how does X work / X 怎么工作" | **无**（无 `mechanism` Kind）| ✅（`怎么`）| ❌（不扫方法体）| ❌ | ❌ | ❌ | **完全走旧流程 + dataflow 枝节**，新能力零贡献 |
| 7 | **Dataflow** | "where does X flow / 流向哪里" | `return_value` + 可能 `conditional` | ✅ | ⚠️ | ❌ | ❌ | ❌ | **dataflow 独跑**，evidence 进但不进 GT |
| 8 | **Conditional** | "what condition triggers X / 什么条件触发 X" | `conditional` | ✅ | ⚠️（短 guard 可能）| ❌ | ❌ | ❌ | **dataflow + conditional evidence**，无 GT |
| 9 | **Call graph** | "who calls X / 谁调用 X" | `call_chain` + `enumeration` | ✅ | ⚠️（绑定点）| ⚠️ 如果 X 是 Name | ⚠️ | ⚠️ | **dataflow 主跑**，有时有 chains |
| 10 | **Comparison** | "X 和 Y 的区别" | `return_value` + `enumeration` | ❌（无关键词）| ✅（两边方法都扫）| ⚠️ | ❌（Relationship 不在白名单）| ❌ | **concrete values 有料，Ground Truth 无** |
| 11 | **Location** | "X 定义在哪" | **无** | ❌ | ❌ | ❌ | ❌ | ❌ | **完全走旧流程**（纯 grep/LLM）|

**图例**：✅ 基本保证触发；⚠️ 条件性触发 / 能力残缺；❌ 不触发 / 落空。

**观察**：11 种问题里，**只有 1/2/3（enumeration / registration / identity）能完整走完新流程并得到 Ground Truth**。其他类型要么新能力触发不全，要么触发了但 evidence 被 Answer Chain predicate 白名单挡在 GT 外。

---

## 4. 新流程 vs 旧流程能力对比

### 4.1 新流程（deterministic pipeline）能做的

- **事实同源稳定**：短方法返回值、registration binding、config 叶子、struct embedding → 跨 run **字节级稳定**（P1 + Bug B + Bug C 修完后 5/5）
- **多跳解析**：`Register → NewFoo → Foo.Name → "x"` 这类 **命名 / 身份类** 3 跳最稳定（已验证）
- **跨语言**：Go / Java / Python / JS / TS / Rust（`extractConcreteValues` 多套模式，见 explorer.go 同文件内的 pattern 列表）
- **配置文件扫描**：YAML / JSON / TOML 叶子扁平化为点路径（`extractConfigValues`）
- **不依赖 LLM 理解**：只要 tree-sitter 能 parse，就能产出事实
- **确定性排序**：ERM + shape bonus 把答题链排到前
- **观测性**：一套完整的永久 debug 日志（filter 计数、per-file 统计、chain 样本、answer_chain 全文——见 dd7151e commit）

### 4.2 新流程**不**能做的（落到 LLM 身上）

- **方法体内部逻辑**：短方法 ≤3 行只抓 return；中等函数只抓 return/bind/map 行；**控制流、条件分支、循环逻辑不进 concrete values**
- **语义归纳**："X 和 Y 是什么关系"、"X 为什么这么设计"
- **Mechanism 问题**："X 怎么工作" 没 ERM Kind，没 chain shape，finalizer 要重新读文件理解
- **关系类链**：interface satisfaction、embedding 链已在 `hierarchyChains` 生成，但 **`identifyAnswerChains` 不收 relationship predicate**，所以不进 Ground Truth
- **跨问题比较**：两个类型并列对比没有专门结构
- **定位类**："X 定义在哪" 压根不触发

### 4.3 旧流程（main 已有）做的

- LLM 自由探索（`grep` / `read_file` / `list_files`）
- Phase 0 breadth scan → Phase 1 depth read 两阶段
- 投资 LLM 能力去理解语义、写叙述、总结
- **所有问题兜底都走这条**（新能力不会关掉它，只是在它之上叠加 deterministic evidence）

### 4.4 对比总表

| 能力 | 旧流程 | 新流程 | 说明 |
|---|:-:|:-:|---|
| 语义理解 | ✅ | ❌ | 新流程只做 pattern matching |
| 跨文件 multi-hop 事实链 | ❌（靠 LLM 理解）| ✅（deterministic） | 新流程核心战果 |
| 短返回值 / 身份识别 | ⚠️（靠 LLM 读）| ✅ | 新流程字节级稳定 |
| 方法体内部控制流 | ✅ | ❌ | LLM 擅长，pattern 不擅长 |
| Config → code 流 | ⚠️ | ✅（dataflow engine） | 新流程 lowering 扫 |
| Interface/继承关系 | ⚠️ | ⚠️（建了但挡在 GT 外） | Bug D 级别问题 |
| 机制类叙述 | ✅ | ❌ | LLM 必须做 |
| 定位查询 | ✅（grep）| ❌ | 新流程无入口 |
| 结果稳定性 | ❌（LLM 采样）| ✅（GT section）| 新流程核心价值 |
| 触发成本 | 零 | 每次跑 ERM + concrete values + 视情况 dataflow | **现在对所有问题都付** |

---

## 5. 当前分流是否最优

**整体判断**：**对"注册 / 身份 / 枚举"类问题 —— 接近最优；对"机制 / 比较 / 定位"类问题 —— 明显失衡。**

### 5.1 `needsDataflowAnalysis` 关键词过宽

**位置**：`internal/agent/evidence.go:597-607`

**问题**：触发词太泛：`how many` / `多少` / `怎么` / `config` / `registered` 都进。但：

- `"有多少个 agent"` 只要 ERM enumeration + concrete values 就够，`dataflow.Analyze` 跑了**纯属浪费**
- `"怎么"` 触发 dataflow，但 dataflow 引擎对机制类问题几乎**零贡献**（它 lower 不到行为语义）

**成本**：
- 每次多花 `O(files × avg_func_size)` 的 lowering
- 候选文件选择是 graph-structural 的（`dataflow/engine.go:89-143`），**没做 entity 过滤**，很容易把整个仓库的 handler 类文件拉进来

**修法**：two-phase router —— 先跑便宜的 ERM + concrete values，如果 ERM 已 satisfied 就 skip dataflow；或者 `needsDataflow` 区分"single-hop 查值" vs "multi-hop 传播"（已在 [memory 记录](../../.claude/projects/-home-chatpp-codrax/memory/project_dataflow_trigger_issues.md)）。

### 5.2 `identifyAnswerChains` predicate 白名单太窄

**位置**：`internal/agent/erm.go:538-541`

**问题**：只接 `resolution_chain / binds* / returns`。

**后果**：
- interface / inheritance 链（`hierarchyChains` 已生成）**永远不进 Ground Truth**
- `[MECHANISM]` / `[CONDITIONAL]` / `[RELATIONSHIP]` 这三类 evidence **永远不进 GT**
- 问题类型 4/5/8/10 明明有可用事实，却被挡在 GT 外

**修法**：按问题 Kind 动态开放白名单——
- `"Is X a Y?"` 开 `inheritance` / `relationship`
- `"Config 控制"` 开 `config_value` / `mechanism`
- `"What condition"` 开 `conditional`

### 5.3 ERM 没有 `mechanism` Kind

**位置**：`internal/agent/erm.go:47-137`

**问题**：`"How does X work / X 怎么工作"` 没任何 ERM Kind 被触发。

**后果**：
- 没有质量门
- 没有 continuation 催读
- 没有 Ground Truth
- 这类问题**完全走旧流程**，不享受 openai 分支任何新能力

**修法**：加 `mechanism` Kind，触发时让 ERM 要求"读 X 的 `Execute/Run/Do` 类方法的**完整**函数体"（不是只扫短返回）。触发词：`how does`、`怎么`、`什么机制`、`如何工作`。

### 5.4 Concrete Values 扫描没有 entity 亲和过滤

**位置**：`internal/agent/explorer.go:1554-1686`

**问题**：遍历 `filesToScan × 所有符号`，**不看 ERM entity**；ERM 只驱动文件优先级，不驱动**函数 / 符号**优先级。

**后果**：
- 一个 `filesToScan` 进来就扫全文件里的所有短方法，哪怕它们跟问题无关
- 这些值进 `allValues` 后靠 filter 剪，filter 又主要靠 file-set（P1 已修）+ notes（剩余依赖）
- 当 allValues 很多时（1500-1800），过滤后的 relevant 仍可能包含 800-1000 项，chain 构造 O(N²) 成本高

**修法**：对 `allScoredFiles` 之外的 cross-ref 加入文件，限定扫描到与 ERM entity 有名字相关的符号。

### 5.5 Ground Truth 只给 top-5 链，`maxChains` 死数字

**位置**：`internal/agent/erm.go:775`（`identifyAnswerChains` 调用时写死 `5`）

**问题**：对机制 / 比较类问题，5 条太少；对身份类问题，5 条已够。

**修法**：按问题 Kind 动态 `maxChains`——enumeration 给 10-15，identity 给 3-5，mechanism 给 8-12。

### 5.6 LLM notes 与 concrete values 双重提取

**问题**：LLM 的 `[DIRECT]` tag 和 concrete values 提取同一事实，靠 `StableEvidenceID` 去重。LLM 花了一次 token 做机器能更稳地做的事。

**修法**：ERM 告诉 LLM "这类事实我会自己搞定，你专注关系和机制"——Prompt 里明确告知哪些 kind 由 deterministic 流水线保证，让 LLM 专注它擅长的部分。

---

## 6. 分层建议

基于上面的不优之处，openai 分支下一步的**架构级**方向应是：

### A. 从"所有流水线都跑"退到"按问题 Kind 精准分流"

ERM 输出 Kinds → 映射到该激活哪些下游组件：

| 问题 Kind | 激活组件 | 不激活组件 |
|---|---|---|
| `enumeration` / `registration` | concrete values + chains + answer chains | dataflow engine（除非明确多跳） |
| `return_value` (identity) | concrete values | chain multi-pass、dataflow |
| `config_mapping` | dataflow engine (config lowering) + concrete values (YAML/JSON) | chain 构造 |
| `call_chain` (multi-hop) | dataflow engine + chains | — |
| `conditional` | dataflow engine + conditional evidence | chains |
| **`mechanism`** (新) | 新的 mechanism pipeline（见 C） | dataflow engine |

### B. 打通 Ground Truth 对非 chain evidence 的支持

按 ERM Kind 为 `identifyAnswerChains` 切换 predicate 白名单：

```go
// 伪代码
func (e *explorerEvaluator) answerChainPredicates() map[types.EvidenceKind]bool {
    pred := map[types.EvidenceKind]bool{
        types.EvidenceDataflowPath: true,  // resolution_chain
        types.EvidenceConcrete:     true,  // binds / returns
    }
    for _, req := range e.ermRequirements {
        switch req.Kind {
        case "return_value":
            pred[types.EvidenceRelationship] = true  // interface satisfaction
        case "config_mapping":
            pred[types.EvidenceMechanism] = true
        case "conditional":
            pred[types.EvidenceConditional] = true
        case "mechanism":
            pred[types.EvidenceMechanism] = true
            pred[types.EvidenceRelationship] = true
        }
    }
    return pred
}
```

Relationship / Conditional / Mechanism evidence 也能进 GT。

### C. mechanism Kind 的专属流水线

机制问题**不做多跳解析**，改为"聚焦函数体扫描"：

1. ERM 识别 mechanism 关键词（`how does`、`怎么`、`什么机制`）
2. 根据 ERM entity 定位核心符号（如 `X.Execute`、`X.Run`、`X.Do`）
3. 读取这些方法的**完整源码**（不是只扫短返回）
4. 跑分支计数、调用点提取、返回路径分析
5. 生成结构化 `EvidenceMechanism` 条目
6. 按 B 的新白名单流入 Ground Truth

### D. 跟 LLM 职责划分写入 prompt

在 Phase 2 evidence collection prompt 里明确告知：

> **Note**: Short method returns (`Name()`, `Type()`, `Kind()`), registration bindings, and config file values are extracted automatically by the deterministic pipeline. You do NOT need to record these as `[REGISTRATION]` / `[DIRECT]` tags — focus your extraction on `[RELATIONSHIP]` / `[MECHANISM]` / `[CONDITIONAL]` evidence which requires your semantic understanding.

减轻 LLM 负担，让它专注讲故事，减少 token 浪费和双重提取。

---

## 7. 一句话总结

> **openai 分支把"查值 / 找注册 / 证身份"这三类问题从"LLM 靠运气"升到"确定性流水线"——这是主要战果。但它把成本均摊到所有问题（dataflow 过度触发），没给机制 / 比较 / 定位类问题开专用通道（fallback 回纯 LLM），而且 Ground Truth 的 predicate 白名单过窄（把已经生成的 relationship / inheritance / mechanism evidence 挡在外面）。**
>
> **下一步的最高杠杆是"按 ERM Kind 精细化分流 + 放宽 Ground Truth 白名单"，而不是继续给 concrete values 加新模式或给 `needsDataflowAnalysis` 加新关键词。**

---

## 附录：相关 commit 与 memory 索引

### openai 分支关键 commit（c18e52d → 4a69e49）

| Commit | 主题 |
|---|---|
| `c18e52d` | 跨语言证据链与数据流分析详细方案（设计文档）|
| `e3168f9` | Java `getString` 配置键 regex 修复 + 字段读写 evidence |
| `03c0861` | Question-aware evidence ranking + 中文 dataflow 触发 + synthesis 预算控制 |
| `4cc1f2d` | Evidence Requirement Model (ERM) |
| `7fe8725` | Concrete values + resolution chains 作为结构化 evidence |
| `90773be` | 解耦 evidence 生成与 synthesis markdown cap（breakpoint 6）|
| `fde9586` | Dataflow evidence pipeline 修复总文档 |
| `2417047` | Resolution chain 构建使用 uncapped concrete values（breakpoint 7 半修）|
| `59e6b71` | 分析文档：breakpoint 7 |
| `a69dfa6` | Answer chain identification layer（breakpoint 8）|
| `e414ca4` | 分析文档：breakpoint 8 |
| `0f62b2b` | 稳定性下一步提案（P1-P4）|
| `dd7151e` | **P1 (allScoredFiles filter) + Bug B (chain evidence uncapped) + 永久诊断日志** |
| `4a69e49` | **Bug C (answer chain shape bonus)** |

### Memory 交叉引用

- `project_stability_next_steps.md` —— P1 / Bug B / Bug C 状态
- `project_dataflow_evidence_pipeline.md` —— 8 个 breakpoint 的分析
- `project_dataflow_trigger_issues.md` —— `needsDataflowAnalysis` 关键词过宽（早已记录）
- `project_explorer_deep_dive.md` —— explorer 18 commits 的演进
- `project_explorer_precision_gaps.md` —— 3 轮精度评估（pre-openai 分析）
- `feedback_cap_vs_evidence_separation.md` —— markdown cap 与 evidence cap 必须分离（Bug B 的根本原则）
- `feedback_first_principles_root_cause.md` —— 不接受 LLM 随机性作为根因
