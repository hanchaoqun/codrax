# S1 / S2 / S3 三层修复全记录:从 1/5 到 10/10

> 本文档记录 2026-04-12 下半天一次完整的"观察失败 → 局部修复 → 发现回归 → 全链路 trace → 三层根因 → 协调修复"的调试过程。
> 主线是 df1 eval 从 1/5 PASS 一路爬升到 5/5 PASS、df3 保持 5/5 PASS 的 8 次 commit 演进。
>
> 关键转折点是第 4 轮失败后,用户下达了"遇到需要 debug 时,务必检查每一个链路上关键数据流后再给根因"的硬规则。在此之前的 4 次迭代全部是局部修复;此后一次端到端 trace 定位了 3 个彼此独立的失败层。
>
> 技术细节、根因推导、修复设计、代码片段、过拟合审计全部包含。

---

## 0. 背景与目标系统

本次修复的代码库是 codrax:一个 5 层多 agent 架构的代码问答系统,用 Go 实现。

相关层次:

```
Orchestrator (L1)
    ├─ Analyzer agent     — 分类问题、产出 TaskList
    ├─ Explorer agent     — 搜索 / 读文件 / 抽取结构化证据
    └─ Finalizer agent    — 把 Ground Truth 翻译成用户可读答案
```

本文档讨论的"数据流链路"指的是:**用户请求 → Analyzer → Explorer → ERM/证据抽取 → identifyAnswerChains → extractAnswerSymbols → BusContext → context builder → Finalizer prompt → LLM → ParseOutput → recordTaskFinalize → Renderer → 用户最终输出**。

每一环都有可能失败。本次 debug 的核心教训是:**三环同时出问题,只修一环不够**。

## 1. 两个 eval 测试用例

评估基准:

| case | 问题 | EXPECT_CONTAINS | EXPECT_NOT_CONTAINS |
|---|---|---|---|
| df1 | "有多少个 agent 可以调用 subagent?" | `Explorer`, `SubExplorer` | `Orchestrator`, `BaseAgent`, `SubAgentRuntime` |
| df3 | "explorerEvaluator 的 ContinuationPrompt 是怎么实现的?有哪几种 push 策略?" | `ContinuationPrompt`, `explorer.go` | (无) |

df1 是典型的 registration 问题:"哪些 agent 注册/绑定了 subagent"。df3 是 mechanism 问题:"某个方法的实现步骤"。

判题器:对最终 answer 文本做子串匹配。`missing:X` = EXPECT_CONTAINS 的子串没出现;`banned:X` = EXPECT_NOT_CONTAINS 的子串出现了。

## 2. 早上的里程碑(起点)

早上 commit `d28fc80` 的里程碑记录:

- **确定性管线已达设计天花板**:df1 和 df3 的 `t11_gate_skip = 5/5`,答案链(`answer_chain[0]`)在 5 次 run 中稳定是正确的规范链 `RegisterDefaultSubAgents → SubExplorer`
- **但最终答案正确率仍然 1-2/5**:finalizer LLM(gpt-4o)无视 Ground Truth,幻觉出 `Orchestrator/BaseAgent/SubAgentRuntime`
- 结论:"**模型层问题,不可在确定性层修复**"

这个结论是本次调试要推翻的前提。事实证明用应用层结构化手段是可以修复的,不需要换模型。

## 3. 第一性原则分析(调试的出发点)

在进入调试之前,做了一次第一性原则分析,得出两个架构瓶颈:

### 瓶颈 1:`identifyAnswerChains` 的排名缺乏"终端验证"

排名打分是纯生成式的(score = shape_bonus × entity_overlap),没有任何 "这条链终端真的是一个 agent 类型吗" 的判别式检查。df1 run 3 的反例就是这样被排到 top-0 的:

```
chain[0..8]: `RegisterDefaults()` binds ... → `Registry.List()` assigns name := range r.tools {
```

这些链 top-9 全是 `Registry.List()` 遍历,终端是 `range r.tools/r.agents/r.skills` — 根本不是 sub-agent 的调用链。

### 瓶颈 2:Finalizer 把"翻译"和"表达"混在一次 LLM 调用

Finalizer 的职责理论上可以分成两步:
1. **翻译**(deterministic): chain → symbol list,把答案链的终端符号抽出来
2. **表达**(generative):symbol list → prose,把符号列表组织成自然语言

但当前实现是一次 LLM 调用同时做这两件事。Ground Truth 作为 prompt 文本传给 LLM,LLM 可以:
- 忽视 Ground Truth(df1 run 4 的 `Orchestrator/BaseAgent` 幻觉)
- 从 EvidenceItems 段里抓别的符号(gpt-4o 记忆力够强,"Context is large" 正是被训练出来的习惯)

两者叠加导致 1-2/5 的最终正确率。

### 对策:L0-1 + L0-2

- **L0-1**(终端验证):为 `identifyAnswerChains` 的 kind 之 `registration`/`call_chain` 增加一个 discriminative 终端检查
- **L0-2**(extract-then-express):新增一个确定性 `extractAnswerSymbols` 步骤,把 Ground Truth 翻译结果作为结构化字段 `AnswerSymbols []AnswerSymbol` 传递给 Finalizer,Finalizer 的 prompt 收到一个"必须且只能列这些符号"的硬列表

L0-1/L0-2 的详细设计保存在 `project_L0_1_terminal_verification_design.md` 和 `project_L0_2_extract_then_express_design.md`。

