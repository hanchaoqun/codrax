> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# Block-Only Carrier 终局迁移：实施计划

| 项 | 值 |
|---|---|
| 文档版本 | 1.0 |
| 编写日期 | 2026-05-03 |
| 基线 commit | `e9f271e`（origin/main HEAD，含 P0a/P0b/P1 #2/P1 #3/P2 #4/P3 #6 oracle/P3 #6 typed-relation hint 7 个新 commit，叠加在用户方案文档基线 `1cb88400` 之上） |
| 上游设计文档 | memory `project_block_only_terminal_migration_plan.md` (verbatim 用户方案) |
| 关联 memory | `feedback_no_system_backfill_to_user_panel.md` / `feedback_precise_signals_for_hard_gates.md` / `feedback_no_overfitted_solutions.md` / `feedback_no_internal_info_in_llm_prompts.md` / `feedback_root_cause_only.md` / `feedback_no_defer_no_split_issues.md` |
| 终局判定 | 见 §10「终局 grep 归零清单」 — 主链命中数=0 才允许宣布完成 |

---

## 1. 总目标

把 read-mode 最终答案 carrier 从 V1 (`shape + summary/steps/symbols/value/boolean` schema) 完全迁移到 V2 block-only 架构（`AnswerDocumentV2 + Blocks[]`），中间通过 `AnswerSemanticView` 编译 block contract，终局**彻底删除** V1 carrier 与所有相关 helper / oracle / renderer / hedging / prompt 章节。

## 2. 非目标（明确不在本计划内）

- 不重造 `ClaimForm` / `FacetCoverageContract` / `RenderedClaimUse` / `AnswerSurfacePlan`（已落地的官方底座）。
- 不动 write-mode 的 `change_plan` / `change_report` 输出语义；只在 B8 把它们从 `AnswerShape` 拆到独立 `WriteOutputKind` enum。
- 不动 `EvidenceItem` / `EvidenceClosure` / `Citation` / `CodeSnippet` / `AnswerExactResolution` 这些共享数据结构（V1/V2 共用）。
- 不动 explorer / extractor 的投资逻辑；本计划只触碰 finalizer + emit + render + contract_check 链路。
- 不引入新的 ViolationKind（除非批次 §3.X 显式列出）；不增 yaml 旋钮（除非批次显式列出）。

---

## 3. 跨批次红线（每个 commit 必须自检）

以下 12 条红线**全部适用于每一批**。每个 PR / commit 提交前必须逐条核查，并在 commit message 末尾注明已核查。

### R1. LLM 是答案唯一作者
来源：`feedback_no_system_backfill_to_user_panel`
- 系统**永不**写入 `AnswerDocumentV2.Blocks[]` / `Caveats[]` / `Snippets[]` 等 LLM 待 emit 的字段。
- 系统可写：prompt 输入、validator reject、oracle violation、retry hint、telemetry、debug log。
- 任何"自动追加 caveat 行" / "自动补全 block" / "自动改写 summary block" 的 PR 一律拒绝。

### R2. 精确信号才能驱动硬门
来源：`feedback_precise_signals_for_hard_gates`
- 所有 V2 validator / oracle 的输入必须是 typed enum / typed flag / verbatim string match / typed graph lookup。
- 禁止 ranker score、相似度、关键词频率、prose substring 模糊匹配作为硬 reject 依据。

### R3. 不写关键词分类表
来源：`feedback_no_custom_keyword_matching`
- BlockRequirement 编译只读 `QuestionFamily` typed enum + `Predicates` typed flag + `AnswerSubject` typed enum。
- 严禁在 `block_requirement_compile.go` 或 prompt 里写关键词列表。

### R4. Prompt 不暴露内部术语
来源：`feedback_no_internal_info_in_llm_prompts`
- prompt 文本中**禁止**出现：`AnswerSemanticView` / `BlockRequirement` / `FacetCoverageContract` / `RenderedClaimUse` / `QuestionFamily` / `MutableState` / `StageOutput` 等 Go 类名 / 包名 / 文件名。
- prompt 用 LLM 通用语言："required answer blocks" / "facets each block must cover" / "claim shape this evidence supports" 等。

### R5. 单 oracle 一次失败模式
来源：`feedback_no_overfitted_solutions`
- 每个新 V2 oracle 必须能 articulate 一句话："这个 oracle 只 catch <这一类> 的失败"。
- 5-Q 审计：(a) 7 family 通用？(b) 不耦合本仓？(c) 不耦合 yaml 配置文件？(d) 不假设最大层级 / 步数 / bucket 数？(e) 不耦合单题型？

### R6. 不允许双 authority 长期共存
来源：用户方案 §3.1
- B1-B5：V1 主、V2 telemetry-only（V2 violation logged，不 reject）。
- B6：V2 主、V1 telemetry-only（V1 violation logged，不 reject）。
- B8：V1 oracle / validator / renderer / helper **全部删除**，无降级 fallback。
- 任何"V1 keep alive as backup"的代码路径都视为本计划失败。

### R7. 没有"先留着以后再说"的代码
来源：`feedback_no_defer_no_split_issues` + 用户方案 §6.0
- B8 一次性删旧。批次中间发现的 V1 残留必须在该批次解决，不允许"列入下批"。
- 任何 `// TODO: remove later` / `// kept for potential rollback` 的注释都需在 PR review 阶段被指出。

### R8. 根因修复非局部修复
来源：`feedback_root_cause_only`
- 本计划本身就是删根。每个批次内部修复也需走结构性路径。
- 严禁在 B5 / B6 阶段为绕过 V2 缺陷而往 V1 上贴补丁。

### R9. 推送前确认
来源：`feedback_confirm_before_push`
- 每个批次的 commit 提交完成后停下，向用户确认才 push。
- 紧急修复（CI 红）例外，但需当场说明。

### R10. 删 LLM-facing 指令前三步走
来源：`feedback_redundant_inline_directive_removal`
- B6 / B8 删 prompt 章节前必须：grep canonical → 逐 agent 消费者排查 → 端到端 eval 实测。

### R11. PASS ≠ 绿
来源：`feedback_eval_pass_is_not_green`
- B5 起每批必须人工审最终答案：完整性、richness、图例齐全、无关键因素缺失。
- B7 必须每个 family 至少一例真实 eval 答案原文审查。

### R12. 静态可见 gap 不留实测兜底
来源：`feedback_static_auditable_gaps_first`
- 每批结束前 `rg` 自检红线列表（§10）；命中数变化必须能解释来源。

---

## 4. 当前基线状态（核查后定型）

`origin/main HEAD = e9f271e` 时刻的真实状态（grep 验证）：

