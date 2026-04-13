# Analyzer v3 重构方案（AnalysisIR / DAG / HDP / RiskMatrix / QualityGate）

> 状态：方案草案 v1
> 范围：Layer 1 orchestrator + Layer 2 analyzer/explorer/finalizer 契约重构
> 原则：**无历史兼容包袱**。旧的 `TaskItem.{QuestionKind, AnswerShape, Keywords, Entities, ...}` 作为 analyzer 出口字段将被直接删除，下游统一改读新的 `AnalysisIR`。
> 参考当前 HEAD：`d97bda8`（path-strip ship）。所有改动基于该基线。

---

## 0. 术语约定

| 术语 | 定义 |
|------|------|
| **AnalysisIR** | Analyzer 阶段唯一结构化产物。强类型中间表示，下游只能读 IR，不再读 `TaskItem` 的 analyzer 字段。 |
| **TaskGraph** | IR 中的有向图；节点是探索/验证/实施动作；边表达依赖与证伪反馈。 |
| **HDP** | Hypothesis-Driven Planning：每个节点必须绑定≥1条可证伪假设。 |
| **RiskMatrix** | 六维风险评分矩阵（0–5），替代目前的 `Writing/HighRisk` 二值。 |
| **AnswerContract** | 可机器判定的交付合同，前置在 analyzer，强制下游。 |
| **Scenario Compiler** | 把 `RequestModel` 编译成 `TaskGraph + EvidencePlan + AnswerContract` 的场景模板引擎。 |
| **Quality Gate** | Analyzer 出口的确定性自检。不达标即 `analysis_rejected`。 |

---

## 1. 背景与当前架构痛点摘要

以下仅列出本次重构要直接处理的问题（详细代码锚点见 §11）：

1. **真相源重复**：analyzer 输出同时写入 `TaskItem.{Keywords, Entities, QuestionKind, AnswerShape, Writing, HighRisk, Complexity}`，下游散落在 `explorer.go:179/257/265/1187`、`finalizer.go:54/86`、`orchestrator.go:716-732` 各自解释，任何 analyzer 行为变更都要改 3+ 处。
2. **线性 `TaskList`**：`types.TaskList` 是顺序队列，无法表达假设分支、并行探索、反事实路径。orchestrator 以 per-task mini-pipeline 顺序遍历（`orchestrator.go:187-193`），复杂问题的多假设验证只能靠 explorer 内部超限重试。
3. **无跨语言归一**：analyzer prompt 第 52-54 行仅以自然语言提示 "include BOTH Chinese and English forms"；无 canonical term graph，召回稳定性依赖 LLM 一次性输出。
4. **无可证伪假设**：MissingPiece 是布尔 hint，不携带假设状态，orchestrator `decideNextStage()` 无法根据假设证伪驱动回溯。
5. **风险模型二值**：`determineActivePolicy()` 只读 `Writing/HighRisk` 两个 bool，无法表达安全 / 数据完整性 / 兼容性 / 性能 / 运维 / 合规 的多维风险。
6. **Analyzer 无自检**：`ParseOutput` 的唯一 fail-safe 是"若 todo_write 未被调用则装一个默认任务"（`analyzer.go:117-132`），analyzer 从不拒绝自己的输出。
7. **合同松散**：`AnswerShape` 是 soft prompt；finalizer 靠 S3 扫描大写标识符做符号校验（`finalizer.go:161-197`），非结构化合同。
8. **单一 prompt 负担过重**：analyzer 依赖一张 90 行系统 prompt 承载所有场景，无模板编译，复杂/安全审计/性能分析等差异化策略无法显式表达。
9. **无反事实路径**：探索层易过拟合单一解释，无对照路径收敛机制。
10. **无 analyzer 专项评测**：`eval/cases/` 只有 df1/df3/t1–t5 端到端样例，analyzer 层的 intent accuracy / DAG validity / contract completeness 无独立指标。

---

## 2. 目标架构总览

```
User Request
   │
   ▼
┌───────────────────────── Analyzer v3 ─────────────────────────┐
│                                                               │
│  ① Language Normalizer  (internal/analysis/normalizer/)       │
│     → canonical TermGraph                                     │
│                                                               │
│  ② Request Understanding LLM call (few-shot, structured-out)  │
│     → RequestModel (intent/scenario/risk/ambiguity)           │
│                                                               │
│  ③ Scenario Compiler  (internal/analysis/compiler/)           │
│     → TaskGraph + EvidencePlan + AnswerContract               │
│                                                               │
│  ④ Hypothesis Planner                                         │
│     → HypothesisSet, 每个 TaskNode 绑定假设                    │
│                                                               │
│  ⑤ Risk Matrix Evaluator                                      │
│     → 6-dim RiskMatrix → RunPolicy                            │
│                                                               │
│  ⑥ Counterfactual Branch Expander (可选,按不确定度触发)        │
│     → counterfactual TaskNodes + reconcile_node               │
│                                                               │
│  ⑦ Quality Gate (deterministic)                               │
│     → pass / analysis_rejected(reason,retryable)              │
│                                                               │
└─────────────────────────┬─────────────────────────────────────┘
                          │ AnalysisIR (single source of truth)
                          ▼
               BusContext.AnalysisIR
                          │
          ┌───────────────┼───────────────┬──────────────┐
          ▼               ▼               ▼              ▼
      Orchestrator     Explorer        Finalizer    ContractChecker
      (DAG scheduler) (TermGraph     (AnswerContract  (reject/补证据)
                       + Hypotheses)   renderer)
```