## 4. 第一轮 L0 实施(commit `8d9c35f` / `a72015f`)

### 4.1 L0-1:终端谓词(`8d9c35f`)

新增终端谓词系统:

```go
// internal/agent/erm.go
type terminalPredicate func(chainText string, graph *repomap.Graph) bool

var terminalPredicateByKind = map[string]terminalPredicate{
    "registration": terminalIsConcreteSymbolRef,
    "call_chain":   terminalIsConcreteSymbolRef,
    "return_value": terminalIsConcreteLiteral,
}

func terminalIsConcreteSymbolRef(chainText string, graph *repomap.Graph) bool {
    terminal := extractTerminalSegment(chainText)
    badPatterns := []string{
        "range ",          // loop header
        "for _, ",
        "make(", "append(", "len(", "cap(",
        "assigns name :=",
    }
    for _, bad := range badPatterns {
        if strings.Contains(terminal, bad) {
            return false
        }
    }
    if hasMethodCallShape(terminal) { return true }
    if hasReturnsLiteralShape(terminal) { return true }
    if graph != nil && containsGraphSymbol(terminal, graph) { return true }
    return false
}
```

并在 `identifyAnswerChains` 中对候选链应用谓词,失败的乘 ×0.2 (demote-not-drop,保留 fallback 安全网):

```go
if len(terminalPreds) > 0 {
    terminalOK := true
    for _, p := range terminalPreds {
        if !p(ev.Summary, graph) { terminalOK = false; break }
    }
    if !terminalOK { bonus *= 0.2 }
}
```

### 4.2 L0-2:extract-then-express(`a72015f`)

新增数据类型 `AnswerSymbol`,贯穿 BusContext → StageOutput → AgentContext,然后在 `identifyAnswerChains` 之后运行 `extractAnswerSymbols`:

```go
type AnswerSymbol struct {
    Name      string `json:"name"`
    File      string `json:"file,omitempty"`
    Line      int    `json:"line,omitempty"`
    Chain     string `json:"chain"`
    Kind      string `json:"kind"`
    Rationale string `json:"rationale,omitempty"`
}
```

Finalizer prompt 增加 "Translation mode":

```
## Translation mode: symbols are already chosen

The deterministic pipeline has produced a structured Answer Symbols
list (see the 'Extracted Answer Symbols' section below). Your task
is to write a brief prose answer that mentions EXACTLY the symbols
in that list — no more, no less.
```

### 4.3 第一次 eval(`a72015f`):1/5

**这一层是全部故事的起点**。跑 df1 × 5:

| run | verdict |
|:-:|---|
| 1 | FAIL `missing:Explorer missing:SubExplorer` |
| 2 | FAIL `banned:BaseAgent` |
| 3 | FAIL `missing:SubExplorer` |
| 4 | PASS |
| 5 | FAIL `banned:BaseAgent` |

**1/5**。比基线还差(基线 1-2/5)。为什么?

## 5. 第二轮:构造器原点漏洞

### 5.1 症状

跟踪 run 1 的日志。L0-1 IS 工作:

```
[erm] L0-1 terminal predicate demoted chain: `NewBaseAgent()` ...
[erm] L0-1 terminal predicate demoted chain: `NewProposeSubAgents()` ...
...
```

很多 `range r.tools` 链被 demote。但还是有构造器链保留在 top-N 里:

```
[explorer]   answer_chain[3]: `NewBaseAgent()` returns &BaseAgent{ → `BaseAgent.buildToolSchemas()` assigns ok := t.(*tool.ProposeSubAgents); ok {
```

这个链的终端是 `assigns ok := t.(*tool.ProposeSubAgents); ok {` — 包含 `ProposeSubAgents`,通过 `hasMethodCallShape`,谓词接受。于是这条链进入了 AnswerSymbols,finalizer 听话地列出 `BaseAgent`。

### 5.2 根因

L0-1 只检查终端,不检查原点。构造器起手的链(`NewFoo() returns &Foo{}`)在语义上根本不是 registration chain 而是 constructor chain,但终端检查对此一无所知。

### 5.3 修复 L0-1 v2:原点谓词(`f34c0ac`)

为 `registration` kind 增加原点谓词:

```go
var originPredicateByKind = map[string]terminalPredicate{
    "registration": chainOriginIsRegistrationLinkage,
}

func chainOriginIsRegistrationLinkage(chainText string, _ *repomap.Graph) bool {
    first := chainText
    if idx := strings.Index(chainText, "→"); idx >= 0 {
        first = chainText[:idx]
    }
    if strings.Contains(first, " binds ") { return true }
    if strings.Contains(first, "Register") { return true }
    return false
}
```

Demote 因子用更狠的 ×0.1:构造器起手**绝对不可能**是 registration chain,应该被压得很死。

### 5.4 第二次 eval(`f34c0ac`):2/5

有改善但仍然 `banned:BaseAgent` 反复出现。原因:

```
run 3 demote log:
`NewBaseAgent()` binds ONLY name types.AgentName, deps *Dependencies, eval Evaluator
                 ^^^^^^^^^^^ 
                  <-- 这里 !!!
```

**concrete-values 抽取器对每个函数都生成 `NewFoo() binds ONLY <参数列表>` 格式**,"binds ONLY" 是**通用签名格式**,不是 registration 标记。我的原点谓词 `strings.Contains(first, " binds ")` 被误触发了。