| 维度 | 数量 | 备注 |
|---|---|---|
| `AnswerShape` enum 值 | 9（read 7 + write 2） | list_of_symbols / step_list / value / boolean / config_value / explanation / none + change_plan / change_report |
| `doc.Shape` / `RequiredAnswerShape` / `AnswerShape` 引用文件 | 66 | 含测试 |
| 上述符号引用代码行 | 415 | 删旧总规模 |
| `runAnswerShapeOracle` / `resolveAnswerDocShape` / `EffectiveRequiredAnswerShape` / `reconcileShape` 命中文件 | 20 | 主路由消费者 |
| `renderAnswerDoc{ListOfSymbols,StepList,Value,Boolean,ExplanationSkeleton}` 命中 | 12（全在 `render/answerdoc.go`） | shape 分流 renderer |
| V2 carrier 文件 | 0（不存在） | `answer_document_v2.go` / `answer_semantic_view.go` / `answerdoc_v2.go` 都缺失 ✓ |
| 新底座（已落地） | 4 文件 | `claim_form.go` / `facet_plan.go` / `rendered_claim_use.go` / `answer_surface_plan.go` ✓ |
| `AnswerSurfacePlan.RequiredShape` 字段 | 仍存在 | B8 删 |
| `AnswerSurfacePlan.FacetCoverage` 字段 | 已存在 ✓ | 编译完成 |
| `QuestionFamily` 7 值 | 已落地 ✓ | QF{RootCauseTrace,ConfigPrecedence,RoleLookup,CallChain,Enumeration,Architecture,Generic} |

最近 commits 与本计划的耦合：
- `e9f271e`（TypedRelationHint）：通过 prompt 渲染入 Structured Evidence section，与 V1/V2 carrier 正交，B5 V2 接管时无需迁移。
- `4727c6c`（ViolStructuralEnumerationDivergence oracle）：当前读 `doc.Symbols` + `doc.Summary`。**B5 必须改读 V2 blocks**（`AnswerDocumentV2.Blocks[].Items[].Label` + `BlockSummary.Text`）。
- `6ce4ef8`（ViolSymbolAnchorMismatch oracle）：当前读 `doc.Symbols` 长度 + `EnumerationBoundary.DeclaredCount`。**B5 必须改读 V2 blocks**。

---

## 5. 八批 commit 实施计划

### 批次概览

| 批次 | 用户方案 Phase | 主题 | 预估 LOC | 风险 | 默认行为变化 |
|---|---|---|---|---|---|
| **B1** | Phase 0 + 1（part A） | AnswerSemanticView 类型 + 编译器骨架 | +600 | 低 | 无 |
| **B2** | Phase 1（part B） | 6 family BlockRequirement 编译规则 | +700 | 中 | 无 |
| **B3** | Phase 2（part A） | AnswerDocumentV2 类型 + emit_answer_document v2 schema 接入 | +500 | 中 | 无 (V1 默认) |
| **B4** | Phase 2（part B） + Phase 4（part A） | V2 validator 4 件套 + V2 oracle telemetry-only 接入 | +700 | 中 | 无 (telemetry-only) |
| **B5** | Phase 3 | V2 renderer / hedging / ParseOutput 分支 + 既有两 oracle 适配 | +800 | 中 | 无 (V2 路径不默认) |
| **B6** | Phase 4（part B） + Phase 5 + Phase 6 | finalizer prompt 引入 block contract，默认切 V2 | +500 / -200 | **高** | 默认切 V2 |
| **B7** | Phase 6（稳定门） | 6 family eval + V1/V2 一致性比对 + richness audit | +200 (test) | 高 | 同 |
| **B8** | Phase 7 + Step A 全部 | 零容忍删旧 | -2200+ | 高 | V1 完全消失 |

### 5.1 B1 — AnswerSemanticView 类型 + 编译器骨架

**Phase 对应**：用户方案 Phase 0（冻结）+ Phase 1（part A）

#### B1 任务列表

**B1-T1 新增类型文件 `internal/types/answer_block_kind.go`**

定义 `AnswerBlockKind` 闭枚举 + `AllAnswerBlockKinds()` + `SurfaceRole` 枚举：

```go
type AnswerBlockKind string
const (
    BlockSummary     AnswerBlockKind = "summary"
    BlockSection     AnswerBlockKind = "section"
    BlockOrderedList AnswerBlockKind = "ordered_list"
    BlockBulletList  AnswerBlockKind = "bullet_list"
    BlockScalar      AnswerBlockKind = "scalar"
    BlockDecision    AnswerBlockKind = "decision"
    BlockTable       AnswerBlockKind = "table"
    BlockDiagram     AnswerBlockKind = "diagram"
    BlockCaveat      AnswerBlockKind = "caveat"
)

type SurfaceRole string  // 已在 rendered_claim_use.go 定义则 reuse
```

**B1-T2 新增类型文件 `internal/types/answer_semantic_view.go`**

定义：
- `AnswerSemanticView` 主结构
- `BlockRequirement{Kind, MinCount, MaxCount, Required, FacetIDs, AcceptableClaimForms, Rationale}`
- `DiagramFacetGraph{Required, Kind, NodeFacets, EdgeFacets}`
- `UncertaintyRule{TriggerFacet, ExpectedBlockKind, MissingMessage}`
- `RichnessCandidate{Kind, FacetID, Optional}`
- `AllAnswerSemanticViewBlockRequirements()` 用于结构性测试覆盖率检查

**B1-T3 新增编译器骨架 `internal/types/answer_semantic_view_compile.go`**

实现 `BuildAnswerSemanticViewForAgentContext(*AgentContext) *AnswerSemanticView` 与 `BuildAnswerSemanticViewForBusContext(*BusContext) *AnswerSemanticView`。

骨架阶段（B1）只做 placeholder：
- 读 `AnswerSurfacePlan` + `RequestModel.RequiredAnswerShape` + `FacetCoverage`
- 暂时只产出空 `RequiredBlocks` slice + 复用 SurfacePlan 现有字段
- 真正的 family-aware 编译规则在 B2 实现

**B1-T4 单测 `internal/types/answer_semantic_view_test.go`**

最少 4 个测试：
- nil 输入 nil 输出
- 任意 RequiredShape 都能编出非 nil view
- AllAnswerBlockKinds() 全闭枚举
- 编译输出的 FacetCoverage 与输入 SurfacePlan 一致

**B1-T5 trace 日志接入**

在 `BuildAnswerSemanticView*` 末尾 emit `[trace/sv]` debug 行 — 字段：family / required_blocks_count / optional_blocks_count / has_diagram / uncertainty_rules_count / richness_candidates_count。