关键不变式：
- **Analyzer 是 AnalysisIR 的唯一写者**；tool 仅允许通过 analyzer 自有的 structured-output 路径写入（不再使用 `todo_write`）。
- **下游只读 IR**；`AgentContext` 暴露的是 IR 的 narrowed 视图，不再塞 `CurrentTaskKeywords/Entities/QuestionKind/AnswerShape`。
- **RunPolicy 在 analyze 阶段一次固化**；orchestrator 只能读，不能降级。
- **每次 stage 切换都要跑 `ContractChecker`**；finalizer 失败则回退到补证据节点而非强行产出。

---

## 3. 数据模型：`AnalysisIR` 强类型规范

新增文件：`internal/types/analysis_ir.go`

```go
package types

// AnalysisIR is the sole output of the analyze stage.
// It replaces TaskItem.{Keywords, Entities, QuestionKind, AnswerShape,
// Writing, HighRisk, Complexity, Title, Description} as analyzer truth source.
type AnalysisIR struct {
    Version       string          `json:"version"`          // "v3"
    RequestModel  RequestModel    `json:"request_model"`
    TaskGraph     TaskGraph       `json:"task_graph"`
    EvidencePlan  EvidencePlan    `json:"evidence_plan"`
    AnswerContract AnswerContract `json:"answer_contract"`
    HypothesisSet []Hypothesis    `json:"hypothesis_set"`
    RunPolicy     RunPolicy       `json:"run_policy"`
    QualityGate   GateReport      `json:"quality_gate"`
    TraceID       string          `json:"trace_id"`
}

// ── 3.1 RequestModel ───────────────────────────────────────────
type RequestModel struct {
    RawRequest    string       `json:"raw_request"`     // 原始用户输入
    Language      string       `json:"language"`        // ISO 639-1: zh/en/...
    Intent        Intent       `json:"intent"`          // explain/trace/refactor/debug/...
    Scenario      Scenario     `json:"scenario"`        // 场景枚举（§6）
    TermGraph     TermGraph    `json:"term_graph"`
    Ambiguities   []Ambiguity  `json:"ambiguities"`
    RiskMatrix    RiskMatrix   `json:"risk_matrix"`
    Complexity    Complexity   `json:"complexity"`      // simple/moderate/complex
}

type Intent string
const (
    IntentExplain       Intent = "explain"       // 解释代码/机制
    IntentRootCause     Intent = "root_cause"    // 定位故障
    IntentTrace         Intent = "trace"         // 追踪数据流/调用链
    IntentEnumerate     Intent = "enumerate"     // 枚举集合
    IntentConfigQuery   Intent = "config_query"  // 查配置
    IntentReturnValue   Intent = "return_value"  // 查返回值/字面量
    IntentRefactor      Intent = "refactor"
    IntentBugfix        Intent = "bugfix"
    IntentSecurityAudit Intent = "security_audit"
    IntentUnknown       Intent = "unknown"
)

type Scenario string
const (
    ScenarioArchitectureExplain   Scenario = "architecture_explain"
    ScenarioRootCause             Scenario = "root_cause"
    ScenarioSecurityAudit         Scenario = "security_audit"
    ScenarioRefactorDesign        Scenario = "refactor_design"
    ScenarioConfigTrace           Scenario = "config_trace"
    ScenarioPerformanceBottleneck Scenario = "performance_bottleneck"
    ScenarioGeneric               Scenario = "generic"
)

type Complexity string // "simple" | "moderate" | "complex"

type Ambiguity struct {
    Clause       string   `json:"clause"`
    Options      []string `json:"options"`
    Resolution   string   `json:"resolution"` // how analyzer resolved it
}

// ── 3.2 TermGraph（canonical 术语图）─────────────────────────
type TermGraph struct {
    Canonical []CanonicalTerm `json:"canonical"`
    Aliases   []TermAlias     `json:"aliases"`
}

type CanonicalTerm struct {
    ID         string   `json:"id"`                // canonical_id
    Surface    string   `json:"surface"`           // canonical surface form
    Language   string   `json:"language"`          // zh/en/code
    Kind       TermKind `json:"kind"`              // symbol/concept/config/command/literal
    Domain     string   `json:"domain,omitempty"`  // e.g. "explorer","erm"
    Confidence float32  `json:"confidence"`
}

type TermKind string
const (
    TermSymbol  TermKind = "symbol"   // CamelCase/snake_case identifier
    TermConcept TermKind = "concept"  // 自然语言概念（需要被映射到 symbol）
    TermConfig  TermKind = "config"   // YAML/JSON key path
    TermCommand TermKind = "command"
    TermLiteral TermKind = "literal"
)

type TermAlias struct {
    Source     string  `json:"source"`
    Target     string  `json:"target"`       // → CanonicalTerm.ID
    Relation   string  `json:"relation"`     // "synonym" | "translation" | "abbreviation" | "instanceof"
    Confidence float32 `json:"confidence"`
}

// ── 3.3 TaskGraph（DAG）──────────────────────────────────────
type TaskGraph struct {
    Nodes           []TaskNode        `json:"nodes"`
    Edges           []TaskEdge        `json:"edges"`
    ExecutionPolicy ExecutionPolicy   `json:"execution_policy"`
}

type TaskNode struct {
    ID               string          `json:"id"`
    Type             TaskNodeType    `json:"type"`
    Objective        string          `json:"objective"`        // 自然语言目标
    Inputs           []string        `json:"inputs"`           // canonical term IDs
    Outputs          []string        `json:"outputs"`          // expected artifact names
    EntryConditions  []string        `json:"entry_conditions"` // 必须满足的前置信号
    ExitArtifacts    []string        `json:"exit_artifacts"`   // 必须产出的 artifact
    SuccessCriteria  []Criterion     `json:"success_criteria"` // machine-checkable
    Hypotheses       []string        `json:"hypotheses"`       // → HypothesisSet[].ID
    SearchHints      SearchHints     `json:"search_hints"`     // 给 explorer 的 keyword/entity hint
    IsCounterfactual bool            `json:"is_counterfactual,omitempty"`
    MaxRetries       int             `json:"max_retries"`
}

type TaskNodeType string
const (
    NodeProbe        TaskNodeType = "probe"          // 基础探测（无假设绑定）
    NodeEvidence     TaskNodeType = "evidence"       // 取证
    NodeValidate     TaskNodeType = "validate"       // 证伪假设
    NodeReconcile    TaskNodeType = "reconcile"      // 反事实分支收敛
    NodeDesign       TaskNodeType = "design"         // 设计（仅 writing）
    NodeImplement    TaskNodeType = "implement"      // 实施（仅 writing）
    NodeReview       TaskNodeType = "review"         // 评审（仅 writing）
    NodeVerify       TaskNodeType = "verify"         // 验证（仅 writing）
    NodeFinalize     TaskNodeType = "finalize"       // 交付
)

type TaskEdge struct {
    From     string   `json:"from"`
    To       string   `json:"to"`
    EdgeType EdgeType `json:"edge_type"`
    Guard    string   `json:"guard,omitempty"` // optional condition expression
}

type EdgeType string
const (
    EdgeHardDependency      EdgeType = "hard_dependency"      // from 必须先完成
    EdgeSoftDependency      EdgeType = "soft_dependency"      // 建议顺序
    EdgeValidationFeedback  EdgeType = "validation_feedback"  // to 失败回溯到 from
)

type ExecutionPolicy struct {
    MaxParallelism int      `json:"max_parallelism"`
    CriticalPath   []string `json:"critical_path"` // node IDs
    RetryBudget    int      `json:"retry_budget"`  // 整图重试上限
}

type Criterion struct {
    Kind string `json:"kind"` // "fact_count" | "symbol_present" | "hypothesis_state" | ...
    Expr string `json:"expr"` // DSL：稍后定义,v3 起步只支持少数 Kind
}

type SearchHints struct {
    KeywordIDs []string `json:"keyword_ids"` // → TermGraph ID
    EntityIDs  []string `json:"entity_ids"`  // → TermGraph ID
}

// ── 3.4 EvidencePlan ────────────────────────────────────────
type EvidencePlan struct {
    Budget        EvidenceBudget          `json:"budget"`
    SourceMix     map[string]int          `json:"source_mix"` // "grep":40,"repomap":30,"read":30
    StopConditions []StopCondition        `json:"stop_conditions"`
}

type EvidenceBudget struct {
    MaxFiles       int `json:"max_files"`
    MaxBytes       int `json:"max_bytes"`
    MaxReactIters  int `json:"max_react_iters"`
    MaxToolCalls   int `json:"max_tool_calls"`
}

type StopCondition struct {
    Kind string `json:"kind"` // "all_hypotheses_decided" | "contract_satisfied" | "budget_exhausted"
    Expr string `json:"expr,omitempty"`
}

// ── 3.5 AnswerContract（强合同）─────────────────────────────
type AnswerContract struct {
    RequiredAnswerShape AnswerShape  `json:"required_answer_shape"`
    MustInclude         []string     `json:"must_include"`         // canonical term IDs
    MustExclude         []string     `json:"must_exclude"`
    CitationReq         CitationReq  `json:"citation_requirements"`
    AcceptanceTests     []Acceptance `json:"acceptance_tests"`
    Language            string       `json:"language"` // 输出语言
}

type AnswerShape string
const (
    ShapeListOfSymbols AnswerShape = "list_of_symbols"
    ShapeStepList      AnswerShape = "step_list"
    ShapeValue         AnswerShape = "value"
    ShapeBoolean       AnswerShape = "boolean"
    ShapeConfigValue   AnswerShape = "config_value"
    ShapeExplanation   AnswerShape = "explanation" // 解释类（长文)
    ShapeNone          AnswerShape = "none"
)

type CitationReq struct {
    Required     bool   `json:"required"`
    Granularity  string `json:"granularity"` // "file" | "file_line" | "file_line_range"
    MinCitations int    `json:"min_citations"`
}

type Acceptance struct {
    Kind string `json:"kind"` // "contains_symbol"|"regex_match"|"symbol_set_exact"|"citation_count_ge"
    Expr string `json:"expr"`
}

// ── 3.6 HypothesisSet（HDP）─────────────────────────────────
type Hypothesis struct {
    ID                     string           `json:"id"`
    Statement              string           `json:"statement"`
    RequiredEvidence       []Criterion      `json:"required_evidence"`
    FalsificationCondition Criterion        `json:"falsification_condition"`
    Priority               int              `json:"priority"` // 0-100
    Status                 HypothesisStatus `json:"status"`
}

type HypothesisStatus string
const (
    HypUnknown   HypothesisStatus = "unknown"
    HypConfirmed HypothesisStatus = "confirmed"
    HypRejected  HypothesisStatus = "rejected"
)

// ── 3.7 RiskMatrix & RunPolicy ──────────────────────────────
type RiskMatrix struct {
    Security      RiskLevel `json:"security"`
    DataIntegrity RiskLevel `json:"data_integrity"`
    Compatibility RiskLevel `json:"compatibility"`
    Performance   RiskLevel `json:"performance"`
    Ops           RiskLevel `json:"ops"`
    Compliance    RiskLevel `json:"compliance"`
}

type RiskLevel struct {
    Level    int      `json:"level"`    // 0..5
    Evidence []string `json:"evidence"` // 自由文本理由
}

type RunPolicy struct {
    Writing             bool     `json:"writing"`
    RequireDesignReview bool     `json:"require_design_review"`
    RequireCodeReview   bool     `json:"require_code_review"`
    RequireVerify       bool     `json:"require_verify"`
    ForbidSkipStages    []string `json:"forbid_skip_stages"`
    // 注意：此字段是 analyzer→orchestrator 的只读固化结果
}

// ── 3.8 Quality Gate 报告 ───────────────────────────────────
type GateReport struct {
    Passed       bool          `json:"passed"`
    Rejected     bool          `json:"rejected"`
    Retryable    bool          `json:"retryable"`
    Checks       []GateCheck   `json:"checks"`
}

type GateCheck struct {
    Name      string  `json:"name"`      // "coverage"|"dag_closure"|"budget"|"contract"
    Passed    bool    `json:"passed"`
    Score     float32 `json:"score"`     // 0..1
    Threshold float32 `json:"threshold"`
    Detail    string  `json:"detail"`
}
```