### 5.5 修复 L0-1 v3:复合结构检查(`497fb12`)

保留两条接受路径,但第二条从"包含 binds"变成"binds ONLY 后跟一个大写起手 + `(` 的调用表达式":

```go
func chainOriginIsRegistrationLinkage(chainText string, _ *repomap.Graph) bool {
    first := chainText
    if idx := strings.Index(chainText, "→"); idx >= 0 {
        first = chainText[:idx]
    }
    // 路径 1:函数名含 Register
    if strings.Contains(first, "Register") { return true }
    // 路径 2:`binds ONLY` 后跟调用表达式(构造器 / Register 函数调用)
    bindsIdx := strings.Index(first, "binds ONLY ")
    if bindsIdx < 0 { return false }
    rest := first[bindsIdx+len("binds ONLY "):]
    return firstTokenIsCallExpression(rest)
}

func firstTokenIsCallExpression(seg string) bool {
    seg = strings.TrimLeft(seg, " \t")
    if seg == "" || seg[0] < 'A' || seg[0] > 'Z' { return false }
    i := 0
    for i < len(seg) && isIdentChar(seg[i]) { i++ }
    return i < len(seg) && seg[i] == '('
}
```

**关键结构判断**:
- `RegisterX() binds ONLY NewFoo(deps)` → 后跟 `NewFoo(` 是调用 → ✅ 接受
- `NewBaseAgent() binds ONLY name types.AgentName` → 后跟 `name` 小写 → ❌ 拒绝

同时把 L0-2 改为 **strict mode**(之前的 loose mode 会从 demoted 链抽符号):

```go
// internal/agent/erm.go (497fb12 版本)
func extractAnswerSymbols(chains []string, questionKind string, reqs []EvidenceRequirement, graph *repomap.Graph) []types.AnswerSymbol {
    // ...
    terminalPreds := terminalPredicatesFor(reqs)
    originPreds := originPredicatesFor(reqs)

    for _, chain := range chains {
        if !strings.Contains(chain, "→") { continue } // 无 arrow 跳过
        ok := true
        for _, p := range terminalPreds { if !p(chain, graph) { ok = false; break } }
        if ok { for _, p := range originPreds { if !p(chain, graph) { ok = false; break } } }
        if !ok { continue }
        // ... extract from string ...
    }
}
```

### 5.6 第三次 eval(`497fb12`):2/5

仍然 2/5。两个新失败模式:`missing:Explorer missing:SubExplorer`(run 1、3、4)和 `banned:BaseAgent`(run 3)。

### 5.7 第三次迭代:EvidenceItem 重构(`b595033`)

观察发现当前 L0-2 从**字符串**抽取符号有两个固有缺陷:

1. **无 arrow 单跳链无法处理**:`RegisterDefaultSubAgents() binds ONLY NewSubExplorer(deps)` 没有 `→`,`extractSymbolFromChain` 把整条字符串塞给 `firstUppercaseIdent`,取到的是 **subject**(`RegisterDefaultSubAgents`)而非 **object**(`NewSubExplorer`)。
2. **尾部 `(file:line)` 解析脆弱**:正则匹配 source locator 逻辑复杂,结构字段 `Source/LineStart` 直接给,为什么不用?

修复:

```go
// identifyAnswerChains 返回值从单一 []string 改为 ([]string, []EvidenceItem)
func identifyAnswerChains(...) ([]string, []types.EvidenceItem) {
    // ...
    for _, c := range candidates {
        if !seenText[c.text] {
            chains = append(chains, c.text)
        }
        if c.strictOK {
            strictItems = append(strictItems, c.src)
        }
    }
    return chains, strictItems
}

// extractAnswerSymbols 消费 []EvidenceItem 而非 []string
func answerSymbolFromEvidence(ev EvidenceItem, questionKind string, graph *repomap.Graph) AnswerSymbol {
    sym := AnswerSymbol{
        File: ev.Source, Line: ev.LineStart,
        Kind: questionKind, Chain: ev.Summary,
    }
    switch {
    case isRegistrationShape(ev):
        // Concrete + binds:Object 是 "NewSubExplorer(deps)",
        // stripNewPrefix(firstUppercaseIdent) → "SubExplorer"
        sym.Name = stripNewPrefix(firstUppercaseIdent(ev.Object))
    case ev.Kind == EvidenceConcrete && ev.Predicate == "returns":
        // Subject = "SubExplorer.Name",取 dot 前的 receiver
        sub := ev.Subject
        if dot := strings.Index(sub, "."); dot > 0 { sub = sub[:dot] }
        sym.Name = firstUppercaseIdent(sub)
    case ev.Kind == EvidenceDataflowPath && ev.Predicate == "resolution_chain":
        // 多跳链需要字符串解析,因为终端信息比 Subject/Object 更丰富
        terminal := extractTerminalSegment(ev.Summary)
        sym.Name = firstUppercaseIdent(terminal)
    // ...
    }
    return sym
}
```

**关键收益**:无 arrow 的 concrete binds 也能正确产出 AnswerSymbol,因为 `Object = "NewSubExplorer(deps)"` 直接给定。

### 5.8 第四次 eval(`b595033`):df1 3/5,df3 4/5

df1 爬到 3/5。**但 df3 首次回归到 4/5**。df3 run 3 的 verdict 是 `missing:ContinuationPrompt missing:explorer.go`,而且 run-3.out 显示 "(no result)" — 连字都没有。