#### B1 红线核查
- ✅ R1: 不动 prompt / doc 字段
- ✅ R2: 输入全 typed
- ✅ R3: 无关键词
- ✅ R4: 不动 prompt
- ⚠️ R5: 编译规则需说清楚每个 family 怎么映射 — B1 只 placeholder 骨架，B2 才实质化（B2 必填）
- ✅ R6: 无 V2 oracle 上线
- ✅ R7: 无遗留代码
- ✅ R8: 结构性新增
- ✅ R12: grep 红线 #1-#4 命中数应**不变**（B1 不删任何旧符号）

#### B1 完成条件 (DoD)
- `go test ./internal/types/...` 全绿
- `go test ./...` 全绿（B1 不应改动其它包行为）
- `go vet ./...` 干净
- `make` build 干净
- `[trace/sv]` 日志在带 RequestModel 的任意 dispatch 都打出
- 终局 grep 红线（§10）命中数不变（B1 不删旧符号；只增 V2 类型）

#### B1 回滚预案
B1 所有改动是新文件 + 新 trace log，零行为变化。回滚 = `git revert`，无副作用。

---

### 5.2 B2 — 6 Family BlockRequirement 编译规则

**Phase 对应**：用户方案 Phase 1（part B）

#### B2 任务列表

**B2-T1 编译规则文件 `internal/types/answer_semantic_view_compile_<family>.go`**

每个 family 一个文件（避免 mega-file）：
- `compile_root_cause_trace.go` — QFRootCauseTrace
- `compile_config_precedence.go` — QFConfigPrecedence
- `compile_role_lookup.go` — QFRoleLookup
- `compile_call_chain.go` — QFCallChain
- `compile_enumeration.go` — QFEnumeration
- `compile_architecture.go` — QFArchitecture
- `compile_generic.go` — QFGeneric

每个 family 的 compile 函数返回 `[]BlockRequirement` + `[]BlockRequirement`(optional) + `*DiagramFacetGraph` + `[]UncertaintyRule` + `[]RichnessCandidate`，全部基于 typed 输入：

| Family | Required Blocks（示例，B2 实质化时根据 FacetCoverage 表精确填） |
|---|---|
| QFRootCauseTrace | BlockSummary(1) + BlockOrderedList(1, principal cause chain) + BlockDiagram(1, optional) + BlockCaveat(0..N, drift markers) |
| QFConfigPrecedence | BlockSummary(1) + BlockOrderedList(1, precedence layers) + BlockTable(0..1, layer-by-key) + BlockCaveat(0..N) |
| QFRoleLookup | BlockSummary(1) + BlockScalar(1) + BlockSection(0..1, supporting context) |
| QFCallChain | BlockSummary(1) + BlockOrderedList(1, hops) + BlockDiagram(1, sequence) |
| QFEnumeration | BlockSummary(1) + BlockOrderedList or BlockTable (1, members) + BlockCaveat(0..N) |
| QFArchitecture | BlockSummary(1) + BlockSection(1+, layers) + BlockDiagram(1, flowchart) |
| QFGeneric | BlockSummary(1) + 其他 optional |

**核心约束**：
- MinCount/MaxCount 不写"最多 3 层"等假设；MaxCount=0 表示无上限
- FacetIDs 引用 `FacetCoverage.Required[i].Kind`
- AcceptableClaimForms 引用 `ClaimForm` 已有枚举值
- Rationale 是 LLM-facing 说明（B6 prompt 渲染时用）

**B2-T2 修改 `BuildAnswerSemanticViewFor*`**

把 B1 的骨架替换为：

```go
func BuildAnswerSemanticView(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
    family := ResolveQuestionFamily(ir.RequestModel)
    switch family {
    case QFRootCauseTrace: return compileRootCauseTrace(ir, plan)
    // ...
    case QFGeneric: return compileGeneric(ir, plan)
    }
}
```

**B2-T3 单测 `internal/types/answer_semantic_view_compile_<family>_test.go`**

每个 family 至少 3 测试：
- 最小输入产出非空 view
- RequiredBlocks 中 Summary block 必含 Required=true + MinCount=1
- 涉及 diagram 的 family（CallChain / Architecture）必有 DiagramFacetGraph

总计 21 测试。

**B2-T4 添加 family-completeness 结构性测试 `answer_semantic_view_coverage_test.go`**

`TestAllQuestionFamiliesHaveCompiler` — 遍历 `AllQuestionFamilies()`（如已存在则用；否则新增），每个都要 `BuildAnswerSemanticView` 不返回 nil。

#### B2 红线核查
- ✅ R1: 不动答案字段
- ✅ R2: 输入全 typed (RequestModel.Predicates / Intent / AnswerSubject)
- ✅ R3: **严格自查**：编译规则**不允许**含字符串关键词表（"if subject contains 'config' then..."）；只读 typed enum
- ✅ R4: 不动 prompt
- ⚠️ R5: 5-Q 审 — 7 family 是否都按统一抽象编（不能 6 个 family 用一个套路、QFGeneric 用另一个套路特例化）
- ✅ R6: 仍无 V2 oracle 上线
- ✅ R7-R12: 同 B1

#### B2 完成条件 (DoD)
- 所有 7 family 都有非空 BlockRequirement 编译输出
- 21 单测全绿
- `[trace/sv]` 日志在每个 family 都能产生有意义内容（required_blocks_count > 0）
- 终局 grep 红线命中数不变

#### B2 回滚预案
回滚 = `git revert`，零行为影响。

---

### 5.3 B3 — AnswerDocumentV2 类型 + emit_answer_document v2 schema

**Phase 对应**：用户方案 Phase 2（part A）

#### B3 任务列表

**B3-T1 新增类型 `internal/types/answer_document_v2.go`**

定义 `AnswerDocumentV2` 主结构 + `AnswerBlock` + `AnswerBlockItem` + `AnswerDiagramBlock`，按用户方案 §4.2 的 schema 落地。**不**复用 V1 的任何 helper。

**B3-T2 修改 `internal/types/context.go::MutableState`**

新增字段（**不替换** V1）：

```go
answerDocumentV2 *AnswerDocumentV2
```

加 accessor：
- `MutableState.AnswerDocumentV2() *AnswerDocumentV2`
- `MutableState.SetAnswerDocumentV2(*AnswerDocumentV2)`
- `MutableState.ResetAnswerDocumentV2()`

**B3-T3 修改 `internal/tool/emit_answer_document.go`**

schema 入口加 `document_model` 字段：

```json
"document_model": {"type": "string", "enum": ["", "v2"], "description": "..."}
```

Execute 路由：
- `document_model == ""` → 走 V1 解析（不变）
- `document_model == "v2"` → 走新解析路径，写 `Mutable.SetAnswerDocumentV2`
- 其他值 → reject with clear error

**B3-T4 V2 schema 子结构定义**

V2 schema 必须显式拒绝 V1 字段（顶层 `shape` / `steps` / `summary` 等）。schema 描述用 LLM-facing 语言（R4 红线）。

