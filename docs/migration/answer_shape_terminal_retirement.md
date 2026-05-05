> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# AnswerShape 彻底退役主文件施工清单（终局版）

> **归档元信息**
> - 状态：**SHIPPED** — PR1–PR6 已全部落地
> - 归档日期：2026-05-03 / 完工日期：2026-05-03
> - 起始基线：`origin/main@3c71fe6` (`feat(shape-retirement): read-mode 主链 view-driven 化 (P1-P11, 21 任务)`)
> - 完工基线：PR1 `7718f80` → PR2 `42ed529` → PR3 `1b5d3b3` → PR4 `0892f81` → PR5 `be83e9f` → PR6 (final)
> - 累计 LOC：约 -700 净（删 type AnswerShape + 7 const + IsEmittable + IR 字段 + 5 条 self-consistency rules + reconcileFromObservations + degraded-fallback shape 注入 + RepairSwapShape + buildAnswerDocRequiredFieldRetryHint + analysisAnswerShapes 表 + 14 处 LLM-facing prompt shape 文案 + 200+ 测试 fixture）
> - §12.17 grep 归零矩阵：全部 10 条 production-code 阈值 = 0
> - 前置文档：[`answer_shape_retirement.md`](answer_shape_retirement.md)（基于 7a14a9d 的初版，本文档延续并接管其未尽事项）
> - 范围：read-mode 彻底删除 `AnswerShape` 世界；write-mode 收敛到 `WriteOutputKind`
> - 不在范围：V2 block 渲染层调整、富答案策略调整、新 evaluator 设计

## 完工速览

| PR | commit | 净 LOC | 影响文件 | 阶段 |
|---|---|---|---|---|
| PR1 | 7718f80 | -163 | 7 | P0-A 删 emit_analysis schema + P0-B 删 skill enum |
| PR2 | 42ed529 | -315 | 13 | P0-D 删 analyzer 内部 reconcile + P0-E degraded fallback + P1-C hint composer 注释 |
| PR3 | 1b5d3b3 | -67  | 30 | P1-A 删 finalizer hint + P1-B 删 skill OutputFormat shape 文案 + P1-D RepairSwapShape→RepairSwapView + P1-E ViolShape*→Viol*View |
| PR4 | 0892f81 | +5  | 4  | P2-A taxonomy AppliesToShapes→AppliesToFamilies + back-compat alias |
| PR5 | be83e9f | -188 | 32 | P0-C 删 type AnswerShape + 7 const + IR 字段 + P2-D answer_document SummaryCap 注释 + P3 docs/architecture.md |
| PR6 | (final)  | ~ -25 | ~10 | grep 归零 sweep + V1-routing 测试 rename + 残余 IRField "answer_shape" → "question_kind" |

---

## 0. 现状基线核对（基于 3c71fe6 的真实代码状态）

施工前在当前 HEAD 上做了一次 grep 级核对，下表是设计文档 §2.2 列出的"未删除干净的部分"在当前代码里的真实命中。**所有标 ✅ 的命中点都进入下文 §6 的施工范围；标 🟡 的是设计文档里写"已经做完"但实际还有残影的小尾巴；标 ❌ 的是设计文档错判（实际已经不存在或已经退化为 stub）。**

### 0.1 设计文档 §2.1 "已经完成"中的事实核对

| 设计文档断言 | 现状 grep | 结论 |
|---|---|---|
| `AnswerSurfacePlan.RequiredShape` 已退休 | `internal/types/answer_surface_plan.go:19` 注释明确 retired；无字段 | ❌ 设计文档准确 |
| `answer_shape_runtime.go` 已删除 | `ls internal/orchestrator/answer_shape_runtime.go` → not found | ❌ 设计文档准确 |
| `internal/analysis/contract/checker.go::checkShape` 已是退休 stub | `checker.go:162-171` `checkShape(_, _) []Violation { return nil }` | ❌ 设计文档准确 |

### 0.2 设计文档 §2.2 "未删除干净"中的事实核对

#### 命中 1：analyzer schema 仍输出 `answer_shape`
- `internal/tool/emit_analysis.go:54` `AnswerShape string`（params 字段）
- `internal/tool/emit_analysis.go:72` `ShapeConfidence float64`
- `internal/tool/emit_analysis.go:190` schema enum 仍引用 `skill.AnalysisAnswerShapeValues()`
- `internal/tool/emit_analysis.go:234` `shape_confidence` schema
- `internal/tool/emit_analysis.go:295` required-fields 列表含 `answer_shape`
- `internal/tool/emit_analysis.go:362` `normalizeAnswerShape(p.AnswerShape)`
- `internal/tool/emit_analysis.go:421` `validateConfidenceRange(... p.ShapeConfidence)`
- `internal/tool/emit_analysis.go:568` 写入 `rm.ShapeConfidence`
- `internal/tool/emit_analysis.go:660,695,707,711,732` 5 条 `validateSelfConsistency` rules 读 `answer_shape` 文案
- `internal/tool/emit_analysis.go:1230` 只读校验 `check("answer_shape", ...)`
- `internal/tool/emit_analysis.go:1298-1301+` `normalizeAnswerShape` helper

✅ 完全符合设计文档判断；规模比设计文档列得更大（schema + 5 consistency rules + reject string + normalize helper）

#### 命中 2：`internal/skill/analysis_contract.go` 仍把 shape 作为一等义务
- 第 119 行：`analysisAnswerShapes []AnalysisEnumChoice` enum 表
- 第 129-137 行：7 条枚举说明（list_of_symbols / step_list / value / boolean / config_value / explanation / none）
- 第 238-256 行：`AnalysisAnswerShapeChoices()` / `AnalysisAnswerShapeValues()`
- 第 282 行：长达 ~1500 字符的 description 串教 LLM "answer_shape must match the answer's structural shape: list_of_symbols when..."
- 第 349, 351, 364, 381, 388, 410, 435, 441-443 行：goal / instructions 反复说"choose answer_shape"

✅ 完全符合设计文档判断

#### 命中 3：核心 IR 仍存活 shape 字段
- `internal/types/analysis_ir.go:445` `AnalyzerHints.Shape string`
- `internal/types/analysis_ir.go:65` `RequestModel.ShapeConfidence float64`
- `internal/types/analysis_ir.go:818` `AnswerContract.RequiredAnswerShape AnswerShape`
- `internal/types/analysis_ir.go:948-958` `type AnswerShape string` + 7 个常量
- `internal/types/analysis_ir.go:965-972` `AnswerShape.IsEmittable() bool`

✅ 完全符合设计文档判断

#### 命中 4：write-mode 残留 ShapeChangePlan / ShapeChangeReport
- `grep "ShapeChangePlan\|ShapeChangeReport" --include="*.go"` → 0 hits

❌ 设计文档误判：这两个常量已经不存在。设计文档 §2.2(7) / §6.7 的 write-mode 残留点已经清干净，可以从待办清单去除。

#### 命中 5：analyzer / patcher 仍在主动修 shape
- `internal/agent/analyzer.go:763` retry hint 编 `data["answer_shape"] = hints.Shape`
- `internal/agent/analyzer.go:1001-1002` trace 文案 `shape=%s`
- `internal/agent/analyzer.go:1087-1090` "sub_topics detected, forcing answer_shape explanation"（**强写**）
- `internal/agent/analyzer.go:1411` 注释提到 V2 不再读 `RequiredAnswerShape`，但下方还在维护
- `internal/agent/analyzer.go:1961-1996` `mapLegacyAnswerShape` helper
- `internal/agent/analyzer.go:2156-2218` `reconcileFromObservations` 仍把 ShapeConfidence ≥ 0.7 当作"重发判据"，并 `rm.AnalyzerHints.Shape = shapeValueLabel` 强改
- `internal/analysis/patcher/patcher.go:111` patch allowlist 含 `"answer_shape": true`
- `internal/analysis/patcher/patcher.go:265-274` patch 分支 `case "answer_shape"`
- `internal/analysis/gate/coherence.go:240, 318-369, 458` shape-vs-subject + axis_collapse rule 仍以 shape 文案描述
- `internal/agent/analyzer_boundary.go:61-71` 边界层强改 `rm.AnalyzerHints.Shape`
- `internal/types/context.go:452` Field 字段说明仍含 "shape"
- `internal/types/context.go:142-1374` `AnswerDocAttemptShape` 全套 setter/getter——**这是答案文档"成功的体型快照"，命名沾"shape"但语义与 AnswerShape 无关，不在删除范围**