## 6. 关键转折点:用户下达新的调试规则

连续四轮迭代,每一轮都是"看到一层问题 → 修那一层 → 再跑一遍 → 发现新问题",到第四轮 df3 开始回归时,**用户介入**:

> 遇到需要 debug 时,务必要认真思考每一个链路上关键数据流,包含但不限输入输出等详细数据,都要检查清楚后再给根因结论和系统优化方案,避免局部优化!记住!

这条规则被保存为永久反馈 `feedback_trace_full_dataflow_before_fixing.md`,要求:

1. **列出完整数据流路径**,从用户请求到最终输出
2. **对每一步检查实际中间数据结构**,不是只看表层
3. **找出所有可能致因**,不是第一个看到的
4. **检查看起来正常的层**
5. **只有在整条链理清后再提方案**,攻击根因

这是第四次迭代到第五次迭代的**决定性转变**。后面不再是"看到就修",而是 trace 完再说。

## 7. 端到端 trace(决定性动作)

对三个失败样本做全链路 trace。

### 7.1 df1 run 2(`missing:SubExplorer`)

| 环节 | 实际数据 |
|---|---|
| analyzer 输出 | `question_kind=enumeration`, `entities=["subagent"]`, `answer_shape=list_of_symbols` |
| extractAnswerSymbols | **返回 nil** — `enumeration` 不在 `terminalKinds` |
| ctx.AnswerSymbols | 空 |
| finalizer 路径 | **legacy shape path**,走 `list_of_symbols` 软约束 |
| answer_chains | top 3 正确(`RegisterDefaultSubAgents → SubExplorer`) |
| LLM 输出 | 忽略 shape 约束,从 `enums.go` 拉 8 个 `AgentXxx` 常量 |

**失败层:L0-2 触发条件**。Enumeration kind 直接绕过 L0-2 抽取。

### 7.2 df1 run 4(`banned:Orchestrator/BaseAgent/SubAgentRuntime`)

| 环节 | 实际数据 |
|---|---|
| analyzer 输出 | `question_kind=call_chain`, `entities=["subagent"]` |
| identifyAnswerChains | 5 loose + 3 strict,answer_chain[0] 正确 |
| extractAnswerSymbols | **正确返回 `[SubExplorer]`** |
| finalizer prompt | **"Extracted Answer Symbols" + "Translation mode" + "EXACTLY these" 全部存在** |
| LLM 输出 | **完全忽略 symbol list**,写 `Orchestrator/BaseAgent/SubAgentRuntime` |

运行期 prompt(从 debug log 抓出):

```
## Extracted Answer Symbols (deterministic, authoritative)
The deterministic pipeline has already identified the answer to this question.
Your task is to render these symbols as prose. You MUST NOT add or remove symbols;
your training-data recall is irrelevant here.

- **SubExplorer** (internal/agent/sub_explorer.go:20)

Strict rules:
1. Your answer lists EXACTLY these symbols, no others.
2. For each symbol, cite its file:line if provided.
3. If a plausible-looking name is not in the list above, it is NOT part of the answer.
```

**LLM 对此视而不见**。实际输出:

```
有两个具体代理（agent）可以调用subagent：Orchestrator 和 BaseAgent。
• internal/orchestrator/orchestrator.go:18 — Orchestrator 结构体...
• internal/agent/agent.go:103 — BaseAgent 使用 SubAgents 作为其依赖...
```

**失败层:LLM 指令遵循**。确定性管线都做对了,gpt-4o 自由发挥。

### 7.3 df3 run 3(`missing:ContinuationPrompt` → "(no result)")

| 环节 | 实际数据 |
|---|---|
| analyzer **iter=0** | todo_write 创建 task `49fe6c8d` (kind=mechanism) |
| analyzer **iter=1** | todo_write **再次调用 REPLACE** 为 task `bf7d4a1a` (kind=enumeration) |
| 外层 pipeline loop | 仍然以旧 taskID `49fe6c8d` 运行 |
| explorer + finalizer | 正常执行,finalizer `content_len=124`,**产出有效答案**包含 `ContinuationPrompt` |
| `recordTaskFinalize("49fe6c8d", out)` | → `UpdateTaskResult` 遍历 tasks,**旧 ID 找不到,静默 fall through** |
| Renderer | 所有 `Task.Result` 为空 → `(no result)` |

**失败层:Orchestrator 任务身份违反**。

关键证据是 log 里的这三行:

```
2026-04-12T10:07:53.812 DEBUG [diag finalizer] iter=0 ASSISTANT content_len=124 tool_calls=0
2026-04-12T10:07:53.812 DEBUG [diag finalizer] iter=0 ASSISTANT content:
## Answer
**Answer:** 
- `ContinuationPrompt`
- `ShouldStop`
- `MidLoopCheck`
...
2026-04-12T10:07:53.813 INFO final answer:
(no result)
```

finalizer 产出了 124 字节的有效答案,里面有 `ContinuationPrompt`,但用户最终看到的是 `(no result)`。差距就在这两行之间 — `recordTaskFinalize → UpdateTaskResult → Tasks[i].Result` 的赋值被一次静默的"ID not found" 落空了。

### 7.4 三层独立的根因

**关键结论**:三个失败不是同一个 bug 的三种表现,而是三个不同层的独立 bug,**任何单层修复都不够**。