**B3-T5 单测 `internal/tool/emit_answer_document_v2_test.go`**

最少 8 测试：
- 无 document_model = V1 兼容（byte-identical）
- document_model="" = V1 兼容
- document_model="v2" + 合法 blocks = 接受
- document_model="v2" + V1 字段（shape）= 拒绝
- document_model="v3" = 拒绝
- V2 块缺 ID = 拒绝
- V2 块未知 Kind = 拒绝
- V2 写入 Mutable.AnswerDocumentV2 不污染 V1.AnswerDocument

**B3-T6 finalize_preview 适配 `internal/agent/finalize_preview.go`**

partial salvage 路径需识别 `document_model="v2"`：当流式截断时尝试解出已有 blocks，挂到 V2 字段。V1 路径不变。

#### B3 红线核查
- ✅ R1: tool emit 是 LLM 写入，不是系统 backfill
- ✅ R2: schema 校验全 typed
- ✅ R3: 无关键词
- ✅ R4: schema description 用 LLM-facing 语言（"answer block kind enum"）
- ⚠️ R5: V2 schema validator 必须能 articulate 一句话 — "确保 LLM emit 的 blocks[] 满足 schema"
- ✅ R6: V2 不默认；V1 路径完全不变
- ✅ R7: V2 解析器是新代码，无遗留

#### B3 完成条件 (DoD)
- V2 emit 路径能完整 round-trip 一个最小 AnswerDocumentV2（summary block + 1 ordered list）
- V1 emit 路径**byte-identical** 与 B2 之前（字节对比 evidence_grounder 的 schema 输出）
- 8 单测全绿
- `MutableState.AnswerDocument()` 与 `MutableState.AnswerDocumentV2()` 不会同时非 nil（emit 互斥）

#### B3 回滚预案
回滚 = revert 整批；V1 路径不变所以无外部影响。

---

### 5.4 B4 — V2 validator 4 件套 + V2 oracle 接入（telemetry-only）

**Phase 对应**：用户方案 Phase 2（part B）+ Phase 4（part A）

#### B4 任务列表

**B4-T1 新增 `internal/orchestrator/contract_check_block.go`**

实现 4 个 V2 validator（用户方案 §5 Phase 4 的清单）：

1. `validateRequiredBlockCoverage(doc *AnswerDocumentV2, view *AnswerSemanticView) []Violation`
2. `validatePrincipalClaimUse(doc *AnswerDocumentV2, view *AnswerSemanticView) []Violation`
3. `validateDiagramEdgeSupport(doc *AnswerDocumentV2, view *AnswerSemanticView) []Violation`
4. `validateUncertaintyBlockPresence(doc *AnswerDocumentV2, view *AnswerSemanticView) []Violation`

**B4-T2 引入 4 个新 ViolationKind（只增、不删）**

```go
ViolBlockCoverageMissing    ViolationKind = "block_coverage_missing"
ViolPrincipalClaimUseMissing ViolationKind = "principal_claim_use_missing"
ViolDiagramEdgeUnsupported  ViolationKind = "diagram_edge_unsupported"
ViolUncertaintyBlockMissing ViolationKind = "uncertainty_block_missing"
```

注册进 `AllViolationKinds()` + `cgec_completeness_test.go::covered/kindSymbols` 双表。

**B4-T3 V2 oracle 接入 contract_check.go runContractCheck**

```go
if mut != nil {
    if docV2 := mut.AnswerDocumentV2(); docV2 != nil {
        view := types.BuildAnswerSemanticView(...)
        result.Violations = append(result.Violations,
            validateRequiredBlockCoverage(docV2, view)...)
        // ... 其他 3 个
    }
}
```

**关键**：4 个新 ViolationKind 默认 SOFT (telemetry-only)。绝不影响 `Result.Passed`。

**B4-T4 fallback policy 注册**

`fallback_policy.go::DefaultFallbackPolicy` 给 4 个新 kind 都映射到 `FallbackFinalizerOnly`（telemetry 期不触发 retry）。当 V1 oracle 退场时改为合适的 fallback target。

**B4-T5 单测每个 validator 至少 4 case**

总 16 测试 + 1 个 AllViolationKinds 完整性测试。

#### B4 红线核查
- ✅ R1: validator 不写答案
- ✅ R2: 4 个 validator 输入全 typed
- ✅ R3: 无关键词
- ✅ R4: 不动 prompt
- ⚠️ R5: 每个 validator 必须有 1-句失败模式说明
- ✅ R6: V2 telemetry-only；V1 oracle 仍硬执行；不打架
- ✅ R12: grep 红线命中数应**仍不变**（V1 oracle 没动）

#### B4 完成条件 (DoD)
- 4 个 validator + 16 单测全绿
- 给 emit V1 的请求跑 contract_check，4 个新 kind 不会出现（V2 doc nil）
- 给 emit V2 的请求跑 contract_check，4 个新 kind 各自能在合适 case 触发
- closure ledger 能正确接住新 kind（per-stage stats 正确归类到 finalize）

#### B4 回滚预案
revert 整批；V1 oracle 路径未动。

---

### 5.5 B5 — V2 renderer + ApplyAuthorityHedgingV2 + 既有两 oracle 适配

**Phase 对应**：用户方案 Phase 3

#### B5 任务列表

**B5-T1 新增 `internal/render/answerdoc_v2.go`**

`RenderAnswerDocumentV2(*AnswerDocumentV2) string` — block-by-block 渲染，无 shape 分流。每个 BlockKind 一个内部 helper。Diagram 块用 mermaid renderer（既有 `internal/render/mermaid_render.go`）。

**B5-T2 新增 `internal/render/apply_authority_hedging_v2.go`**

`ApplyAuthorityHedgingV2(*AnswerDocumentV2, ...) *AnswerDocumentV2` — 按 block role + claim_uses + citation authority 处理：
- caveat block 不触动
- principal block + claim_form 衰减 → 加 caveat block 而非改写正文
- 不复用 V1 的 hedgeSteps/hedgeSymbols/hedgeSummary/hedgeBoolean

**B5-T3 修改 `internal/agent/answer_document_evaluator.go::ParseOutput`**

```go
docV2 := mut.AnswerDocumentV2()
if docV2 != nil {
    docV2 = ApplyAuthorityHedgingV2(docV2, ...)
    return RenderAnswerDocumentV2(docV2), ...
}
// V1 fallback unchanged
return RenderAnswerDocument(mut.AnswerDocument()), ...
```

**B5-T4 适配 `runStructuralEnumerationDivergenceOracle`（commit 4727c6c）**

当 `AnswerDocumentV2` 非 nil：从 V2 blocks 提取等价的 emitted-name set + summary text：
- emitted-name set = `BlockOrderedList` / `BlockBulletList` / `BlockTable` 中 items[].Label union
- summary text = 任意 BlockSummary.Text 拼接