✅ 大部分符合设计文档；要点是"分清两个 Shape"——`AnswerShape` 删；`AnswerDocAttemptShape` 保留（属 V2 答案体型监测，单独章节澄清）

#### 命中 6：finalizer prompt / repair / hint 仍向模型暴露旧 shape 术语
- `internal/agent/answer_document_evaluator.go:154,269` 注释提及 retired 但旁边的 reject 文案仍走旧路
- `internal/agent/answer_document_evaluator.go:2200` 解析 reject 字符串 "summary length %d exceeds cap %d for shape=%s"
- `internal/agent/answer_document_evaluator.go:2347-2349` `answerDocRejectCodeAbsentExactConfigValueShape` + reject prose 含 "shape=config_value" / "shape=explanation"
- `internal/agent/answer_document_evaluator.go:2364-2549` 多处 retry hint switch 命中 "shape=value requires summary..." / "shape=config_value requires summary..." / "shape=explanation requires a non-empty summary"
- `internal/skill/defaults.go:121, 127, 171-175, 268` skill prompt 仍按 shape 分支讲格式
- `internal/context/builder.go:681, 744, 798, 1639, 1644, 1950-1953, 2330-2338` shape-aware 路径仍在
- `internal/types/repair.go:45-48, 71, 80, 173-175, 224` `RepairSwapShape` repair kind 全套
- `internal/types/violation.go:73, 79, 226, 339, 517` violation 注释里 shape-era 描述
- `internal/analysis/hint/composer.go:268` 只剩一条 reference 注释，主路径已不写 shape——**hint composer 实际已经主动迁完，设计文档判断略保守**

✅ 大致符合；hint composer 比设计文档想象中干净；evaluator + skill + builder 是真主战场

#### 命中 7：degraded fallback 仍硬编码 shape
- `internal/orchestrator/orchestrator.go:5040` `Shape: string(types.ShapeExplanation)`
- `internal/orchestrator/orchestrator.go:5049` `RequiredAnswerShape: types.ShapeExplanation`

✅ 完全符合设计文档判断

#### 命中 8：taxonomy / reviewer / metadata 形态级残留
- `internal/types/answer_taxonomy.go:25, 37, 53, 58, 91, 94-99, 153` `AppliesToShapes []string` 字段名 + 注释
- `internal/orchestrator/answer_taxonomy_store.go:195, 345-347` 排名 + 序列化路径仍读 `AppliesToShapes`
- `internal/orchestrator/answer_reviewer.go:49-108, 232-250` reviewer prompt + back-compat alias 仍用 "applies_to_shapes"

✅ 完全符合设计文档判断

#### 命中 9：测试 / 文档 / 注释残留
- `internal/tool/emit_answer_document_v2_test.go:31, 46` `RoutesV1` 测试名残留（设计文档列出的样本，已确认）
- `docs/architecture.md:309, 423, 465, 621, 629, 686, 702, 1319-1322` 8+ 段仍按 AnswerShape 行文（包括 `type AnswerShape string` 代码块）
- `grep -l "AnswerShape" --include="*_test.go"` → 36 个测试文件
- 多个 source comment 仍把 shape 作为"当前机制"叙述

✅ 完全符合设计文档判断；但规模偏大（测试 36 个 + 文档 1 大节 + 注释若干）

### 0.3 现状结论

**设计文档判断 9 处中：8 处准确、1 处过判（write-mode ShapeChangePlan/Report 已不存在）。** 实施时可以直接按设计文档 §6 流程跑，但 §6.7 阶段中的"删除 ShapeChangePlan / ShapeChangeReport"任务已无对象，应替换为"扫描 write-mode 残余 shape 注释 + answer_document.go 内 SummaryCapFor 系列死代码"。

---

## 1. 文档目的

这份文档基于当前远端最新代码基线编写：

- `origin/main@3c71fe67`

目标不是再解释"为什么 block-only 是好方向"，而是给出一份**可以直接指导开发实施**的主文件施工清单，解决下面这个现实问题：

- 最终答案载体已经是 V2 block-only；
- 但 `AnswerShape` 仍然残留在 analyzer、IR、prompt、runtime fallback、repair、hint、taxonomy、测试和文档中；
- 当前系统仍处于"**block-only carrier + shape 语义残留**"的双世界状态；
- 如果不系统性清理，后续开发会持续被旧 shape 语义误导，造成：
  - prompt 继续向模型暴露旧术语；
  - runtime 继续通过 shape 做隐式分流；
  - 新老 gate 相互打架；
  - 新人继续在旧字段上堆逻辑；
  - 最终答案虽然是 V2 blocks，但上游语义仍然不是单一真理源。

这份文档的最终目标是：**指导开发团队彻底删除 read-mode 的旧 shape 世界**，形成唯一主链：

- `QuestionFamily`：唯一 coarse classifier
- `AnswerSemanticView`：唯一 read-mode runtime semantic contract
- `AnswerDocumentV2`：唯一最终答案 carrier
- `WriteOutputKind`：唯一 write-mode output classifier

---

## 2. 当前现状审计

## 2.1 已完成的部分

下面这些链路，当前代码已经切到 block-only / semantic-view 世界，不应重复开发：

### 最终答案 carrier 已经是 V2 block-only

- [internal/types/answer_document_v2.go](../../internal/types/answer_document_v2.go)
- [internal/tool/emit_answer_document.go](../../internal/tool/emit_answer_document.go)
- [internal/tool/emit_answer_document_v2.go](../../internal/tool/emit_answer_document_v2.go)
- [internal/render/answerdoc.go](../../internal/render/answerdoc.go)
- [internal/render/apply_authority_hedging.go](../../internal/render/apply_authority_hedging.go)
- [internal/orchestrator/contract_check_block.go](../../internal/orchestrator/contract_check_block.go)

### 新官方语义底座已经存在

- [internal/types/claim_form.go](../../internal/types/claim_form.go)
- [internal/types/facet_plan.go](../../internal/types/facet_plan.go)
- [internal/types/rendered_claim_use.go](../../internal/types/rendered_claim_use.go)
- [internal/types/answer_semantic_view.go](../../internal/types/answer_semantic_view.go)
- [internal/types/answer_semantic_view_compile.go](../../internal/types/answer_semantic_view_compile.go)
- [internal/types/answer_semantic_view_helpers.go](../../internal/types/answer_semantic_view_helpers.go)
- [internal/types/answer_surface_plan.go](../../internal/types/answer_surface_plan.go)

### 已经删掉或替换掉的旧 runtime 控制点

- `AnswerSurfacePlan.RequiredShape` 已退休，当前 [internal/types/answer_surface_plan.go](../../internal/types/answer_surface_plan.go) 已明确说明由 semantic view helpers 替代
- `answer_shape_runtime.go` 已删除，不应再被列为待删项
- 旧 shape oracle 主校验链已不再是 read-mode 的活跃主 gate：
  - [internal/analysis/contract/checker.go](../../internal/analysis/contract/checker.go) 中 `checkShape` 已是退休 stub

## 2.2 仍未删除干净的部分