### 3.9 TaskItem 删除字段

在 `internal/types/task.go` 中移除：
- `Writing bool` → 迁入 `RunPolicy.Writing`
- `HighRisk bool` → 由 `RiskMatrix` 推导
- `Complexity string` → 迁入 `RequestModel.Complexity`
- `Keywords []string` → 迁入 `TermGraph`（TaskNode 只引用 ID）
- `Entities []string` → 同上
- `QuestionKind string` → 迁入 `RequestModel.Intent`
- `AnswerShape string` → 迁入 `AnswerContract.RequiredAnswerShape`

`TaskItem` 降级为"执行态"容器：保留 `ID, Title, Description, Status, Result`。`TaskList` 完全退居为 orchestrator 的 per-node 执行登记簿（或直接被 TaskGraph 替代；见 §7 迁移批次 B3）。

### 3.10 BusContext 改造

`internal/types/context.go`：
- 新增字段 `AnalysisIR *AnalysisIR`（只允许 analyzer 写入；其它 stage 读，但可通过专用 API 回写 `HypothesisSet[i].Status` 与 `TaskGraph.Nodes[i].Status`）。
- 删除 `AgentContext` 中的 `CurrentTaskKeywords/Entities/QuestionKind/AnswerShape/...`，改为暴露 `*AnalysisIR` 引用 + `CurrentNodeID string`。
- `BuildAgentContext()` 根据 `CurrentNodeID` 定位 `TaskNode`，解析其 `SearchHints` 为 resolved keywords/entities（使用 TermGraph 查表）后填充 explorer 所需的临时视图字段（只对 explorer 可见，其它 agent 不透出）。