| 失败 | 失败层 | 根因 |
|---|---|---|
| df1 run 2 | L0-2 触发 | `extractAnswerSymbols` 只对 `registration/call_chain/return_value` 激活,`enumeration` 跳过 |
| df1 run 4 | LLM | gpt-4o 无视 prompt 硬约束 |
| df3 run 3 | Orchestrator | `todo_write` replace-all 导致 taskID 孤立,`UpdateTaskResult` 静默丢失 |

## 8. 三层修复方案

### 8.1 S1:任务身份 fallback(`be0a47e`)

**根因推导**:

1. 外层 pipeline loop 调用 `nextPendingTask()` 拿到一个 task 指针,取 `ID` 作为 `taskID`
2. 调用 `runTaskPipeline(taskID, ...)`,进入一个固定 taskID 的循环
3. analyzer 再次调用 `todo_write`,这是一个 tool,tool 执行时能直接 mutate `BusContext.Mutable.TaskList`
4. `todo_write` 的语义是 **full replacement**(见 `internal/tool/todo_write.go` 的注释:"full-list replacement. The caller must send the complete desired state every call")
5. TaskList 被换掉后,旧 taskID 不再存在
6. 流程正常走完 explorer + finalizer
7. `recordTaskFinalize(taskID, out)` 调用 `UpdateTaskResult(taskID, finalAnswer, TaskDone)`
8. `UpdateTaskResult` 老版本的实现:

```go
func (m *MutableState) UpdateTaskResult(id, result string, status TaskStatus) {
    m.mu.Lock()
    defer m.mu.Unlock()
    for i := range m.taskList.Tasks {
        if m.taskList.Tasks[i].Id == id {
            m.taskList.Tasks[i].Result = result
            m.taskList.Tasks[i].Status = status
            return
        }
    }
    // 线性扫描结束,什么都没做 ← 静默丢失!
}
```

9. 没有 match 的分支 `return` 默默掉出,`finalAnswer` 丢掉
10. `Renderer.RenderResult` 遍历 `Tasks[i].Result`,都是空字符串,输出 `(no result)`

**修复原理**:

核心修改是让 `UpdateTaskResult` 在找不到 ID 时 fall back 到一个合理的 target task,按优先级:**in_progress → pending → first task**。同时把返回值从 `void` 改为 `string`(实际写入的 ID),这样 caller 能检测到 fallback 发生并记日志。

```go
func (m *MutableState) UpdateTaskResult(id, result string, status TaskStatus) string {
    if m == nil { return "" }
    m.mu.Lock()
    defer m.mu.Unlock()
    for i := range m.taskList.Tasks {
        if m.taskList.Tasks[i].ID == id {
            m.taskList.Tasks[i].Result = result
            m.taskList.Tasks[i].Status = status
            return id
        }
    }
    // Fallback: in_progress → pending → first
    idx := -1
    for i := range m.taskList.Tasks {
        if m.taskList.Tasks[i].Status == TaskInProgress { idx = i; break }
    }
    if idx < 0 {
        for i := range m.taskList.Tasks {
            if m.taskList.Tasks[i].Status == TaskPending { idx = i; break }
        }
    }
    if idx < 0 && len(m.taskList.Tasks) > 0 { idx = 0 }
    if idx >= 0 {
        m.taskList.Tasks[idx].Result = result
        m.taskList.Tasks[idx].Status = status
        return m.taskList.Tasks[idx].ID
    }
    return ""
}
```

Caller(`recordTaskFinalize`)更新:

```go
actual := o.busCtx.Mutable.UpdateTaskResult(taskID, answer, types.TaskDone)
if actual == "" {
    logging.Warning("[orchestrator] recordTaskFinalize: task list empty; finalizer answer (%d bytes) dropped",
        len(answer))
} else if actual != taskID {
    logging.Warning("[orchestrator] recordTaskFinalize: task ID %q not found, fell back to %q (likely a mid-pipeline todo_write replacement)",
        taskID, actual)
}
```

**设计要点**:
- **Demote-not-drop**:把静默数据丢失改为"写到能写的地方"+"记日志",是严格加法,不会破坏任何现有场景
- **返回实际 ID**:让 caller 观察到 fallback 事件,不隐藏
- **优先级 in_progress 优先**:通常 analyzer 再次 todo_write 后新 task 会被标记 in_progress;用它作为 target 最接近"原本要发生的事"

**过拟合审计**(按 `feedback_no_overfitted_solutions` 的 5 项):
- Reverse:去掉 fallback = 恢复静默丢失 bug,反向破坏
- Deletion:没有 df3 这条规则也成立("静默数据丢失是 bug")
- Class:所有 taskID 被替换的场景,不仅 df3
- No-bait:正常路径(ID 匹配)完全不受影响
- No-contamination:规则来自"数据不应静默丢失"的不变量,不是从 df3 反推
**5/5 pass**

测试:5 个单测覆盖匹配 ID、fallback 到 in_progress/pending/first、空列表边界。

### 8.2 S2:证据驱动的 L0-2 触发(`86611bf`)

**根因推导**:

1. 早期 L0-2 设计为"只对某些 question_kind 触发"
2. `terminalKinds = {"registration", "call_chain", "return_value"}`,其他 kind 直接返回 nil
3. df1 的分析器对"有多少个 agent 可以调用 subagent?"分类:
   - iter 0 可能分为 `enumeration`(因为"多少")
   - 也可能分为 `call_chain`(因为"调用")
   - 或 `registration`(implied by call_chain)