### 1. Analyzer 仍然把 `answer_shape` 作为正式输出字段

文件：

- [internal/tool/emit_analysis.go](../../internal/tool/emit_analysis.go)
- [internal/skill/analysis_contract.go](../../internal/skill/analysis_contract.go)

当前问题：

- `emit_analysis` 的参数 schema 仍包含：
  - `answer_shape`
  - `shape_confidence`
- tool schema 仍要求模型显式产出 `answer_shape`
- skill 合同仍把 `answer_shape` 当成 analyzer 的一等义务

为什么这是阻塞：

- 只要 analyzer 还被要求生成 `answer_shape`，`AnswerShape` 就不是"历史字段"，而是**当前主链的正式语义轴**
- 这会继续污染 analyzer 自我修复、patcher、consistency gate、degraded fallback、prompt 文案和测试

### 2. 核心 IR 里仍存活 `AnswerShape`

文件：

- [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)

当前问题：

- `AnswerContract.RequiredAnswerShape`
- `AnalyzerHints.Shape`
- `ShapeConfidence`
- `type AnswerShape`

为什么这是阻塞：

- 只要核心 IR 还存 shape 字段，任何新代码都可能继续读取它
- IR 是系统最强耦合点，留在这里就相当于保留了一条暗门

### 3. Analyzer / patcher 仍在主动修 shape

文件：

- [internal/agent/analyzer.go](../../internal/agent/analyzer.go)
- [internal/analysis/patcher/patcher.go](../../internal/analysis/patcher/patcher.go)
- [internal/analysis/gate/coherence.go](../../internal/analysis/gate/coherence.go)
- [internal/agent/agent.go](../../internal/agent/agent.go)
- [internal/types/context.go](../../internal/types/context.go)

当前问题：

- analyzer 仍会根据观察结果重写 `AnalyzerHints.Shape`
- patcher 仍允许 `answer_shape` patch
- coherence gate 仍有 shape-specific rule 文案
- context pressure fallback 仍把 best-effort `answer_shape` 当成降级产物

为什么这是阻塞：

- 这代表 shape 不只是"模型输出字段"，而是**编译后运行时仍然可被自动修正的策略变量**

### 4. finalizer prompt / repair hint 仍向模型暴露旧 shape 术语

文件：

- [internal/agent/answer_document_evaluator.go](../../internal/agent/answer_document_evaluator.go)
- [internal/context/builder.go](../../internal/context/builder.go)
- [internal/skill/defaults.go](../../internal/skill/defaults.go)
- [internal/analysis/hint/composer.go](../../internal/analysis/hint/composer.go)
- [internal/types/repair.go](../../internal/types/repair.go)
- [internal/types/violation.go](../../internal/types/violation.go)

当前问题：

- retry hint 中仍会出现：
  - `shape=value`
  - `shape=config_value`
  - `shape=explanation`
- skill 中仍会用 "For step_list shape... / For list_of_symbols shape..."
- repair/violation 类型仍以 "swap shape" 为核心语义

为什么这是阻塞：

- 即使 runtime 已经 block-only，只要 prompt 还继续向 LLM 暴露 shape 语言，模型就会沿旧 mental model 组织答案
- 这会导致：
  - 新 V2 blocks 里仍然套着旧 shape 解释套路
  - prompt 术语和 runtime 真相不一致
  - 新开发人员误以为 shape 仍是官方 contract

### 5. degraded fallback 仍硬编码 shape

文件：

- [internal/orchestrator/orchestrator.go](../../internal/orchestrator/orchestrator.go)

当前问题：

- `buildDegradedFallbackIR(...)` 仍直接注入：
  - `AnalyzerHints.Shape = explanation`
  - `AnswerContract.RequiredAnswerShape = ShapeExplanation`

为什么这是阻塞：

- 这是最隐蔽的 shape 复活入口
- 即使主路径删净，只要 degraded fallback 继续注入 shape，线上压力场景下旧世界还会被重新带回主链

### 6. taxonomy / reviewer / metadata 仍有形态级残留

文件：

- [internal/types/answer_taxonomy.go](../../internal/types/answer_taxonomy.go)
- [internal/orchestrator/answer_taxonomy_store.go](../../internal/orchestrator/answer_taxonomy_store.go)
- [internal/orchestrator/answer_reviewer.go](../../internal/orchestrator/answer_reviewer.go)

当前问题：

- `AppliesToShapes` 字段名仍然残留
- 注释和 reviewer metadata 仍有旧 `shape` 说法

为什么这是阻塞：

- 这是"看起来不是功能问题，但会持续误导开发"的典型残留
- 会让后续新逻辑继续沿着 shape 维度组织 taxonomy/review

### 7. write-mode 已有 `WriteOutputKind`，但 IR 里 shape 残留仍未清完

> ❌ **Section 0.2 命中 4 已确认 ShapeChangePlan / ShapeChangeReport 在当前 HEAD 不存在；本节保留作为历史背景说明，实际无施工对象。**

文件：

- [internal/types/write_output_kind.go](../../internal/types/write_output_kind.go)
- [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)

当前问题：

- ~~`ShapeChangePlan`~~（已删除）
- ~~`ShapeChangeReport`~~（已删除）

为什么这是阻塞：

- write-mode 已经有了独立输出轴，read/write 不应继续共享 `AnswerShape`
- 这是最容易先删的一批，但如果不先清掉，会拖慢整个删旧工程

### 8. 测试、注释、文档仍大量保留旧 shape 世界

文件类型：

- `*_test.go`
- `docs/*.md`
- `docs/design/*.md`
- `docs/migration/*.md`

典型例子：

- [internal/tool/emit_answer_document_v2_test.go](../../internal/tool/emit_answer_document_v2_test.go) 中仍有 `RoutesV1` 语义命名残留
- [docs/architecture.md](../architecture.md) 仍有 shape-era 文字

为什么这是阻塞：

- 删旧不能只删功能代码
- 测试、注释、文档中的旧术语会继续误导后续实现和 code review

---

## 3. 最终目标

必须明确：最终目标**不是**"让 `AnswerShape` 退化成一个无害枚举"，而是：

### 目标 A：read-mode 彻底删除 `AnswerShape`

read-mode 最终不允许再有：

- `AnswerShape`
- `RequiredAnswerShape`
- `AnalyzerHints.Shape`
- `ShapeConfidence`
- `shape=` prompt 语言
- shape-based repair / hint / violation / fallback

### 目标 B：分类、运行时语义、最终载体三层彻底解耦

最终体系必须只有：

1. **分类层**
   - `QuestionFamily`
   - `QuestionStructure`
   - `AnswerSubject`
   - `PredicateAxis`

2. **运行时语义层**
   - `AnswerSemanticView`
   - `FacetCoverageContract`
   - `RenderedClaimUse`
   - `DiagramContract`
   - `ExactResolution`
   - `RichnessCandidates`

3. **最终载体层**
   - `AnswerDocumentV2`

4. **write-mode 输出层**
   - `WriteOutputKind`

### 目标 C：所有 prompt / hint / repair 都切到 family/view/block/facet 语言

最终对模型暴露的术语中，不应再出现：

- `answer_shape`
- `shape=value`
- `shape=step_list`
- `list_of_symbols shape`
- `swap shape`

应统一替换为：

- `question family`
- `required blocks`
- `principal scalar`
- `ordered principal list`
- `enumeration slate`
- `facet coverage`
- `diagram obligations`
- `uncertainty boundary`

### 目标 D：删旧要做到"零容忍"

不允许：

- 保留未消费的旧字段
- 保留旧 fallback 以"更保险"
- 保留旧 gate 以"多一道检查"
- 保留旧 prompt 以"兼容模型习惯"
- 保留旧测试名和注释以"先不影响功能"