---

## 4. 模块新增 / 改动

### 4.1 新增目录

```
internal/analysis/
├── normalizer/          # §4.2
│   ├── extract.go       # term extraction
│   ├── canonicalize.go  # canonical form + language tag
│   ├── alias.go         # alias graph
│   ├── rules_zh.go      # 中文规则
│   ├── rules_en.go      # 英文规则
│   └── normalizer_test.go
├── compiler/            # §4.3
│   ├── scenario.go      # Scenario 分类器
│   ├── templates.go     # 六个场景模板
│   ├── compile.go       # RequestModel → TaskGraph/EvidencePlan/AnswerContract
│   └── compile_test.go
├── risk/                # §4.4
│   ├── matrix.go        # 6-dim scoring
│   ├── policy.go        # RiskMatrix → RunPolicy 映射
│   └── risk_test.go
├── hdp/                 # §4.5
│   ├── planner.go       # 假设规划
│   ├── binder.go        # 节点↔假设绑定校验
│   └── hdp_test.go
├── counterfactual/      # §4.6
│   ├── expander.go      # 反事实分支扩展
│   └── counterfactual_test.go
├── gate/                # §4.7
│   ├── gate.go          # deterministic quality gate
│   ├── checks.go        # 每项 check 独立函数
│   └── gate_test.go
└── contract/            # §4.8
    ├── checker.go       # 运行期合同校验
    └── checker_test.go
```