4. 当 analyzer 把它分为 `enumeration` 时,L0-2 被跳过
5. 但 enumeration 问题的答案可能**完全是一个符号列表** — 比如"列出所有注册的 sub-agent"
6. 跳过 L0-2 后,finalizer 退回到遗留的 shape-based 软约束,gpt-4o 无视它

**修复原理**:

L0-2 触发条件不应该基于 analyzer 的 question_kind 分类(太依赖分析器的准确性),而应该基于**证据本身的形状**(shape-based trigger)。只要 strict subset 里有任何一个带"单符号终端"形状的证据,就应该抽取。

```go
func extractAnswerSymbols(items []EvidenceItem, questionKind string, graph *repomap.Graph) []AnswerSymbol {
    if len(items) == 0 { return nil }
    
    // S2:证据驱动的门,不再依赖 questionKind
    if !hasTerminalEvidence(items) {
        return nil
    }
    // ... extract ...
}

func hasTerminalEvidence(items []EvidenceItem) bool {
    for _, ev := range items {
        if isRegistrationShape(ev) { return true }       // Concrete + binds
        if ev.Kind == EvidenceConcrete && ev.Predicate == "returns" { return true }
        if ev.Kind == EvidenceRegistration { return true }
        if ev.Kind == EvidenceRelationship && ev.Predicate == "calls" { return true }
        if ev.Kind == EvidenceDataflowPath && ev.Predicate == "resolution_chain" { return true }
    }
    return false
}
```

**设计要点**:
- **解耦 analyzer 误分类的影响**:即使 analyzer 把 registration 问题分为 enumeration,只要 explorer 产出了 registration 证据,L0-2 仍然能触发
- **mechanism 问题仍然跳过**:mechanism 证据不带单符号终端,`hasTerminalEvidence` 对它返回 false,legacy 路径继续生效(df3 不受影响)
- **questionKind 仍然写到 `sym.Kind` 字段**用于下游报告,只是不再是 gate

**过拟合审计**:
- Reverse:返回 kind-based gate = 恢复 df1 run 2 的 bug
- Deletion:"translation 层应该对可翻译的证据都运行"是一般性原则
- Class:所有可能产出 registration 证据的问题,不仅 df1
- No-bait:mechanism 问题的 step 证据没有单符号终端,返回 nil,legacy path 正常
- No-contamination:`hasTerminalEvidence` 检查的是 EvidenceItem 的结构字段,不是问题文本或 ground truth
**5/5 pass**

### 8.3 S3:Finalizer retry loop 符号集校验(`c92068b`)

**根因推导**:

1. df1 run 4 的 finalizer prompt 里有明确的"Translation mode: EXACTLY these symbols"段
2. gpt-4o 依然输出了 `Orchestrator/BaseAgent/SubAgentRuntime`
3. 原因分析:gpt-4o 看到 `EvidenceItems` 段里提到 `Orchestrator/BaseAgent`(作为 context),"记忆力"驱动它把这些名字当成答案的一部分
4. **prompt-level soft constraint 对 gpt-4o 不是铁律**;需要**结构性验证**

**修复原理**:

让 `finalizerEvaluator` 参与 ReAct loop。translation 模式下:
- `ShouldStop` 返回 false(允许 soft-stop path 走到 ContinuationPrompt)
- `ContinuationPrompt` 在 soft-stop 时被调用,校验 LLM 响应里的符号集
- 任何"out-of-list 符号"触发 correction prompt 并重试
- 重试上限 2 次(防止死循环)

首先把 `finalizerEvaluator` 从 stateless 改为 stateful:

```go
type finalizerEvaluator struct {
    answerSymbols    []types.AnswerSymbol
    allowedSymbolSet map[string]bool
    retriesUsed      int
}

const maxFinalizerCorrectionRetries = 2
```

在 `BuildInitialPrompt` 时捕获状态:

```go
func (e *finalizerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
    e.answerSymbols = ctx.AnswerSymbols
    e.allowedSymbolSet = make(map[string]bool, len(ctx.AnswerSymbols))
    for _, s := range ctx.AnswerSymbols {
        e.allowedSymbolSet[s.Name] = true
    }
    e.retriesUsed = 0
    // ... prompt building ...
}
```

`ShouldStop` 条件化:

```go
func (e *finalizerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
    // translation 模式:let ReAct loop 走到 soft-stop + ContinuationPrompt
    if len(e.allowedSymbolSet) == 0 {
        return true // legacy 一次性
    }
    return false
}
```

核心的 `ContinuationPrompt` 校验:

```go
func (e *finalizerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, _ []types.ToolResult) (string, bool) {
    if len(e.allowedSymbolSet) == 0 { return "", false }
    if e.retriesUsed >= maxFinalizerCorrectionRetries {
        logging.Debug("[finalizer] S3 retries exhausted, accepting response")
        return "", false
    }

    violations := outOfListSymbols(resp.Content, e.allowedSymbolSet)
    if len(violations) == 0 { return "", false }

    e.retriesUsed++
    logging.Debug("[finalizer] S3 correction #%d: out-of-list symbols: %v", e.retriesUsed, violations)

    var b strings.Builder
    b.WriteString("Your previous answer contained symbol names that are NOT in the authoritative Answer Symbols list:\n\n")
    for _, v := range violations {
        fmt.Fprintf(&b, "  - `%s` ← not allowed\n", v)
    }
    b.WriteString("\nThe ONLY symbols you may mention are:\n")
    for _, s := range e.answerSymbols {
        if s.File != "" {
            fmt.Fprintf(&b, "  - **%s** (%s:%d)\n", s.Name, s.File, s.Line)
        } else {
            fmt.Fprintf(&b, "  - **%s**\n", s.Name)
        }
    }
    b.WriteString("\nRewrite your answer listing EXACTLY these symbols and no others.\n")
    return b.String(), true
}
```