凡是 read-mode shape 旧物，只要新的 family/view/block 主链已能表达，就必须删掉。

---

## 4. 责任边界设计

## 4.1 Analyzer 层责任

负责：

- 理解用户问题
- 输出 coarse classification：
  - `QuestionFamily`
  - `QuestionStructure`
  - `AnswerSubject`
  - `PredicateAxis`
  - `ExactResolution` 候选
  - `DiagramHint`

不再负责：

- 预测最终答案 shape
- 决定是 `value` 还是 `step_list`
- 决定 summary cap
- 直接告诉 finalizer "你必须按 shape 输出"

为什么：

- analyzer 负责"问题是什么"，不是"最终文档长什么样"
- 否则就会继续把分类层和渲染层耦合在一起

## 4.2 Semantic Compiler 层责任

负责：

- 从 analyzer 输出和 evidence state 编译：
  - `AnswerSemanticView`
  - `FacetCoverageContract`
  - `Diagram obligations`
  - `Richness candidates`
  - `Uncertainty policies`

不再负责：

- 直接写 prose
- 使用 shape 枚举做 runtime shortcut

为什么：

- 这是新的唯一 read-mode runtime contract
- 需要成为 finalizer、validator、renderer 的共同真理源

## 4.3 Finalizer Prompt Builder 层责任

负责：

- 将 semantic view / facets / current citations 转成 LLM 可消费 prompt
- 明确 required blocks、required facets、diagram obligations、uncertainty boundary

不再负责：

- 通过 prompt prose 临时修正 shape 逻辑
- 用 `shape=value` 之类提示弥补 runtime 缺口

为什么：

- prompt 只能表达 contract，不应承担 schema 迁移补丁职责

## 4.4 Validator / Contract Check 层责任

负责：

- 检查：
  - block coverage
  - facet coverage
  - rendered claim use
  - diagram validity
  - uncertainty/render policy compliance

不再负责：

- 再次运行旧 shape 推断
- 用文本启发式猜"这看起来像 value/step_list"

为什么：

- 一旦 validator 继续做 shape 猜测，旧世界就没死

## 4.5 Renderer 层责任

负责：

- 把 `AnswerDocumentV2` blocks 转成人可读答案
- 应用 authority hedging
- 渲染 diagram block / ordered list / sections / caveats

不再负责：

- 根据 shape 做 carrier 选择
- 通过 post-normalization 补 shape 语义

为什么：

- renderer 应只做 materialization，不应再决定答案结构

---

## 5. 实施总策略

删除 `AnswerShape` 必须遵循一个原则：

**先迁消费点，再删字段；先切语义，再切术语；先切主路径，再切 fallback；最后做 grep 归零。**

如果顺序反了，会出现：

- prompt 还在讲 shape，但 IR 字段没了
- fallback 还在注 shape，但主链删了
- validator 还在查 shape，但 finalizer 已改 block
- 文档/测试和真实实现长期分裂

因此推荐 7 个阶段：

1. 冻结 shape 再增长
2. 切 analyzer 输出
3. 切 prompt / hint / repair
4. 切 runtime / degraded fallback
5. 切 taxonomy / reviewer / metadata
6. 删字段、删类型、删 tests / docs
7. 做终局审计和 grep 归零

---

## 6. 分阶段施工清单

## 6.1 Phase 0：冻结 shape 再增长

### 目标

确保在删旧过程中，不再有新代码继续依赖 `AnswerShape`。

### 要做什么

1. 在 code review 规则里新增硬要求：
   - read-mode 禁止新增 `AnswerShape` 消费者
   - 禁止新增 prompt 文案里的 `shape=...`
   - 禁止新增 tests 断言 `shape=value/step_list/...`

2. 在迁移文档和团队约定中明确：
   - `QuestionFamily` 是唯一 coarse classifier
   - `AnswerSemanticView` 是唯一 runtime semantic contract

### 为什么先做

- 不先冻结，后面删旧边删边长，永远做不完

---

## 6.2 Phase 1：删除 analyzer 输出中的 `answer_shape`

### 核心目标

让 analyzer 不再生成、修正、patch `answer_shape`。

### 主文件

#### A. [internal/tool/emit_analysis.go](../../internal/tool/emit_analysis.go)

### 必须删除/改造

- `emitAnalysisParams.AnswerShape`
- `emitAnalysisParams.ShapeConfidence`
- schema 中的 `"answer_shape"`
- schema 中的 `"shape_confidence"`
- `normalizeAnswerShape(...)`
- `rejectDegenerateClassification(...)` 中所有 shape-specific 分支
- `validateSelfConsistency(...)` 中所有基于 shape 的一致性规则

### 替代方案

用现有字段组合替代 shape：

- `QuestionFamily`
- `QuestionStructure`
- `AnswerSubject`
- `PredicateAxis`
- `DiagramHint`
- `ExactResolution`

### 具体策略

1. tool schema 改成不再接受 `answer_shape`
2. 所有 consistency rule 改成对以下关系做校验：
   - family vs subject
   - family vs predicate axis
   - family vs question structure
   - exact-target intent vs subject kind
   - diagram hint vs family
3. `ShapeConfidence` 直接删除，不建议保留为别的 confidence 桶，除非有明确消费者

### prompt 写法要求

把"选择 answer_shape"替换为：

- 识别问题所属 family
- 识别问题是否要求完整枚举、角色定位、优先级链、调用链、根因追踪
- 识别 diagram 是否有价值
- 识别 exact target / absence / ambiguity

LLM 可见文案应避免：

- `answer_shape=value`
- `step_list`
- `list_of_symbols`

应改成：

- `single resolved scalar`
- `ordered mechanism list`
- `enumeration with completeness boundary`
- `role lookup result`

#### B. [internal/skill/analysis_contract.go](../../internal/skill/analysis_contract.go)

### 必须删除/改造

- `analysisAnswerShapes`
- `AnalysisAnswerShapeChoices()`
- `AnalysisAnswerShapeValues()`
- 所有向模型解释 `answer_shape` 的段落

### 替代方案

新增或强化：

- `QuestionFamily` 说明
- `QuestionStructure` 说明
- `AnswerSubject` 说明
- `PredicateAxis` 说明
- `DiagramHint` 说明
- `Exact target / ambiguity / absence` 说明

### 为什么必须做

- 只删 tool schema 不删 skill，会导致 prompt 和 schema 打架

#### C. [internal/agent/analyzer.go](../../internal/agent/analyzer.go)

### 必须删除/改造

- `mapLegacyAnswerShape(...)`
- 所有 `AnalyzerHints.Shape` 强改逻辑
- 所有 `shape=%s` runtime trace
- `reconcileFromObservations(...)` 里用 literal 观察结果强推 `value` 的逻辑

### 替代方案

保留 literal 观察，但不要映射 shape，改成增强：

- `AnswerSubject`
- `QuestionStructure`
- `ExactResolution`
- `QuestionFamily`

例如：

- 有明确 quoted literal，不是推成 `value`
- 而是推高 `QuestionFamily=role_lookup` 或 `resolved_literal_or_symbol` facet 置信度

#### D. [internal/analysis/patcher/patcher.go](../../internal/analysis/patcher/patcher.go)

### 必须删除/改造

- patch allowlist 中的 `answer_shape`
- 所有 `field=="answer_shape"` patch 分支

### 替代方案

patch 只允许修：

- `QuestionFamily`
- `QuestionStructure`
- `AnswerSubject`
- `PredicateAxis`
- `ExactResolution`
- `DiagramHint`

---

## 6.3 Phase 2：删除 IR 中的 `AnswerShape`

### 核心目标

让 IR 不再保存 shape 语义。

### 主文件

#### A. [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)

### 必须删除