### 4.2 Language Normalizer

实现三步：
1. **Term Extraction**：混合策略——正则（CamelCase / snake_case / 点分路径 / 引号字面量 / `@decorator` / CLI flag）+ repo 符号表命中（调用现有 `internal/tool` 的 grep/repo_map 接口做 1 轮 bulk 查询）。
2. **Canonicalization**：为每个 surface 打 `language/kind/domain` 标签；合并 `Token/token_/Token()` 族归一到同一 canonical ID。
3. **Alias Graph Build**：翻译对（zh↔en）、同义词（词典内嵌 + `alias_zh.yaml`）、缩写、`is-a` 关系。

禁止 explorer 自由拼接 keywords——所有检索都必须经 `normalizer.Resolve(termID) → []string`（给出所有可能的 surface）。

### 4.3 Scenario Compiler

`Compile(rm RequestModel) (TaskGraph, EvidencePlan, AnswerContract, error)`

六个起步模板 + 兜底 `generic`：

| 模板 | 必备节点 | 证据预算 | 默认合同 |
|------|---------|---------|---------|
| architecture_explain | probe → evidence(×N) → reconcile → finalize | 中 | ShapeExplanation + min_citations=3 |
| root_cause | probe → evidence → validate(×hyp) → reconcile → finalize | 中高 | ShapeStepList + file_line citations |
| security_audit | probe → evidence(×risk_dim) → validate → review → finalize | 高 | ShapeListOfSymbols + must_include=CVE terms |
| refactor_design | probe → evidence → design → review → finalize | 中高 | ShapeExplanation + acceptance=design_contract |
| config_trace | probe → evidence → validate → finalize | 低 | ShapeConfigValue + file_line |
| performance_bottleneck | probe → evidence → validate → finalize | 中 | ShapeListOfSymbols |

分类策略：`RequestModel.Intent + Ambiguities + Scenario hints` → 规则表优先，LLM 兜底。

### 4.4 Risk Matrix

- `Evaluate(rm RequestModel) RiskMatrix`：启发式评分 + LLM 补齐（仅在风险词出现时触发，默认全 0）。
- `DerivePolicy(risk RiskMatrix, writing bool) RunPolicy`：

| 条件 | Policy |
|------|--------|
| writing=false | `require_*=false` |
| security≥3 或 compliance≥3 | `require_design_review=true, require_code_review=true` |
| data_integrity≥3 | `require_verify=true` |
| compatibility≥3 | `forbid_skip_stages=[design_review]` |
| performance≥4 | 附加 verify 节点 |

### 4.5 HDP Planner

`Plan(rm RequestModel, tg *TaskGraph) ([]Hypothesis, error)` 负责：
1. 基于 `Intent + TermGraph + Ambiguities` 生成候选假设。
2. 把 `Hypothesis` 绑定到 `TaskGraph.Nodes[*].Hypotheses`。
3. Binder 校验：每个 `NodeEvidence/NodeValidate` 必须至少绑定 1 条假设；`NodeProbe` 允许无绑定。