关键辅助函数 `outOfListSymbols`:

```go
func outOfListSymbols(text string, allowed map[string]bool) []string {
    if text == "" { return nil }
    var out []string
    seen := make(map[string]bool)
    n := len(text)
    for i := 0; i < n; i++ {
        if i > 0 && isIdentChar(text[i-1]) { continue }
        if !isIdentStart(text[i]) { continue }
        j := i
        for j < n && isIdentChar(text[j]) { j++ }
        tok := text[i:j]
        i = j - 1
        if !looksLikeCodeSymbol(tok) { continue }
        if allowed[tok] || seen[tok] { continue }
        seen[tok] = true
        out = append(out, tok)
    }
    return out
}

func looksLikeCodeSymbol(tok string) bool {
    if len(tok) < 3 { return false }
    // snake_case
    if strings.Contains(tok, "_") { return true }
    // CamelCase (第二个大写字母)
    for i := 1; i < len(tok); i++ {
        if tok[i] >= 'A' && tok[i] <= 'Z' { return true }
    }
    // ≥8 字符的单词大写 — Go identifiers like "Orchestrator", "Explorer"
    if tok[0] >= 'A' && tok[0] <= 'Z' && len(tok) >= 8 { return true }
    return false
}
```

**关键设计决定**:`looksLikeCodeSymbol` 的结构性过滤不使用英文停用词表。过滤规则是:

1. 含下划线 → snake_case → 几乎一定是代码
2. 内部有大写 → CamelCase → 几乎一定是代码
3. 首字母大写 + 长度 ≥ 8 字符 → 覆盖 `Orchestrator` / `Explorer` / `SubExplorer` / `BaseAgent`;同时跳过常见英文 prose 大写词 `The/Answer/Summary/Context/Content/Example/Caveat`(都 ≤ 7 字符且无内部大写)

**8 字符阈值的来源**:来自**英文词汇分布**,不是 df1 的任何符号。常见英文 prose 首字母大写词绝大多数 ≤7 字符;Go identifiers 大于 8 字符是常态。这是语言层面的统计规律,不是对 case 的拟合。

**为什么不用英文停用词表?** 停用词表是 `feedback_overfitting_audit_stopwords` 明令反对的做法。结构性规则更稳健。

**过拟合审计**:
- Reverse:降低阈值 → Summary/Context 被误报;提高 → Orchestrator 被漏掉。8 是 sweet spot
- Deletion:"validate LLM output against authoritative list" 是通用 LLM reliability 技术
- Class:所有 list_of_symbols 问题进入 translation mode 后都适用
- No-bait:mechanism/step_list 问题不进 translation mode,不受影响
- No-contamination:filter 是 shape-based,8 字符阈值来自英文分布
**5/5 pass**

## 9. eval 回归对比

从 1/5 爬到 5/5 的全过程:

| # | Commit | df1 | df3 | 关键特征 |
|---|---|:-:|:-:|---|
| 基线 | `d28fc80` | 1-2/5 final | 5/5 | 早上里程碑;确定性层正确,LLM 层失败 |
| 1 | `a72015f` | 1/5 | — | L0-2 v1;soft constraint 被 gpt-4o 无视 |
| 2 | `497fb12` | 2/5 | 5/5 | L0-1 compound origin + L0-2 strict;构造器链被严格过滤 |
| 3 | `b595033` | 3/5 | **4/5** | EvidenceItem 重构;df3 首次回归,暴露 S1 bug |
| 4 | **`c92068b`** | **5/5** ✅ | **5/5** ✅ | **S1 + S2 + S3 协调修复** |

## 10. 8 次 commit 时间线

```
8d9c35f  L0-1 terminal verification
a72015f  L0-2 v1 extract-then-express
f34c0ac  L0-1 constructor-origin demote
497fb12  L0-1+L0-2 hardening (strict + compound)
b595033  L0-2 refactor: EvidenceItem-based extraction
───── 此处用户介入,建立 "trace full data flow" 规则 ─────
be0a47e  S1: UpdateTaskResult stale-ID fallback
86611bf  S2: evidence-driven L0-2 trigger
c92068b  S3: finalizer retry-loop symbol validation
```

前 5 次 commit 是 "看到问题修那一层" 的迭代,后 3 次是一次完整 trace 后的协调修复。**后 3 次的总 LOC 不比前 5 次多,但直接把 df1 推到 5/5**。

## 11. 核心教训

### 11.1 局部优化的代价是巨大的

每一次局部修复:
1. 看起来都是对的(每次审计都通过)
2. 每次都能在"单一失败模式"上看到改善
3. 但**整体 pass rate 在 1-3/5 之间震荡**,平均 ≤ 3/5

端到端 trace 后的一次协调修复,直接到 5/5。**不是每次修复都不够好,是每次都漏了 2/3 的问题**。

### 11.2 Debug 前必须先列数据流