V1 doc 路径保持不变。

**B5-T5 适配 `runSymbolAnchorTrackOracle`（commit 6ce4ef8）**

`gotSymbols` 从 V2 blocks 计数：BlockOrderedList 等列表项数 OR 等价 anchor 数。`requiredShape` 适配为 `view.RequiredBlocks` 主类型（list/scalar/explanation）映射。

**B5-T6 适配 TypedRelationHint 渲染**

确认 `formatEvidenceItemsWithOptions` 渲染的 Structured Evidence section 仍被 V2 prompt 接收（由 B6 prompt builder 保证）。B5 只确保 prompt 路径不变。

**B5-T7 V1/V2 一致性 fixture 测试**

`internal/render/answerdoc_v1v2_consistency_test.go`：
- 每个 family 一个最小 fixture
- 同一语义内容分别 emit V1 和 V2
- 渲染后做语义比对（不要求字节相同，要求 user-visible 信息等价）

**B5-T8 单测 `internal/render/answerdoc_v2_test.go`**

每个 BlockKind 至少 1 渲染快照 + 1 边界 case（空 items / 缺 ID / 跨语言 Title）。

#### B5 红线核查
- ✅ R1: renderer 是 LLM-emit-after 阶段；不写新内容到 doc
- ✅ R2: oracle 适配读 typed V2 字段
- ⚠️ R6: V2 路径**仍非默认**（emit 端 B6 才默认 V2）；当前 V2 渲染只在 LLM 主动 emit V2 时触发；V1 与 V2 oracle 仍并行 telemetry
- ✅ R7: 不留 V1/V2 桥接 stub
- ⚠️ R11: 必须人工审 V1/V2 一致性 fixture 渲染原文，不只看 PASS

#### B5 完成条件 (DoD)
- V2 renderer 能正确渲染 6 family 各一最小 fixture
- V1/V2 一致性测试 6 family 全绿
- B4 的 4 个 V2 oracle 在 V2 doc 上能按预期 raise（仍 telemetry）
- 既有 ViolStructuralEnumerationDivergence + ViolSymbolAnchorMismatch 在 V1/V2 doc 上行为一致
- `go test ./...` + `make` + `go vet` 全清

#### B5 回滚预案
V2 renderer 是新文件；ParseOutput 内的 if docV2 != nil 分支可一行 revert。

---

### 5.6 B6 — Prompt 章节切换 + 默认切 V2

**Phase 对应**：用户方案 Phase 4（part B）+ Phase 5 + Phase 6
**风险**：高 — 这是行为变化批

#### B6 任务列表

**B6-T1 修改 `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` + Knowledge & Evidence Pool 重命名**

新增章节（**用 LLM-facing 语言**，遵 R4 红线）：
- `## Required Answer Blocks` — 列 view.RequiredBlocks 的 Kind / MinCount / Title 提示
- `## Facets each block must cover` — 列 view 提供的 facet 信息

**保留** `## Target answer shape` 章节作为过渡（**B8 删**）。两章节并存期由 yaml 旋钮 + CLI flag 控制（见 B6-T2）。

**Sub-task: Knowledge & Evidence Pool section 重命名**

`internal/context/builder.go` 中 `Title: "Structured Evidence"` 改为 `Title: "Knowledge & Evidence Pool"`，并在 section 内顶端加一段 LLM-facing 解释（约 2-3 行）：

> The pool unifies LLM-emitted evidence (provenance=llm_evidence) with system-derived
> typed-graph relations (provenance=typed_graph). Both are authoritative grounding
> sources for citing in your answer blocks; the Provenance column tells you where
> each (subject, object, file:line) tuple came from.

理由（决议见 §12 第 5 项）：
- "Structured Evidence" 是 V1 时代术语，隐含"LLM-emit 的 EvidenceItem"；V2 时代 typed-graph 行也是合法证据。
- 中性概念脱离 V1，未来加新 typed 通道（typed dataflow / typed import-graph / typed call-tree 等）零成本，只需新增 Provenance 值（`typed_dataflow` 等），不必再改 section 名或拆段。
- R4 兼容：所有 prompt-pin 测试需把 `Structured Evidence` 从 expected 列表换成 `Knowledge & Evidence Pool`。
- 影响范围：`builder.go` + `builder_test.go` + 任何 grep `Structured Evidence` 命中的 prompt-pin 测试。

预估：~30 LOC + ~6 测试更新。

**B6-T2 新增 yaml 旋钮 `pipeline_emit_v2_default` + CLI flag `--emit-v2`**

`internal/config/runtime.go` 加：

```go
type PipelineSettings struct {
    ...
    EmitV2Default *bool `yaml:"pipeline_emit_v2_default"` // default true
}
```

`cmd/root.go` 加 CLI flag：

```go
emitV2Mode := rootCmd.PersistentFlags().String("emit-v2", "auto",
    "V2 carrier emission mode: auto (default; follow yaml pipeline_emit_v2_default), on (force V2), off (force V1)")
```

resolution order（最高优先级在前）：
1. `--emit-v2=on` → 强制 V2（无视 yaml）
2. `--emit-v2=off` → 强制 V1（无视 yaml）
3. `--emit-v2=auto`（默认）→ 读 yaml `pipeline_emit_v2_default`（默认 true）

`emit_answer_document.go` 在 schema 默认值上读 resolved 模式：V2 时 prompt 暗示 LLM emit V2；V1 时仍 V1。

CLI flag 的存在理由：B6-B8 测试期需要快速 per-run 切换便于 A/B 比对、回归验证、灰度试探，不必重启 yaml 配置。`auto` 模式确保未指定时跟随生产 yaml。

启动时 INFO log 打出 resolved 模式（`[cmd] emit_v2_mode=on/off (source=cli|yaml)`）便于排查。

**B6-T3 V1 oracle 降级 telemetry-only**

`runAnswerShapeOracle` 在 V2 路径触发时降为 telemetry：
- 仍 raise violation 进 closure ledger
- `Result.Passed` 不变（不 reject）
- 加 yaml 旋钮 `pipeline_v1_oracle_strict_mode`（default false）让操作员能临时升级回 strict（应急回退）

**B6-T4 V2 oracle 升级 strict**

B4 的 4 个 ViolationKind 在默认 SOFT/STRICT 表中升级到 STRICT（fallback target 也启用）。

**B6-T5 端到端 6 family eval**

每个 family 跑 1 case，要求：
- V2 默认下 PASS
- 答案原文人工审：完整、richness 不退化、claim use / facet 标注正确

**B6-T6 灰度回滚说明**

文档（本文档 §8）记录：当生产发现 V2 退化时，操作员设 `pipeline_emit_v2_default: false` + `pipeline_v1_oracle_strict_mode: true`，恢复 V1 主路径。