### 4.6 Counterfactual Expander

触发条件：`RequestModel.Ambiguities` 非空 且 `Complexity=complex` 且 `Intent ∈ {RootCause, SecurityAudit, Explain}`。

产物：为分歧子句生成 `counterfactual` 节点 + 一个 `NodeReconcile` 收敛节点。`reconcile` 节点的 `SuccessCriteria` 形如"两条分支中证据权重更高者胜出"。

反事实分支成本必须计入 `EvidencePlan.Budget`；若超预算则回退单路径。

### 4.7 Quality Gate（确定性）

`Run(ir *AnalysisIR) GateReport`，按顺序跑：

| Check | 阈值 | 失败动作 |
|------|------|---------|
| `coverage` | 请求关键子句被 `TaskGraph` 覆盖率 ≥ 0.9 | reject, retryable |
| `dag_closure` | 无悬空节点、无环、所有 Hard 依赖可达 | reject, retryable |
| `budget_sanity` | 复杂度×场景的预算在合法区间 | reject, retryable |
| `contract_complete` | AnswerContract 必填字段齐全 | reject, retryable |
| `hypothesis_coverage` | 每个关键子句至少对应 1 条 `priority≥50` 的假设 | reject, retryable |
| `risk_consistency` | writing=true 但 RiskMatrix 全 0 → 警告（非 reject） | warn |

`analyzer.ParseOutput`：若 `GateReport.Rejected=true`：
- `Retryable=true` → 通过 `StageOutput.Retry=true` + `Data.reason` 让 orchestrator 重跑 analyze（最多 `MaxRetriesPerStage` 次）。
- `Retryable=false` → `StageOutput.Err` 终止整条 run。

Gate 指标全部写 `logs/` 供评测采集。

### 4.8 Contract Checker（运行期）

`Check(ir *AnalysisIR, draft FinalAnswer) CheckResult`，在 finalizer 产出后、orchestrator 标记 `finalize` 完成前执行。

校验项：`AnswerShape` 匹配、`must_include`/`must_exclude` 符号集、`CitationReq` 引用数量与粒度、`AcceptanceTests` 全通过。

失败动作：orchestrator 将对应 `NodeFinalize` 标记 `blocked`，回溯到最近一个 `NodeEvidence` 节点补证据，计入 `RetryBudget`。超预算则以 `ContractViolation` 交付给用户（而不是静默降级）。

---

## 5. Orchestrator 改造

### 5.1 DAG 调度

`internal/orchestrator/orchestrator.go`：
- `Run()` 分两层：
  - 上层：`runAnalyzeStage()` → 得到 `AnalysisIR` → `runPolicy` 固化。
  - 下层：`runTaskGraph(ir)` 按 DAG 调度，替代当前 `runTaskPipeline()` 的线性 per-task 循环。
- 新增 `scheduler/` 子包（或直接放 `orchestrator/scheduler.go`）：
  - `ReadyNodes(graph, state) []TaskNode`：返回所有 Hard 依赖已完成的节点。
  - `Dispatch(node)`：映射 `TaskNodeType → agent` （保留现有 8 个 agent 类型作为实现层）。
  - 并行度受 `ExecutionPolicy.MaxParallelism` 限制（v3 起步 **强制 `MaxParallelism=1`**，后续批次再真正并行；见 §7 B5）。
- 失败回溯：节点失败时按 `validation_feedback` 边选择回溯目标；若无反馈边则按现有 retry-then-finalize 逻辑降级。

### 5.2 信号语义

- 删除 `ExecutionSignals.HasEnoughFacts/HasPlan/HasPatch/...` 的布尔语义，改为 `NodeStates map[nodeID]NodeState`（`pending/running/done/failed/blocked`）+ `HypothesisStates map[hypID]HypothesisStatus`。
- `decideNextStage` 被 `scheduler.NextNodes` 取代；`isTransitionValidBySignals` 改为 `isNodeReady`。

### 5.3 Policy 只读性

`determineActivePolicy` 完全删除。`runAnalyzeStage` 产出后写 `busCtx.RunPolicy = ir.RunPolicy`，后续 stage 只能读。

---

## 6. Analyzer Agent 改造

`internal/agent/analyzer.go`：
- `BuildInitialPrompt` 改为结构化输出合同（structured output / JSON schema）。prompt 主体只描述 IR 顶层字段与 few-shot。完整 IR schema 以 JSON schema 注入 provider（若 provider 不支持 schema，则 prompt 尾部附上 JSON Schema 片段 + 解析降级路径）。
- 删除 `todo_write` 作为 analyzer 出口——analyzer 在一次 LLM 调用中直接返回 `AnalysisIR` JSON；`ParseOutput` 解析 JSON 并依次调用：
  1. `normalizer.Enrich(ir)` — 补齐 TermGraph 的 canonical 映射。
  2. `compiler.Compile(ir.RequestModel)` — 若 LLM 未产 TaskGraph 则编译器补齐（保证 LLM 只需写 `RequestModel` 即可）。
  3. `risk.DerivePolicy(...)` — 固化 RunPolicy。
  4. `hdp.Plan(...)` — 假设绑定。
  5. `counterfactual.Expand(...)` — 按触发条件。
  6. `gate.Run(ir)` — Quality Gate。