- `type AnswerShape`
- `AnalyzerHints.Shape`
- `AnswerContract.RequiredAnswerShape`
- `ShapeConfidence`
- ~~write-mode 的 `ShapeChangePlan`~~（已删除）
- ~~write-mode 的 `ShapeChangeReport`~~（已删除）

### 替代方案

read-mode 保留：

- `QuestionFamily`
- `QuestionStructure`
- `AnswerSubject`
- `PredicateAxis`
- `DiagramContract`
- `ExactResolution`

write-mode 只保留：

- `WriteOutputKind`

### 注意

这里是**真正的高风险切点**。必须在 Phase 1 的 analyzer/schema/prompt 都切完后再动，否则编译器和 runtime 会同时炸。

#### B. [internal/agent/analyzer_boundary.go](../../internal/agent/analyzer_boundary.go)

### 必须删除/改造

- 所有保留或传播 `AnalyzerHints.Shape` 的边界逻辑

### 替代方案

边界层只传播：

- family
- structure
- subject
- predicate
- exact resolution

---

## 6.4 Phase 3：删除 prompt / hint / repair 中的 shape 术语

### 核心目标

LLM 再也看不到旧 shape 世界。

### 主文件

#### A. [internal/agent/answer_document_evaluator.go](../../internal/agent/answer_document_evaluator.go)

### 必须删除/改造

- `shape=value`
- `shape=config_value`
- `shape=explanation`
- 所有 "target shape" 文案

### 替代方案

统一改成：

- `required blocks`
- `required facets`
- `principal scalar block`
- `ordered mechanism list`
- `enumeration slate`
- `explanation sections`
- `diagram block`

### 具体策略

1. retry hint 不再说：
   - "re-emit with shape=value"
2. 而改说：
   - "re-emit with one principal scalar block and a supporting summary block"
   - "re-emit with an ordered principal list block covering the mechanism path"

#### B. [internal/skill/defaults.go](../../internal/skill/defaults.go)

### 必须删除/改造

- 所有 `For list_of_symbols shape...`
- 所有 `For step_list shape...`
- 所有 `target shape is mandatory`

### 替代方案

skill 必须改成基于：

- family
- required blocks
- required facets
- diagram obligations
- coverage completeness

### prompt 审计要求

LLM-visible prompt 必须满足：

- 无内部术语
- 无 shape 旧名
- 无与 runtime contract 冲突的描述
- 只暴露模型真正需要知道的信息

#### C. [internal/analysis/hint/composer.go](../../internal/analysis/hint/composer.go)

### 必须删除/改造

- 所有 `shape=` 重发提示
- 所有基于 shape 的 hint 模板

### 替代方案

改为：

- `required block missing`
- `facet uncovered`
- `principal scalar missing`
- `diagram edge unsupported`
- `uncertainty boundary missing`

#### D. [internal/types/repair.go](../../internal/types/repair.go)

### 必须删除/改造

- `RepairSwapShape`
- 所有 "swap shape" render text

### 替代方案

按 V2 语义拆成：

- `RepairSwapQuestionFamily`
- `RepairSwapPrincipalBlockKind`
- `RepairAddMissingFacet`
- `RepairReplaceUnsupportedClaimUse`

如果没有真实生产者，直接删除，不要新造概念。

#### E. [internal/types/violation.go](../../internal/types/violation.go)

### 必须删除/改造

- 所有 shape-era violation comments / categories / text

### 替代方案

统一改成：

- family mismatch
- facet coverage failure
- block coverage failure
- unsupported claim form
- diagram contract violation

---

## 6.5 Phase 4：删除 runtime fallback 的 shape 注入

### 核心目标

确保退化路径也走新世界。

### 主文件

#### A. [internal/orchestrator/orchestrator.go](../../internal/orchestrator/orchestrator.go)

### 必须删除/改造

- `buildDegradedFallbackIR(...)` 中对：
  - `AnalyzerHints.Shape`
  - `RequiredAnswerShape`
  的任何赋值

### 替代方案

degraded fallback 只能生成：

- generic `QuestionFamily`
- minimal `QuestionStructure`
- degraded `AnswerSemanticView`

### 为什么必须这样

- degraded fallback 是线上最容易被忽视的暗门
- 只要这里还注入 shape，删旧永远不彻底

---

## 6.6 Phase 5：taxonomy / reviewer / metadata 去 shape 化

### 主文件

#### A. [internal/types/answer_taxonomy.go](../../internal/types/answer_taxonomy.go)

### 必须修改

- `AppliesToShapes` 重命名为 `AppliesToFamilies`

#### B. [internal/orchestrator/answer_taxonomy_store.go](../../internal/orchestrator/answer_taxonomy_store.go)

### 必须修改

- 变量名、注释、评分说明从 shape 改成 family

#### C. [internal/orchestrator/answer_reviewer.go](../../internal/orchestrator/answer_reviewer.go)

### 必须修改

- 所有旧 `applies_to_shapes` 文案和 comments

### 为什么要做

- taxonomy/reviewer 是"结构性元数据"
- 不改这里，团队会继续把 shape 当成长期官方语义轴

---

## 6.7 Phase 6：write-mode 和 summary budget 完全脱 shape

### 主文件

#### A. [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)

### 必须删除

- ~~`ShapeChangePlan`~~（已删除）
- ~~`ShapeChangeReport`~~（已删除）

### 替代方案

- 全部切到 `WriteOutputKind`

#### B. [internal/types/answer_document.go](../../internal/types/answer_document.go)

### 必须检查

当前 budget 已转为：

- `SummaryCapForViewConfig`
- `SummaryCapForView`

### 仍需做的

- 删除所有 shape-era comments / dead helpers / stale names
- 确保没有隐藏的 `SummaryCapFor(shape...)` 残留

> ⚠️ 实施期保留：`AnswerDocAttemptShape`（位于 `internal/types/context.go`）记录答案体型快照，与 `AnswerShape` 命名同根但语义独立（V2 答案监测用），**不在删除范围**。

---

## 6.8 Phase 7：测试、文档、注释、grep 归零

### 核心目标

真正删旧，不留阴影。

### 必须执行的 grep 归零检查

以下命中在 read-mode 终局必须为 0：

- `answer_shape`
- `RequiredAnswerShape`
- `AnalyzerHints.Shape`
- `shape_confidence`
- `ShapeChangePlan`
- `ShapeChangeReport`
- `shape=value`
- `shape=config_value`
- `shape=explanation`
- `list_of_symbols shape`
- `step_list shape`
- `swap shape`

以下命中允许存在，但必须只在迁移文档或历史说明中出现，不得在活跃代码/活跃测试中出现：

- `AnswerShape`
- `shape`

### 必须更新的文件类型

- 所有 `*_test.go`
- `docs/architecture.md`
- `docs/design/*.md`
- `docs/migration/*.md`
- 注释里仍把旧 shape 当当前机制的文件

### 为什么必须做

- 如果测试和文档没跟上，新人会沿着旧词继续写逻辑
- 注释和 docs 的误导作用，长期比少数 dead code 还大

---

## 7. 逐文件主施工清单

下面给出真正可执行的主文件施工顺序，按优先级从高到低。

## P0：最先动，决定删旧成败

1. [internal/tool/emit_analysis.go](../../internal/tool/emit_analysis.go)
2. [internal/skill/analysis_contract.go](../../internal/skill/analysis_contract.go)
3. [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)
4. [internal/agent/analyzer.go](../../internal/agent/analyzer.go)
5. [internal/orchestrator/orchestrator.go](../../internal/orchestrator/orchestrator.go)

为什么：

- 这是 shape 的源头和 fallback 暗门

## P1：第二批，切掉模型可见的旧世界