#### B6 红线核查
- ⚠️ R4: prompt 新章节必须 grep 自查无内部术语
- ⚠️ R5: 默认切 V2 + 灰度旋钮 — 单一改动责任：prompt 通知 LLM 用 V2，V1 oracle 退到 telemetry
- ⚠️ R6: V2 strict + V1 telemetry — 唯一允许的"短期降级"状态，必须有明确的 B8 删除时间
- ⚠️ R10: 删 prompt 章节前三步走（本批暂未删 `## Target answer shape`，B8 删）
- ⚠️ R11: PASS ≠ 绿 — 每个 family 必须人工审原文

#### B6 完成条件 (DoD)
- 6 family eval 全 PASS + 人工审通过
- V2 默认输出，V1 路径仍可通过 yaml 回滚
- V1 oracle 不再 reject（telemetry only）
- V2 oracle 已 strict
- `go test ./...` + `go vet` + `make` 全清

#### B6 回滚预案
- yaml 旋钮 `pipeline_emit_v2_default: false` 一键回 V1
- 极端情况 `git revert` B6 commit；V2 代码仍在但默认不启用

---

### 5.7 B7 — 6 family 稳定门 + V1/V2 一致性比对

**Phase 对应**：用户方案 Phase 6（稳定门）

#### B7 任务列表

**B7-T1 6 family 各一稳定 case 全跑**

case 选择：
- QFRootCauseTrace: `logtri_go.case` 或 `logtri_python.case`
- QFConfigPrecedence: 创建 `qf_config_precedence.case`（如不存在）
- QFRoleLookup: `s11a.case` 或新建
- QFCallChain: `m1a.case` / `m2a.case`
- QFEnumeration: `s5a.case` ✓
- QFArchitecture: `m1b.case` 或新建
- QFGeneric: `s1a.case` 或选合适 case

**B7-T2 V1/V2 一致性 telemetry 接入**

每次 dispatch 双 emit（V1 schema + V2 schema），系统比对 user-visible 输出的语义等价性，记录差异到 `[trace/v1v2_diff]` debug 日志。

**B7-T3 richness audit**

每个 family 至少一例答案原文人工审。审计表（本文档 §11 模板）：

| Family | Case | V1 richness 维度 | V2 richness 维度 | Δ |
|---|---|---|---|---|
| QFRootCauseTrace | logtri_go | ... | ... | ... |

**B7-T4 稳定运行 24 小时**

V2 默认 + V1 telemetry 模式生产运行 ≥ 24 小时无退化（user-reported）。

#### B7 红线核查
- ⚠️ R5: 7 family 通用 — 任何 family 出现需要特例化的 fix 都视为 V2 抽象不足，需回 B2 修编译规则
- ⚠️ R11: 不能只看 substring PASS

#### B7 完成条件 (DoD)
- 6 family 全 PASS + richness 不退化
- 一致性差异都有 ticketed 解释
- 24h 稳定 + 无 user-reported regression

---

### 5.8 B8 — 零容忍删旧

**Phase 对应**：用户方案 Phase 7 + Step A

**风险**：高，但收益是终结双 authority。

**前置硬条件**（用户方案 §9.7 全满足）：
1. ✅ B6 V2 默认稳定 ≥ 1 轮（B7 完成）
2. ✅ 所有 family 都有 V2 live case（B7-T1 完成）
3. ✅ reviewer / hedging / render / contract check 都不读 `doc.Shape`（手工核查 + grep）
4. ✅ AnswerShape 在 read-mode 无真实消费点（grep 自检）
5. ⚠️ Step A 在 B8 内完成

#### B8 任务列表（**严格按以下顺序**，违反顺序会引入编译错误链）

**B8-T0 [前置] write-mode shape 拆出 WriteOutputKind**

新增 `internal/types/write_output_kind.go`：

```go
type WriteOutputKind string
const (
    WriteOutputChangePlan   WriteOutputKind = "change_plan"
    WriteOutputChangeReport WriteOutputKind = "change_report"
)
```

迁移所有 write-mode 引用 `ShapeChangePlan` / `ShapeChangeReport` 的代码使用 `WriteOutputKind`。

预估：~150 LOC 改动，~10 文件。

**B8-T1 删 V1 oracle 与 helper（read-mode 主链消费者）**

按用户方案 §6.6 + §6.5 + §6.3：

文件级删除清单：
- `internal/orchestrator/contract_check.go::runAnswerShapeOracle` 函数 + dispatch
- `internal/orchestrator/contract_check.go::shapeTextForContractCheck`
- `internal/orchestrator/answer_shape_oracle_test.go` 整文件
- `internal/orchestrator/answer_shape_oracle_repair_test.go` 整文件
- `internal/agent/analyzer.go::reconcileShape` 函数 + 所有调用点
- `internal/agent/extractor.go::isListOfSymbolsShape` / `isBoundedPrincipalStepList` / `needsAnswerSymbols` / `requiredAnswerSymbolCount`
- `internal/agent/answer_document_evaluator.go::resolveAnswerDocShape`
- `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` 中 `## Target answer shape` 章节（R10：grep + 实测确认无遗留消费者）

预估：~600 LOC 删除。

**B8-T2 删 V1 renderer + hedging（用户方案 §6.5）**

文件级：
- `internal/render/answerdoc.go` 中 5 个 `renderAnswerDoc<Shape>` + V1 dispatch；保留 `RenderAnswerDocumentV2` 改名为 `RenderAnswerDocument`
- `internal/render/apply_authority_hedging.go` 整文件删；`apply_authority_hedging_v2.go` 改名为 `apply_authority_hedging.go`（除非 V2 文件名留个 v2 标记，否则文件 rename + import path 调整）

测试同步：
- `internal/render/answerdoc_test.go` 中 shape-specific fixture 整体迁到 V2 等价 fixture（保留人工审查通过的语义对照）；shape-only 测试删
- `internal/render/apply_authority_hedging_shape_test.go` 整文件删

预估：~700 LOC 删除 + ~200 LOC 测试迁移。

**B8-T3 删 emit_answer_document V1 schema 路径（用户方案 §6.4）**

`internal/tool/emit_answer_document.go`：
- 删顶层 `shape` 字段 + 解析
- 删 V1 互斥逻辑（shape=X requires Y / shape=X forbids Y）
- 删 `validateValueShapeSummary` + shape swap + ShapeSwapStrictMode
- 删 V1 schema fixture（`emit_answer_document_v1_test.go`）
- `document_model` 字段从 `enum: ["", "v2"]` 简化为：直接 v2-only schema（v1 不再支持）

`internal/agent/finalize_preview.go` V1 partial salvage 路径删除。

预估：~500 LOC 删除。