- `ShouldStop` 保持"无 tool calls 即停"，但因 analyzer 不再使用 tool（除了 normalizer 的 bulk repo 查询），实际只跑 1 轮。
- 所有 analyzer 相关的 prompt 常量集中到 `internal/agent/analyzer_prompt.go`，便于评测回归。

---

## 7. 迁移批次（分阶段交付）

严格按批次顺序，每个批次独立 commit、独立 review、独立跑 eval baseline。每批次附带单元测试与最小端到端回归。

| 批次 | 交付物 | 验收 |
|------|--------|------|
| **B0** | 本方案合并到 `docs/` + CLAUDE.md 增补指引 + eval baseline `analyzer_v3_baseline.json` 抓取当前 HEAD 行为快照 | 方案文档 merged；baseline 指标（df1/df3/t1-t5 + 未来 v3 评测雏形）落盘 |
| **B1** | `internal/types/analysis_ir.go` 新结构体（纯定义，不接入运行时）+ JSON round-trip 测试 | `go test ./internal/types/...` 通过 |
| **B2** | `internal/analysis/normalizer/` 全量 + 中/英规则 + 单测 | normalizer 独立测试集 precision/recall ≥ 0.9 |
| **B3** | `internal/analysis/compiler/` + 六模板 + 单测 | 每个模板 3+ 样例单测通过 |
| **B4** | `internal/analysis/risk/` + `hdp/` + `counterfactual/` + `gate/` + `contract/` + 各自单测 | 各子模块单测通过；集成测试桩 |
| **B5** | Analyzer agent 改造（`analyzer.go` 全替换）+ BusContext/AgentContext 字段迁移（TaskItem 字段删除）+ explorer/finalizer/orchestrator 下游改读 IR | 原 `analyzer_test.go`、`erm_test.go`、`answer_shape_gate_test.go`、`finalizer_test.go` 全部迁移版本通过 |
| **B6** | Orchestrator DAG scheduler（串行版，`MaxParallelism=1`）替换 per-task mini-pipeline | `orchestrator_test.go` 与端到端 df1/df3/t1-t5 至少持平 HEAD baseline |
| **B7** | Contract Checker 接入 finalizer 出口 + 回溯逻辑 | 合同失败的样例能触发回溯；不误伤现有通过样例 |
| **B8** | `eval/analyzer_v3/` 评测框架（见 §9）+ 发布门禁脚本 | CI 门禁脚本跑通，能阻断 regression |
| **B9** | 真正的并行调度（`MaxParallelism>1`）+ 反事实分支真触发 | 多假设样例纳入 eval，`HypothesisStates` 驱动流程 |

**回滚策略**：B1–B4 纯新增，回滚即 `git revert`；B5–B7 是破坏性改动，每批次用独立分支 `refactor/analyzer-v3-bN`，合并前必须过 eval baseline。发现回归立即回退对应批次，不强行往前推。

---

## 8. 测试策略

### 8.1 单元测试

- 每个新模块必须有 table-driven 测试，覆盖正常/边界/异常三档。
- `analyzer_test.go` 重写为 schema roundtrip + gate pass/reject 两组表。
- explorer/finalizer 的下游测试迁移到 `ctx.AnalysisIR` 注入模式。

### 8.2 集成测试

`internal/agent/integration_analyzer_v3_test.go`（新增）：
- 端到端构造 `BusContext`，跑 analyzer→explorer→finalizer 全链路，断言 `AnalysisIR` 字段稳定、`ContractChecker` 通过。
- 中/英双语各 3 个样例。

### 8.3 回归保护

在 B5–B7 每次提交前：
```bash
make test && \
./eval/run.sh df1 df3 t1 t2 t3 t4 t5
```
必须维持 35/35 PASS（HEAD `d97bda8` baseline）。任何下降需分析根因——LLM 方差不作为借口（遵循 `feedback_first_principles_root_cause`）。

### 8.4 Overfit 审计

遵循 `feedback_no_overfitted_solutions`：每批次新增的规则/启发式（normalizer 规则、compiler 模板、risk 表）都要做 5 项反向测试（删除测试、类扩展、无诱饵、无污染、反向措辞），通过才能合并。

---

## 9. Analyzer v3 评测框架

新增 `eval/analyzer_v3/`：