"每一环都检查"包括:
- Analyzer 产出的 TaskItem 各字段(log: `diag analyzer ... tool=todo_write params=...`)
- explorer 的 answer_chain log
- L0-1 的 demote log
- L0-2 的 answer_symbol log
- context builder 拼出的 prompt 段(log: `diag finalizer INIT msg ...`)
- LLM 实际产出的 content(log: `diag finalizer iter=0 ASSISTANT content:`)
- `INFO final answer:` 的输出

**任何一环的实际数据都不能"推理"代替"观察"**。df3 run 3 的诊断就靠这个:finalizer log 里 content_len=124 明明有内容,`INFO final answer` 却是空的,这之间的 gap 直接指向 `recordTaskFinalize → UpdateTaskResult` 的静默丢失。

### 11.3 "静默失败"是一等的数据流反模式

`UpdateTaskResult` 的原实现不是 bug — 线性查找一个不存在的 ID 然后返回是非常合理的。但在**这个 caller 调用路径**里,不存在 = 数据丢失 = 用户看不到答案。任何"搜不到 → silent return" 的代码都应该要么记 warning,要么返回 bool/error 让 caller 知道。

### 11.4 结构性规则优于词表

S3 的 `outOfListSymbols` 最初写成了"有白名单 prose words = The/Answer/Summary",测试就暴露问题:`The` 也没被包括,fails。

改成结构性规则(CamelCase / snake_case / ≥8 char cap)后:
- 不需要列任何词表
- 跨语言可迁移(不仅英文 prose)
- 过拟合审计轻松过关 — 规则来自 Go naming convention + 英文词汇长度分布,不是 df1 的 ground truth

这呼应了 `feedback_overfitting_audit_stopwords` 和 `feedback_shape_based_ranking` 这两条老规则。

### 11.5 Demote-not-drop 的价值

L0-1 的所有谓词都是 demote(×0.1 / ×0.2),不是 drop。理由:
- 如果谓词太严 / case 刚好不符合规范,drop 会导致 answerChains 空输出
- demote 保证 "至少有 fallback"
- 即使 fallback 是错的,eval 会捕获,可以再调
- **drop 是一次性悬崖;demote 是柔性**

这个原则在 S1 里也用了:fallback 到 in_progress/pending/first 是 demote-not-drop 的一个版本 — 宁可写错地方也不要静默丢失。

## 12. 已知残余问题

5/5 全通过不代表系统完美。已记入新里程碑 `project_milestone_2026_04_12_10_of_10.md`:

1. **T2-T5 eval 用例缺失** — 当前只有 df1+df3 两个用例,10 样本方差大,需要 5×5 = 25 样本套件才能可靠判断未来改动的效果
2. **非 Go-convention registration 的覆盖** — S1 的 compound origin check 处理 `BindHandlers() binds ONLY NewFoo(deps)`,但没处理 Python `@register`、Java `@Bean`、Rust `#[derive]` 等注册模式
3. **S3 retry 耗尽的兜底** — 两次重试都失败时,finalizer 直接 ship 最后一个(仍错的)响应,用户侧没有"降级"标记
4. **Finalizer state 非 reentrant** — `finalizerEvaluator` 现在有 per-run 状态,通过 `BuildInitialPrompt` 重置。orchestrator 串行执行时无竞争;未来并行 task 流水线会有问题
5. **5 个未测试的中间 pipeline agent**(planner / implementer / reviewers / verifier)
6. **Mechanism scan v2**(里程碑 N4) — 仍然 deferred

## 13. 所有相关文件引用

### 代码文件

- `internal/types/evidence.go` — `AnswerSymbol` 类型定义
- `internal/types/context.go` — `BusContext.AnswerSymbols`、`AgentContext.AnswerSymbols`、`UpdateTaskResult` 实现
- `internal/types/task.go` — `TaskItem`、`TaskList`
- `internal/agent/agent.go` — `StageOutput.AnswerSymbols`、`Evaluator` 接口、`ContinuingEvaluator`
- `internal/agent/erm.go` — `identifyAnswerChains`、`extractAnswerSymbols`、`answerSymbolFromEvidence`、terminal + origin predicates、`hasTerminalEvidence`
- `internal/agent/explorer.go` — L0-1/L0-2 的调用站点
- `internal/agent/finalizer.go` — Translation mode + `ContinuationPrompt` 验证
- `internal/orchestrator/orchestrator.go` — `recordTaskFinalize`、`applyStageOutput`
- `internal/context/builder.go` — prompt 的 "Extracted Answer Symbols" 段构建
- `internal/render/renderer.go` — `RenderResult` 读 `Tasks[i].Result`

### 记忆文件(Claude 的 cross-session memory)

- `project_milestone_2026_04_12_10_of_10.md` — 10/10 里程碑(最新状态)
- `project_first_principles_2026_04_12.md` — L0-1/L0-2 的原始第一性原则分析
- `project_L0_1_terminal_verification_design.md` — L0-1 详细设计
- `project_L0_2_extract_then_express_design.md` — L0-2 详细设计
- `project_S1_S2_S3_three_layer_fixes.md` — S1/S2/S3 详细设计 + trace 记录
- `feedback_trace_full_dataflow_before_fixing.md` — 本次调试催生的永久规则

---

**文档版本**:`c92068b` HEAD,2026-04-12。
**下次恢复**:从 `project_milestone_2026_04_12_10_of_10.md` 开始读。