**B8-T4 删 V1 carrier 类型（用户方案 §6.1）**

文件级：
- `internal/types/answer_document.go::AnswerDocument` 整 struct 删
- `internal/types/answer_document.go::AnswerStep` / `AnswerStepKind` / `AnswerValue` / `AnswerBoolean` 删
- `internal/types/answer_document.go::PrincipalStepCount` / `CountStepsOfKind` 删
- `internal/types/context.go::MutableState.answerDocument` + `AnswerDocument()` / `SetAnswerDocument()` / `ResetAnswerDocument()` 删；`AnswerDocumentV2` 改名为 `AnswerDocument`，对应字段重命名

注：`AnswerSymbol` 是 emit_answer_symbol 的产物，**保留**（与 V1 carrier 是两层概念，B8 不动）。

预估：~400 LOC 删除。

**B8-T5 删 AnswerShape enum + RequiredAnswerShape 字段（用户方案 §6.1 + §6.2 + §6.3）**

前置：B8-T0 完成（write-mode 已 WriteOutputKind 拆出）。

文件级：
- `internal/types/analysis_ir.go::AnswerShape` type 整删
- `internal/types/analysis_ir.go::ShapeXxx` const 整删（read-mode 7 个值）
- `internal/types/analysis_ir.go::AnswerContract.RequiredAnswerShape` 字段删
- `internal/types/answer_shape_runtime.go` 整文件删（`EffectiveRequiredAnswerShape` / `StableAbsentExactConfigRequiresExplanation` / `ExplanationAllowsAnchorSkeleton` 中 shape 依赖移到 `AnswerSemanticView`）
- `internal/types/answer_surface_plan.go::AnswerSurfacePlan.RequiredShape` 字段删；所有 `plan.RequiredShape` 引用替换为 `view.PrimaryBlockKind`（新方法）

预估：~500 LOC 删除。

**B8-T6 删 V1 测试 + 修文档（用户方案 §6.7）**

文件级：
- `internal/types/answer_shape_runtime_test.go` 整删
- `internal/orchestrator/self_consistency_gate_test.go` 中 shape gating 假设删
- `internal/types/cgec_completeness_test.go` 中 shape-related 整理
- `docs/architecture.md` 中 read-mode final answer 描述章节改写（按 §10.1 模板）
- 旧迁移文档归档到 `docs/archive/`

预估：~300 LOC 删除 + ~150 LOC 文档改写。

**B8-T7 终局 grep 归零自检 + commit**

执行 §10 全部 grep 命令，确认每条都返回 0 命中（除允许残留位置）。

如有残留：**不允许"先留着"**。要么纳入本批，要么取消 B8 重做。

#### B8 红线核查
- ⚠️ R6: 删旧前 V2 必须默认稳定（B7 完成是硬前置）
- ⚠️ R7: 严禁"先注释掉" — 所有删除都是 git delete，不留 dead code
- ⚠️ R8: 删除是结构性的，不允许 stub 桥（`func reconcileShape(...) {}` 空函数也不允许）
- ⚠️ R10: prompt 章节删除三步走 — grep canonical / 逐 agent 消费者 / 端到端 eval
- ⚠️ R12: 终局 grep §10 必须每条 0 命中

#### B8 完成条件 (DoD)
- §10 终局 grep 列表全 0 命中
- `go test ./...` + `go vet ./...` + `make` 全清
- 6 family eval 全 PASS（V2 已是唯一路径）
- 答案 richness 与 B7 baseline 等价
- 删除 LOC 总数 ≥ 2000
- `git diff origin/main HEAD --stat` 显示净 LOC 减少

#### B8 回滚预案
B8 是非破坏性回滚困难批 — V1 已不存在，单 commit revert 不可能恢复 V1 行为。
应急方案：保留 B8 之前的 commit hash，紧急时整批 revert（需要预先把 B8 拆成 7 个独立 commit，每个失败可单独 revert）。

---

## 6. 跨批次依赖关系

```
B1 ─→ B2 ─→ B3 ─→ B4 ─→ B5 ─→ B6 ─→ B7 ─→ B8
                                                  ↑
                                             硬前置：B7 全 PASS
                                             硬前置：B6 V2 默认稳定 ≥ 24h
                                             硬前置：B8-T0（WriteOutputKind 拆分）
```

每批必须等前批完整 ship + push 到 origin/main + 24h 内零 user-reported regression 才能开始下一批。

例外：
- B1 / B2 可并行起草（B2 等 B1 类型定型后落 LOC）
- B3 / B4 串行（B4 测试需要 B3 的 V2 doc 结构）
- B7 stabilization 不能跳

## 7. 测试策略

### 7.1 单测覆盖率

每批新增类型 / 函数必须有 ≥ 80% 行覆盖率（go test -cover）。

### 7.2 V1/V2 并行一致性测试（B5-B7 阶段必须）

`internal/render/answerdoc_v1v2_consistency_test.go` 持续维护。每发现 V1/V2 不等价的边界 case 都要加新 fixture。

### 7.3 6 family eval（B6-B7 必须）

每批合入主线前跑：
- B6 / B7：6 family + s5a + s1a + m1a 至少 9 case
- B8：6 family 必须全绿

### 7.4 Prompt 审计自动化

新增 `internal/skill/prompt_internal_terms_test.go`（或扩展现有），grep prompt 输出禁词列表：
```go
forbidden := []string{
    "FacetCoverageContract", "AnswerSemanticView", "BlockRequirement",
    "RenderedClaimUse", "QuestionFamily", "MutableState", "StageOutput",
    "AnswerSurfacePlan", "DiagramFacetGraph",
}
```

测试在 B6 起永久开启。

## 8. 灰度回滚机制

| 场景 | 操作 | 影响 |
|---|---|---|
| B6 默认切 V2 后 LLM emit V2 出错 | yaml `pipeline_emit_v2_default: false` 或 CLI `--emit-v2=off` | 立刻回 V1 prompt + V1 emit 路径 |
| B6-B7 测试期需要 per-run 切换比对 | CLI `--emit-v2={on,off,auto}` | 单 run 切换；不影响其他并发 run |
| V1 oracle 误降为 telemetry 漏过应 reject 的答案 | yaml `pipeline_v1_oracle_strict_mode: true` | V1 oracle 重新 strict（短期应急） |
| B7 某 family 退化 | 整批 revert + 回 B6 灰度状态 | V2 仍代码在；默认关 |
| B8 某删除引入 bug | 立刻整 commit revert（不靠拆） | V1 已经不存在；revert 可恢复结构 |

灰度旋钮在 B8 完成后**必须**移除（用户方案 §6.0：兼容桥必须有删除条件）。具体清理：B8-T7 内删除 `--emit-v2` CLI flag、`pipeline_emit_v2_default` yaml key、`pipeline_v1_oracle_strict_mode` yaml key 全部 3 项。