6. [internal/agent/answer_document_evaluator.go](../../internal/agent/answer_document_evaluator.go)
7. [internal/skill/defaults.go](../../internal/skill/defaults.go)
8. [internal/analysis/hint/composer.go](../../internal/analysis/hint/composer.go)
9. [internal/types/repair.go](../../internal/types/repair.go)
10. [internal/types/violation.go](../../internal/types/violation.go)

为什么：

- 不切这里，模型和开发者都还会继续看到 shape 语言

## P2：第三批，元数据和测试收尾

11. [internal/types/answer_taxonomy.go](../../internal/types/answer_taxonomy.go)
12. [internal/orchestrator/answer_taxonomy_store.go](../../internal/orchestrator/answer_taxonomy_store.go)
13. [internal/orchestrator/answer_reviewer.go](../../internal/orchestrator/answer_reviewer.go)
14. [internal/types/answer_document.go](../../internal/types/answer_document.go)
15. 全部测试和文档

---

## 8. 关键实现策略细化

## 8.1 不要让 `QuestionFamily` 直接取代 shape

错误做法：

- `if family == QFRoleLookup => old shape=value 的所有行为`

正确做法：

- `QuestionFamily` 只负责 coarse classification
- 真正决定输出结构的是：
  - `AnswerSemanticView`
  - `FacetCoverageContract`
  - `RenderedClaimUse`

原因：

- family 太粗，不能直接替代 runtime semantics

## 8.2 Prompt 改写原则

所有 LLM-visible prompt 必须遵守：

1. 不出现内部术语：
   - 不出现 `AnswerShape`
   - 不出现 `shape=value`

2. 用任务语义而不是内部类型名：
   - 说 `single resolved result`
   - 不说 `value shape`

3. 明确 required blocks：
   - summary block
   - principal scalar block
   - ordered list block
   - diagram block
   - caveat block

4. 明确 required facets：
   - exact resolution
   - uncertainty boundary
   - mechanism path
   - completeness buckets

5. 明确 forbidden behaviors：
   - 不要编造 block
   - 不要画无证据 edge
   - 不要把 historical observation 说成 current mechanism

## 8.3 删除不是"保留兼容"，而是"彻底归零"

任何 read-mode 旧字段，只要：

- 没有主路径消费者
- semantic view 已能表达其语义
- fallback 已迁完

就必须删除，不允许：

- "再留一两个版本"
- "先不删测试"
- "先不删注释"

---

## 9. 面向新开发人员的上手顺序

如果是不熟悉代码的新开发者，建议按这个顺序读和改：

1. 先读现状主链
   - [internal/types/answer_document_v2.go](../../internal/types/answer_document_v2.go)
   - [internal/types/answer_semantic_view.go](../../internal/types/answer_semantic_view.go)
   - [internal/types/answer_semantic_view_helpers.go](../../internal/types/answer_semantic_view_helpers.go)
   - [internal/types/facet_plan.go](../../internal/types/facet_plan.go)

2. 再读答案主链
   - [internal/agent/answer_document_evaluator.go](../../internal/agent/answer_document_evaluator.go)
   - [internal/tool/emit_answer_document.go](../../internal/tool/emit_answer_document.go)
   - [internal/render/answerdoc.go](../../internal/render/answerdoc.go)

3. 再读 shape 残留源头
   - [internal/tool/emit_analysis.go](../../internal/tool/emit_analysis.go)
   - [internal/skill/analysis_contract.go](../../internal/skill/analysis_contract.go)
   - [internal/types/analysis_ir.go](../../internal/types/analysis_ir.go)
   - [internal/agent/analyzer.go](../../internal/agent/analyzer.go)

4. 最后做 prompt / hint / tests / docs 收尾

这样顺序最不容易误删，也最能理解"为什么 AnswerShape 现在还没死透"。

---

## 10. 最终审计要求

施工完成后，必须逐项审计：

### 10.1 语义统一

- read-mode 不再有 `AnswerShape`
- analyzer 不再输出 `answer_shape`
- runtime 不再读取 `RequiredAnswerShape`

### 10.2 Prompt 纯净

- prompt 无内部术语
- prompt 无 shape 旧语义
- prompt 与 runtime contract 不矛盾

### 10.3 Gate 不打架

- 无旧 shape gate 与新 block/facet gate 并存
- degraded fallback 不再复活旧 shape

### 10.4 泛化

- 方案不耦合某个题型
- 不耦合 YAML / config / 单个测试题
- 不假设层数上限

### 10.5 模型可见信息充分

- family / required blocks / required facets / uncertainty boundary / diagram obligations 都能在 prompt 中明确看到
- 不依赖模型自己猜内部 contract

### 10.6 无死代码

- 无未消费字段
- 无未调用 helper
- 无只在注释里还活着的旧逻辑

### 10.7 端到端字段消费完整

所有保留字段必须能回答：

- 谁生产
- 谁消费
- 如何验证
- 如何在用户可见答案面体现

回答不清的字段，要么删，要么补主链消费。

### 10.8 Richness 不退化

- 删 shape 不能把 rich answer 压薄
- 该有 diagram 的 family/view 仍有 diagram
- 该有 mechanism path 的答案仍有 mechanism path
- 该有 uncertainty boundary 的仍有

---

## 11. 终局定义

只有满足下面全部条件，才算真正"删除干净"：

1. `AnswerShape` 不存在于 read-mode 活跃代码
2. `RequiredAnswerShape` 不存在于 IR
3. analyzer/schema/skill/prompt 不再出现 `answer_shape`
4. finalizer/hint/repair 不再出现 `shape=...`
5. degraded fallback 不再注入 shape
6. taxonomy/reviewer/metadata 不再以 shape 命名
7. write-mode 只使用 `WriteOutputKind`
8. V2 block-only 是唯一最终答案载体
9. `QuestionFamily + AnswerSemanticView` 是唯一 read-mode semantic authority
10. grep 归零检查通过

只要有任何一项不满足，就不能声称"AnswerShape 已删除干净"。

---

## 12. 细化施工清单（基于 3c71fe6 的精确删点表）

> 本节由本仓库代码审计自动生成，**所有路径与行号均基于 `origin/main@3c71fe6`**。这是一份"精确到符号"的删除清单——开发实施时按 P0→P1→P2 顺序、每个 PR 一个文件作为最小单元，可以避免编译断链。

### 12.1 P0-A · `internal/tool/emit_analysis.go`

| 删点 | 行号 | 形式 | 替代/说明 |
|---|---|---|---|
| `emitAnalysisParams.AnswerShape` | 54 | struct field | 整字段删 |
| `emitAnalysisParams.ShapeConfidence` | 72 | struct field | 整字段删 |
| schema `"answer_shape"` enum 引用 | 190 | `stringProp{Enum: skill.AnalysisAnswerShapeValues()}` | 删；不替换 |
| schema `"shape_confidence"` | 234 | `map[string]any{...}` | 删；不替换 |
| required-fields 清单含 `"answer_shape"` | 295 | string slice | 删 token |
| `normalizeAnswerShape(p.AnswerShape)` 调用 | 362 | 函数调用 | 删 |
| `validateConfidenceRange(... p.ShapeConfidence)` | 421 | 第 4 参 | 移除 ShapeConfidence 参；helper 签名同步收窄 |
| `rm.ShapeConfidence = p.ShapeConfidence` | 568 | 写 IR | 删 |
| degenerate-reject 文案含 `answer_shape=none` | 660 | string literal | 改写——基于 family/subject/intent 重写 |
| 5 条 `validateSelfConsistency` rules（is_role_locate / is_count vs intent / count vs shape / count vs list_of_symbols / is_history vs list_of_symbols） | 695, 707, 711, 732 | 比较分支 | 改成 family ↔ subject / family ↔ predicate_axis / family ↔ question_structure 一致性，详见 §6.2-A |
| `check("answer_shape", raw.AnswerShape, rm.AnalyzerHints.Shape)` | 1230 | dual-emit drift check | 删该行 |
| `normalizeAnswerShape` 函数定义 | 1298–1350 (~) | 整函数 | 删 |