```
eval/analyzer_v3/
├── cases/
│   ├── intent/          # intent classification 子集
│   ├── multilang/       # zh/en/mix
│   ├── ambiguous/       # 高歧义
│   ├── cross_module/    # 跨模块复杂问题
│   └── long_context/    # 长上下文
├── ground_truth/        # 每样例期望的 AnalysisIR 片段
├── metrics/
│   └── analyzer_v3.go   # 指标计算器
└── run_v3.sh
```

**指标**：

| 指标 | 计算 | 基线目标 |
|------|------|---------|
| Intent accuracy | exact match | ≥ 0.9 |
| Scenario accuracy | exact match | ≥ 0.85 |
| DAG validity | gate.dag_closure pass rate | = 1.0 |
| Coverage score | gate.coverage 平均 | ≥ 0.9 |
| Contract completeness | 必填字段齐全率 | = 1.0 |
| Ambiguity handling | 歧义条款被 RequestModel.Ambiguities 显式记录率 | ≥ 0.8 |
| Downstream success rate | 接入后 df/t 集合通过率 | ≥ HEAD baseline |

**发布门禁**：任何一项低于基线直接阻断合并（CI 脚本 `make eval-analyzer-v3`）。

**失败样例聚类**：`analyzer_v3_report.md` 自动生成，按 `gate.Checks` 失败项分桶，反哺模板与 normalizer 规则。

---

## 10. 风险与未解问题

1. **LLM 结构化输出稳定性**：analyzer 改成一次性吐 IR JSON 后，JSON schema 违规就是整次 analyze 失败。缓解：(a) 使用 provider 原生 structured output；(b) 降级解析 + 单点补齐（由 compiler/normalizer 填充 LLM 缺省字段）。
2. **TermGraph 冷启动成本**：normalizer 的 canonical 词典首次构建需要脱机跑一次 repo symbol dump。缓解：在 `make` 里加 `make term-dict` 目标，产物写 `memory/term_dict.json` 并加入 gitignore。
3. **反事实分支预算失控**：若触发条件过宽，复杂问题证据预算会翻倍。缓解：v3 起步默认关闭反事实（B9 才启用），触发阈值通过 `codrax.yaml` 可调。
4. **并行调度与现有 Mutable 状态冲突**：`BusContext.Mutable` 目前假设单写者。B9 启用并行时必须为每个节点分配独立 `NodeScratch` 后合并，避免写冲突。
5. **RunPolicy 只读导致分析误判成本高**：若 analyzer 低估风险，下游无法升级。缓解：Contract Checker 失败时允许向 `RiskMatrix` 回写一次"升级标记"，触发一次 analyze 重跑（受 `RetryBudget` 约束）。
6. **评测数据集匮乏**：`eval/analyzer_v3/cases/` 需要人工构造；初期目标 30 个样例（5 个子集 × 6 条）。
7. **语言选择副作用**：`AnswerContract.Language` 取 `RequestModel.Language`，但用户可能 `-lang=off`——需要在 `ParseOutput` 里兜底成 `en`。

---

## 11. 现状代码锚点（供实施参考）

| 主题 | 文件:行 |
|------|---------|
| 当前 analyzer agent | `internal/agent/analyzer.go:22-173` |
| ParseOutput fail-safe | `internal/agent/analyzer.go:102-163` |
| TaskItem 定义 | `internal/types/task.go:11-70` |
| TaskList 定义 | `internal/types/task.go:73-77` |
| BusContext/AgentContext | `internal/types/context.go:218-341` |
| BuildAgentContext | `internal/context/builder.go:14-89` |
| Orchestrator.Run | `internal/orchestrator/orchestrator.go:127-212` |
| runTaskPipeline | `internal/orchestrator/orchestrator.go:274-390` |
| decideNextStage | `internal/orchestrator/orchestrator.go:677-707` |
| determineActivePolicy | `internal/orchestrator/orchestrator.go:716-732` |
| Explorer 消费 analyzer 字段 | `internal/agent/explorer.go:179 / 257-265 / 310 / 1187` |
| Finalizer translation mode | `internal/agent/finalizer.go:32-129` |
| Finalizer S3 symbol validation | `internal/agent/finalizer.go:131-197` |
| Task policies YAML | `config/orchestrator.yaml:127-152` |
| 语言 preference 注入 | `internal/orchestrator/orchestrator.go:59-96` |
| ERM hint 入口 | `internal/agent/erm.go:64-126` |
| 已存在的 analysis 包 | `internal/analysis/dataflow/` |

---

## 12. 下一步

B0 完成（本方案 merge）后，立即启动 B1（`analysis_ir.go` 类型定义 + roundtrip 测试）。每批次独立 PR / commit，commit message 前缀统一为 `analyzer-v3/bN:`，便于后续追溯与回滚。