## 9. 通过门 / 进度跟踪表

每批完成后填表：

| 批次 | 起始 commit | 结束 commit | LOC 变化 | 单测数 | eval cases | 灰度状态 | 用户审查 |
|---|---|---|---|---|---|---|---|
| B1 | | | | | | | |
| B2 | | | | | | | |
| B3 | | | | | | | |
| B4 | | | | | | | |
| B5 | | | | | | | |
| B6 | | | | | | | |
| B7 | | | | | | | |
| B8 | | | | | | | |

总目标：净 LOC ~ -1300（V1 删除 2000+ 减 V2 新增 700）。

## 10. 终局 grep 归零清单（B8 完成判定的硬指标）

执行以下 grep。**每条都必须返回 0 命中**（除允许残留位置：`docs/archive/*` / `docs/migration/*`（本计划文档自身）/ write-mode 注释 / 测试 fixture 命名带 `_legacy` / `_compat`）。

```bash
# read-mode shape 路由完全消失
rg -t go "Target answer shape|resolveAnswerDocShape|EffectiveRequiredAnswerShape|reconcileShape|runAnswerShapeOracle"

# shape literal 字符串完全消失（read-mode）
rg -t go "shape=list_of_symbols|shape=step_list|shape=value|shape=config_value|shape=boolean|shape=explanation"

# shape-specific renderer 完全消失
rg -t go "renderAnswerDocListOfSymbols|renderAnswerDocStepList|renderAnswerDocValue|renderAnswerDocBoolean|renderAnswerDocExplanationSkeleton"

# AnswerStep helpers 完全消失
rg -t go "PrincipalStepCount|CountStepsOfKind|AnswerStepKind"

# AnswerShape type 完全消失
rg -t go "type AnswerShape|AnswerShape ="

# RequiredAnswerShape 字段完全消失
rg -t go "RequiredAnswerShape\b"

# extractor V1 helpers 完全消失
rg -t go "isListOfSymbolsShape|isBoundedPrincipalStepList|needsAnswerSymbols|requiredAnswerSymbolCount"

# V1 hedging helpers 完全消失
rg -t go "hedgeSteps\b|hedgeSymbols\b|hedgeSummary\b|hedgeBoolean\b"

# V1 carrier 字段完全消失
rg -t go "AnswerDocument\.Shape|AnswerDocument\.Steps|AnswerDocument\.Symbols|AnswerDocument\.Boolean|AnswerDocument\.Value|AnswerDocument\.SymbolsCompleteness"

# answer_shape_runtime 文件完全消失
rg -t go "answer_shape_runtime|StableAbsentExactConfigRequiresExplanation"
```

允许残留唯一位置：
- 本文档（`docs/migration/block_only_carrier.md`）
- 已归档的旧文档（`docs/archive/*.md`，需带"已废弃于 B8"标记）
- write-mode 不变的 `change_plan` / `change_report` 注释（用 WriteOutputKind 而非 AnswerShape）
- 测试 fixture 路径含 `_legacy` 或 `_compat` 字样且带删除日期

## 11. Richness 审计模板（B7 强制）

每个 family 一份。审计人填表：

```
Family: <QF*>
Case: <case file>
Run timestamp: <YYYY-MM-DD HH:MM:SS>

V1 richness:
- Summary 长度: <chars>
- 主结构 block 数 / item 数: <count>
- 引用 citation 数: <count>
- diagram: <yes/no, complexity>
- caveat 数: <count>
- 主观完整度评分 (1-5): <score>

V2 richness:
- 同上字段对比

Δ评估:
- 是否 V2 失任何 V1 已覆盖的关键事实: <yes/no, 详细>
- 是否 V2 引入任何 V1 没有的新错误: <yes/no, 详细>
- 是否 prompt-perceptible 顺序差异: <yes/no>

判定: <PASS / NEEDS_FIX / BLOCK_B8>
```

## 12. 开放问题 / 假设（2026-05-03 已全部决议）

1. **B6 时 V2 default 是否需要 CLI flag 而非仅 yaml？** — **决议：加 CLI flag `--emit-v2={auto,on,off}`**（默认 auto = 读 yaml）。理由：B6-B8 测试期需要 per-run 快速切换 A/B 比对，不必重启 yaml；详见 §5.6 B6-T2 + §8 灰度回滚表。B8-T7 内 CLI flag 与 yaml key 一并删除。
2. **B8-T2 hedging 文件是否 rename？** — **决议：同意**。`apply_authority_hedging_v2.go` 在 V1 删完后 rename 为 `apply_authority_hedging.go`，go import path 不变。
3. **B7 24h 稳定运行的判定标准？** — **决议：同意**。telemetry 自动指标（每 case PASS rate）+ 用户回报；无 user-reported regression 即视为绿。
4. **MutableState V1+V2 字段共存期是否需要互斥锁？** — **决议：不加锁**。当前设计 emit 互斥（同 dispatch 只产一份）；B3-T5 单测 case 8 显式验证（`V2 写入 Mutable.AnswerDocumentV2 不污染 V1.AnswerDocument`）。
5. **TypedRelationHint 在 V2 prompt 中是否变形？** — **决议：B5 不变 + B6 重命名 section**。B5 仍走 `## Structured Evidence`；B6-T1 子任务把 section title 改为 `## Knowledge & Evidence Pool`（中性概念，不暴露 Go 类名，对未来 typed dataflow / typed import-graph / typed call-tree 等新通道零成本扩展），保留单一 section + Provenance 列。R4（不暴露内部术语）兼容；prompt-pin 测试同步加 forbidden 词条。

详见 §5.6 B6 部分。

## 13. 任务时间表预估

| 批次 | 工作量预估 | 推荐 session 拆分 |
|---|---|---|
| B1 | 0.5-1 天 | 1 session |
| B2 | 1-2 天 | 1-2 session（每 family 半小时） |
| B3 | 1 天 | 1 session |
| B4 | 1-2 天 | 1-2 session（每 validator 半天） |
| B5 | 2-3 天 | 2-3 session |
| B6 | 1-2 天 | 2 session（prompt 编写 + eval） |
| B7 | 2-3 天（含稳定运行） | 1 session 主体 + 24h 等待 |
| B8 | 2-3 天 | 2-3 session（每子任务 T0-T7 一段） |

总计 10-17 天连续工作 + 24h 稳定门。允许串行不允许并行（除 B1+B2 起草）。

## 14. 一句话方针

**这次重构不是修旧 shape 的边角问题，是按编号顺序拆掉旧世界的 8 块关键拼图，最后一块（B8）必须把所有 V1 痕迹归零，没有保留区，没有兼容桥，没有"以后再说"。**