测试同步：删 `internal/tool/emit_analysis_test.go` / `emit_analysis_consistency_test.go` 内全部 shape 断言；新增 family/subject/structure 一致性单测。

### 12.2 P0-B · `internal/skill/analysis_contract.go`

| 删点 | 行号 | 替代 |
|---|---|---|
| `analysisAnswerShapes` 表 | 119–137 | 删；新表 `analysisQuestionFamilies` 由 P0-B 引入（如果 QuestionFamily 已有定义就直接读 types.AllQuestionFamilies） |
| `AnalysisAnswerShapeChoices()` | 238–240 | 删；调用方在 P0-A 已不再使用 |
| `AnalysisAnswerShapeValues()` | 255–256 | 删 |
| description 长串教 LLM 选 shape | 282 | 重写为：教 LLM 选 question family + question structure + answer subject |
| Goal/instruction 多处提 `answer_shape` | 349, 351, 364, 381, 388, 410, 435, 441–443 | 整段重写；output schema 文案以 family/structure/subject 为骨架 |

### 12.3 P0-C · `internal/types/analysis_ir.go`

| 删点 | 行号 | 注意事项 |
|---|---|---|
| `RequestModel.ShapeConfidence` | 65 | 删 |
| `AnalyzerHints.Shape` | 445 | 删；boundary 层在 P2 切完前会断链，按 §5 顺序——本字段最后删 |
| `AnswerContract.RequiredAnswerShape` | 818 | 删；同时检查所有 `EffectiveRequiredAnswerShape` accessor（如有）一并删 |
| `type AnswerShape` + 7 常量 | 948–958 | 整段删 |
| `AnswerShape.IsEmittable()` | 965–972 | 整函数删 |
| 注释 "intentionally orthogonal to AnswerShape" | 889 | 改写为"orthogonal to question family / answer subject" |

**风险**：删 `AnswerShape` 类型会触发约 40 个文件的级联编译错误。必须在 P1（消费点全部迁完）后再做这一步。

### 12.4 P0-D · `internal/agent/analyzer.go`

| 删点 | 行号 | 替代 |
|---|---|---|
| retry hint 写 `data["answer_shape"]` | 763 | 删 key；hint 改为携带 family + subject |
| 两处 trace `shape=%s` | 1001–1002 | 改为 `family=%s subject=%s` |
| sub_topics 强写 ShapeExplanation 块 | 1087–1090 | 删整段——多 sub_topics → multi-topic facet 的事实由 semantic view 表达 |
| `mapLegacyAnswerShape` | 1961–1996 | 整函数删；调用点同步删 |
| `reconcileFromObservations` 内 ShapeConfidence ≥ 0.7 + 强写 shapeValueLabel | 2156–2218 | 改为：观察到精确 literal 时升 `AnswerSubject` 置信度 / 升 `ExactResolution.RequireTargetMention` |
| 注释 "session-22 follow-up B4 — ShapeConfidence floor guard" 整段说明 | 2156–2172 | 重写或删 |

### 12.5 P0-E · `internal/orchestrator/orchestrator.go::buildDegradedFallbackIR`

| 删点 | 行号 | 替代 |
|---|---|---|
| `Shape: string(types.ShapeExplanation)` | 5040 | 删行 |
| `RequiredAnswerShape: types.ShapeExplanation` | 5049 | 删行 |
| 函数注释提及 "ShapeExplanation so the finalizer's prose render path is active" | 5017–5018 | 改为说明 family=Generic + minimal QuestionStructure |

构造完后 IR 仍需通过 contract checker，注意 V2 block contract 在空载 IR 上是否仍能通过；如不行，degraded fallback 内补一份 default `AnswerSemanticView`。

### 12.6 P1-A · `internal/agent/answer_document_evaluator.go`

| 删点 | 行号 | 形态 |
|---|---|---|
| reject 解析 `summary length %d exceeds cap %d for shape=%s` | 2200 | 改成读 view kind |
| `answerDocRejectCodeAbsentExactConfigValueShape` reject prose | 2347–2349 | 重写：用 "exact-resolution facet absent" 语言 |
| literal-grounding gate 注释 | 2364–2365 | 改写 |
| 5 处 retry hint `case strings.Contains(summary, "shape=...")` | 2544–2549 | 整 switch 重写为 view/block/facet 语言 |

### 12.7 P1-B · `internal/skill/defaults.go`

| 删点 | 行号 |
|---|---|
| Bucket-alignment shape 分支讲法 | 121 |
| log-triage prefer step_list 文案 | 127 |
| OutputFormat 5 行 shape 分支讲解（explanation / step_list / list_of_symbols / boolean / value & config_value） | 171–175 |
| answer-symbol slate "list_of_symbols / shape=explanation" 触发文案 | 268 |

整段重写：以 "required blocks + required facets + diagram contract + uncertainty boundary" 为新组织。

### 12.8 P1-C · `internal/analysis/hint/composer.go`

| 命中 | 行号 | 处置 |
|---|---|---|
| 注释 reference `docs/migration/answer_shape_retirement.md` | 268 | 改指向本文件；hint composer 主路径已无 shape 写入，仅清注释 |

### 12.9 P1-D · `internal/types/repair.go`

| 删点 | 行号 |
|---|---|
| `RepairSwapShape` 常量 + 注释 | 45–48 |
| AllRepairKinds 枚举入列 | 71 |
| `IsValid` switch case | 80 |
| Render 模板注释 + 文案 `swap_shape:` | 173–175, 224 |

无生产者直接删；不新造 `RepairSwapQuestionFamily`（设计文档允许"无生产者直接删"）。

### 12.10 P1-E · `internal/types/violation.go`

| 行号 | 处置 |
|---|---|
| 73 | "Caught by runAnswerShapeOracle. SuspectedRoot: answer_shape." → 改成 family / subject / structure |
| 79 | 同上 |
| 226 | 注释删 shape-era 描述 |
| 339 | 同上 |
| 517 | patcher 字段名引用注释——删 `answer_shape` token |

### 12.11 P2-A · `internal/types/answer_taxonomy.go`

| 删点 | 行号 |
|---|---|
| 字段 `AppliesToShapes []string` | 99 |
| 注释 25 / 37 / 53 / 58 / 91 / 94–98 / 153 | 改 family 语言 |

字段重命名为 `AppliesToFamilies`；JSON tag 同步从 `applies_to_shapes` 改成 `applies_to_families`。

> ⚠️ taxonomy 是磁盘缓存；改字段名 = 旧 JSON 文件读不出来。两条策略选一：
> - 在 reviewer 解析路径保留 `applies_to_shapes` 作为只读 back-compat alias（`internal/orchestrator/answer_reviewer.go:232–239` 已经有这个 fallback，保留即可），写入时只发 `applies_to_families`；
> - 或在切换 commit 提供一次性迁移：读旧 → 写新 → 删旧。

### 12.12 P2-B · `internal/orchestrator/answer_taxonomy_store.go`

| 行号 | 处置 |
|---|---|
| 195 | 排名注释 "Family match (AppliesToShapes includes family)" → "Family match (AppliesToFamilies includes family)" |
| 345–347 | 字段访问 `p.AppliesToShapes` → `p.AppliesToFamilies` |

### 12.13 P2-C · `internal/orchestrator/answer_reviewer.go`

| 行号 | 处置 |
|---|---|
| 49 | 注释删 shape |
| 60 | 同 |
| 86 | 同 |
| 100 / 104 / 108 | reviewer schema description 重写为 family 语言 |
| 232 | `AppliesToShapes []string` json:"applies_to_shapes" 作为只读 back-compat alias **保留** |
| 238–250 | 解析时 fallback 逻辑保留；写入字段名改为 `AppliesToFamilies` |

### 12.14 P2-D · `internal/types/answer_document.go`

| 已确认状态 |
|---|
| Summary cap 已切到 `SummaryCapForView` / `SummaryCapForViewConfig` |
| 残余动作：grep `SummaryCapFor.*Shape` / `shape\b` 注释，逐个改写或删除 |

### 12.15 P2-E · 测试矩阵

按"先迁主路径再清测试"原则，本批改动会陆续触发约 36 个 `*_test.go` 失败：

```
internal/tool/emit_analysis_test.go
internal/tool/emit_analysis_consistency_test.go
internal/tool/emit_answer_symbol_test.go
internal/tool/emit_investigation_complete_test.go
internal/tool/emit_investigation_complete_precomplete_test.go
internal/tool/emit_answer_document_v2_test.go      ← RoutesV1 命名
internal/types/analysis_ir_test.go
internal/types/answer_semantic_view_test.go
internal/types/cgec_completeness_test.go
internal/types/evidence_closure_test.go
internal/types/evidence_closure_stage_health_test.go
internal/types/evidence_surface_test.go
internal/types/request_traits_test.go
internal/orchestrator/two_turn_e2e_test.go
internal/orchestrator/orchestrator_dag_test.go
internal/orchestrator/read_e2e_regression_test.go
internal/orchestrator/validate_stuck_test.go
internal/orchestrator/block123_e2e_test.go
internal/orchestrator/contract_check_test.go
internal/orchestrator/degraded_fallback_test.go
internal/orchestrator/mode_dispatch_test.go
internal/orchestrator/stopcond_hotloop_test.go
internal/orchestrator/violation_budget_test.go
internal/orchestrator/write_analyze_dispatch_test.go
internal/orchestrator/plan_mode_e2e_test.go
internal/agent/analyzer_test.go
internal/agent/analyzer_intent_test.go
internal/agent/analyzer_classification_grep_test.go
internal/agent/analyzer_prompt_test.go
internal/agent/analyzer_boundary_test.go
internal/agent/answer_document_evaluator_test.go
internal/agent/extractor_test.go
internal/agent/extractor_axis_test.go
internal/agent/explorer_test.go
internal/agent/explorer_erm_test.go
internal/context/builder_test.go
internal/analysis/contract/checker_test.go
internal/analysis/gate/coherence_test.go
internal/analysis/aggregator/aggregator_test.go
internal/analysis/patcher/patcher_test.go
internal/analysis/hint/composer_test.go
internal/skill/keyword_examples_test.go
internal/tool/repomap/render/render_test.go
```

按 P0/P1/P2 顺序，每完成一批主代码，立刻同步该批关联的测试。

### 12.16 P3 · 文档

| 文件 | 改动概述 |
|---|---|
| `docs/architecture.md` | §"4 stages × 4 agents" 表格、`emit_analysis` 参数清单、coherence-gate 说明、`isMultiTopicExplanation` 算法描述、AnswerShape 代码块（行号 309 / 423 / 465 / 621 / 629 / 686 / 702 / 1319-1322）全部按 family/view/block 重写 |
| `docs/migration/answer_shape_retirement.md` | 头部加 deprecation 提示，指向本文件 |
| `docs/migration/block_only_carrier.md` | 与本文件交叉引用 |
| `docs/design/semantic_surface_contract_*.md` | 已在 view-driven 化提案中预留位置；逐文核对，把残留 shape 词条改 family/view |
| `CLAUDE.md` | "AnswerStep.Kind discipline (Plan D rollout 2026-05-02)" 一段提到 RequiredAnswerShape——改写或加 deprecated 注 |

### 12.17 grep 归零阈值

实施完成的硬阈值（**`-l --include="*.go"`** 排除 `_test.go` 后必须为 0）：

```bash
grep -rn "answer_shape\b" --include="*.go" --exclude="*_test.go"
grep -rn "RequiredAnswerShape" --include="*.go" --exclude="*_test.go"
grep -rn "AnalyzerHints\.Shape" --include="*.go"
grep -rn "ShapeConfidence" --include="*.go"
grep -rn "shape_confidence" --include="*.go"
grep -rn "ShapeChangePlan\|ShapeChangeReport" --include="*.go"
grep -rn "shape=value\|shape=config_value\|shape=explanation" --include="*.go" --exclude="*_test.go"
grep -rn "list_of_symbols shape\|step_list shape\|swap shape" --include="*.go"
grep -rn "RepairSwapShape" --include="*.go"
grep -rn "AppliesToShapes" --include="*.go" --exclude="*_test.go" --exclude="answer_reviewer.go"
```

允许残留：

- `AnswerDocAttemptShape` 系列（context.go，命名同根但语义无关）
- `docs/migration/*.md` 内历史叙述
- `answer_reviewer.go` 内 `applies_to_shapes` JSON 反序列化 back-compat alias

### 12.18 PR 拆分建议

| PR | 范围 | 编译边界 |
|---|---|---|
| 1 | P0-A + P0-B（emit_analysis schema/skill 同切） | 编译可过——schema 不再要 shape，但 IR 仍接受空 shape |
| 2 | P0-D + P0-E + P1-C（analyzer 内部不再写 shape；degraded fallback；hint composer 注释） | 编译可过 |
| 3 | P1-A + P1-B + P1-D + P1-E（finalizer hint + skill defaults + repair/violation 注释） | 编译可过 |
| 4 | P2-A + P2-B + P2-C（taxonomy 字段 rename，含 back-compat alias） | 编译可过；磁盘缓存读旧文件仍 OK |
| 5 | P2-D + P0-C + P3（删 IR 类型 + 文档同步） | **危险点**——所有消费者必须在前 4 个 PR 已迁完 |
| 6 | 测试与 grep 归零 | 末班车 |

每个 PR 必须满足：

1. `make` 通过
2. `go test ./...` 通过
3. PR 描述附本文档 §12.17 grep 归零的本 PR 后的进度（哪些命中已清零、哪些预期会在后续 PR 清零）
4. 不允许把多个 phase 合到一个 PR

---

## 13. 风险与回退

1. **风险一：磁盘 taxonomy 缓存格式变更**——见 §12.11 缓解：写新读双（read 同时认 `applies_to_shapes` / `applies_to_families`，write 只发新名）。回退方案：reviewer 解析层保留 alias 不删（已经在 `answer_reviewer.go:232-239` 实现）。
2. **风险二：删 `type AnswerShape` 引发 36 个 test 文件级联崩**——见 §12.15 缓解：按 PR1-4 顺序先迁消费者；PR5 删类型时所有消费者已切走。回退方案：不可逆，必须按顺序。
3. **风险三：degraded fallback 删 shape 后 V2 block contract 失败**——见 §12.5 缓解：在 fallback 内补一份默认的 `AnswerSemanticView` 占位，让 contract checker 拿到合法 view。回退方案：contract_check_block.go 增加"degraded mode" 短路。
4. **风险四：模型旧 mental model 抗性**——切完后短期内可能出现答案体型回潮（模型仍按 list_of_symbols / step_list 内化偏好排版）。缓解：观察一周线上，必要时在 finalizer system message 加一条 "AnswerShape vocabulary is retired; use blocks + facets" reminder。

---

## 14. 维护

- 本文档进入 in-flight 状态后，每个 PR 落地一阶段，请同步更新 §12 的状态标记（如 `(已完成)` / `(进行中)` / 行号偏移）。
- 实施过程中如发现新的 shape 残留点，**追加到本文档相应 §12.x 节，不要新开文档**——保留单一施工真理源。
- 完成后整篇文档保留作为历史归档，状态置 `shipped`，不删。
