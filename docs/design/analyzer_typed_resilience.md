# Analyzer Typed Resilience — Entity Provenance & Mirror Auto-Align

**Status**: Design (not yet implemented).
**Target sessions**: 4 sessions, ~22-28 commits total (per-phase ship + eval, see §7).
**Predecessors**:
- `docs/design/finalizer_pretrip_prevention.md` (finalizer 三层"auto-fix / soft-note / retry" 范式已 SHIPPED — 本文档把同一范式扩展到 analyzer)
- `docs/design/multirepo_entity_scope_separation.md` (`scope_projection.go` COPY 语义并行 typed lane — 本文档的 EntityProvenance 沿用该模板)
- `docs/design/post_v2_runtime_gap_remediation.md` (V2 typed validators)
**Successor**: 留待 §8 Open Questions §8.3 / §8.4 决策后再产 design

**Owner**: 任意接手者（本文档目标：不熟悉本仓库的开发者读完一遍即可开始改）
**Eval bar**: 真 eval 全集 58 case × 1 round 不回归 + 触发本设计的 forensic case（finalyzer-retry-audit）4 round 全 PASS + 零 emit_analysis 重试

**Baseline**: 本文档所有 `internal/...` 文件 file:line 引用基于 **commit `7ac49a35`** (`HEAD` 时点 — 主线 `Fix finalizer draft duplication and status order`)。后续提交可能让具体行号漂移。**实施前**先 grep 关键 symbol（`reconcileQualifiedCodeSymbolConfigDrift` / `projectPrimaryScopes` / `sanitizeSubTopics` / `validateSelfConsistency` / `filterGenericEntitiesWithWhitelist` 等）对齐行号；若漂移，以 grep 真实结果为准，不要按文档行号瞎改。可用以下命令查 baseline 后所有相关代码改动：

```bash
git log --oneline 7ac49a35..HEAD -- \
  internal/agent/analyzer.go internal/agent/analyzer_intent.go \
  internal/agent/analyzer_code_symbol_reconcile.go internal/agent/scope_projection.go \
  internal/tool/emit_analysis.go internal/tool/analysis_limits.go \
  internal/types/analysis_ir.go internal/types/context.go \
  internal/orchestrator/finalizer_auto_repair.go
```

---

## 0. TL;DR — 一句话设计

把 finalizer 已 SHIPPED 的三层范式（`L1 系统 auto-fix` / `L2 soft-note caveat` / `L3 hard retry`，见 `internal/orchestrator/finalizer_auto_repair.go:24-54` + `internal/orchestrator/violation_root_cause.go:161-174`）**移植到 analyzer 侧**，专门治理三类 LLM"过度配合 typed profile"形态：

1. **Entity 污染** — sub_topic.Entities 写入答案形态词（"清单"/"keep"/"remove"），当前完全不过滤；新黑名单类目无穷，硬卡不可持续
2. **Mirror 字段自相矛盾** — `predicates.is_diagnostic_question=false` 但 `diagnostic_profile.is_diagnostic=true`，当前 reconciler 用 OR 升级到 root_cause；正确做法是按 precedence 静默对齐（参考 `d21ac637` 在 `conversation_reference_profile` 上的先例）
3. **Optional typed profile 硬卡 verbatim** — `error_granularity_profile.source_quotes` 精确字符串匹配，转写问题让 LLM 3 轮重试只为对齐一个 quote；正确做法是 deterministic normalization 后仍无法锚定则软清空

**为什么这次方案 stable + 收敛**：三类问题的根因都是 *"系统让 LLM 为系统的精度负责"*——本设计反过来，让系统按 typed precedence 消化 LLM 的小不一致，LLM 只对**结构性**错误负责（degenerate classification / 必填字段缺失）。新增任何 LLM-facing hard reject 前先问"能否做成 typed cleanup 或并行 metadata lane？"——这是本设计的**单一红线**。

**为什么是 EntityProvenance 而不是黑名单**：黑名单和 oracle 解析是正交两轴（噪声 vs 解析度），且都存在合法但解析失败的 entity（答案形态词、外部库、未来标识符）——见 §3.2。新方案给每个 entity 打 typed metadata（origin / resolved / noise_score / use_for_search / use_for_shape），下游 8 个消费点按用途读 metadata，**不在 analyzer 阶段 drop / demote 任何 LLM 写下的 entity**。

---

## 0.1 2026-05-19 审阅修订结论

远端同步后对 `docs/design/analyzer_typed_resilience_forensic_log_20260518_172241.log` 与当前代码重新核对，本文档的根因判断成立：这次 analyzer 重试不是"模型主判断错误"，而是 optional typed lane 与 mirror lane 让系统把小不一致放大成硬失败 / root_cause 路由。

但原实施顺序不是最优，需调整：

1. **先修直接造成重试的确定性硬 gate**：`error_granularity_profile.source_quotes` 缺失 / 转写不应 reject；`diagnostic_profile.is_diagnostic` 不应在 predicate 明确为 false 时反向劫持 intent。两者都能在系统侧 typed normalize，改动小、ROI 最高。
2. **EntityProvenance 先 telemetry-only，后接 consumer**：这是正确的长期方案，但不是本 forensic case 的最短止血路径。Producer 上线前不得先放宽 blocklist/drop 行为，否则可能扩大噪声实体流入 search/shape。
3. **`GenericEntityBlocklist` 降级必须排在 EntityProvenance telemetry 之后**：原文建议 Phase 5 先跑以提供 NoiseScore，这会在尚无 provenance consumer 时改变现有 entity 集合语义。新的顺序是先保守打标，再用真实 telemetry 决定是否降级 drop。
4. **quote 兼容只能做确定性 normalization，不用"最长子串 ≥4"作为通过依据**：中文短语和通用词太容易误命中。无 exact / normalized anchor 时应软清空该 optional profile，而不是用子串把 profile 留真。
5. **retry-hint leak / user-verbatim 只能作用在 typed 字段，不得扫描用户问题或模型散文做流程决策**：允许对 `entities[]` 里的系统自有泄漏 token 打 provenance 标签；不得用 raw request / free prose keyword match 驱动 hard gate。
6. **SoftByDefault 相关说法需以当前代码为准**：当前 `defaultSoftKinds()` 已从 `ViolKindSpec.SoftByDefault` 派生，`isSoftViolationKind()` 也以 active soft policy / `ViolationProfileFor` 为准。若日志中模型声称"未查 SoftByDefault"，应作为待核验的模型判断，不能直接纳入本 analyzer 设计的前提。

修订后的推荐落地顺序见 §8：Phase 4 → Phase 3 → Phase 1 → Phase 2 → Phase 5。

---

## 0.2 Implementation Tracker

> 每批提交前后都刷新本表，避免后续接手者忘记当前边界。状态只描述本设计范围内的实现进度。

| Batch | Scope | Status | Code paths | Exit criteria |
|---|---|---|---|---|
| Batch 1 | Phase 4 + Phase 3：软化 `error_granularity_profile.source_quotes`；新增 diagnostic mirror typed normalizer；收紧 diagnostic reconciler 入口 | **DONE (2026-05-19)** | `internal/tool/emit_analysis.go`, `internal/agent/analyzer_intent.go`, `internal/tool/emit_analysis_test.go`, `internal/agent/analyzer_intent_test.go` | forensic case 不再因 granularity quote 重试；`diagnostic_profile.is_diagnostic=true` 不再在 predicate=false 且无强诊断信号时把 explain 题升级 root_cause |
| Batch 2 | Phase 1：EntityProvenance producer telemetry-only，不接 consumer | **DONE (2026-05-19)** | `internal/types/analysis_ir.go`, `internal/agent/entity_provenance_projection.go`, `internal/agent/analyzer.go`, `internal/agent/entity_provenance_projection_test.go` | 只新增 typed side-lane 与 summary telemetry；下游行为不变 |
| Batch 3 | Phase 2：consumer wiring，search/shape 分流读取 provenance，nil provenance 全 keep | **DONE (2026-05-19)** | `internal/agent/entity_provenance_filter.go`, `internal/agent/keyword_search.go`, `internal/agent/explorer.go`, `internal/agent/ir_accessor.go` | full eval 不增加 retry；多仓/单仓均不误伤合法 entity |
| Batch 4 | Phase 5：schema 文案同步已交付；blocklist drop→noise 降级等待 telemetry 决策 | **DONE / BLOCKLIST DEFERRED (2026-05-19)** | `internal/tool/emit_analysis.go` schema；`internal/tool/analysis_limits.go` 行为保持不变 | schema 与已交付能力一致；无 telemetry 前不得扩大噪声流 |
| Batch 5 | Blocklist shadow telemetry：保持 drop 行为，仅记录未来 drop→noise 路径会如何处理被丢弃实体 | **DONE (2026-05-19)** | `internal/tool/analysis_limits.go`, `internal/tool/emit_analysis.go`, `internal/tool/emit_analysis_test.go` | `DroppedEntities` 和 user-facing warnings 不变；日志新增 `blocklist_shadow`，证明 dropped generic entity 在未来降级时默认不会进入 search/shape |
| Batch 6 | Telemetry/eval 聚合器：把 analyzer provenance、blocklist shadow、finalizer contract violation / repair kind 汇总成 Markdown/JSON 报告 | **DONE (2026-05-19)** | `eval/telemetry`, `eval/telemetry/README.md` | 可对 `eval/results` / `.codrax/logs` / `../customlogs` 做离线聚合；后续 blocklist 降级必须先用本报告证明安全 |

Batch 1 verification:

```bash
go test ./internal/tool
go test ./internal/agent
go test ./internal/orchestrator
go test ./internal/types
go test ./internal/analysis/...
```

Batch 2 verification:

```bash
go test ./internal/agent
go test ./internal/tool
go test ./internal/orchestrator
go test ./internal/types
go test ./internal/analysis/...
```

Batch 3 verification:

```bash
go test ./internal/agent
go test ./internal/tool
go test ./internal/orchestrator
go test ./internal/types
go test ./internal/analysis/...
```

Batch 4 verification:

```bash
go test ./internal/tool -run 'TestEmitAnalysisSchema|TestEmitAnalysis_Execute'
go test ./internal/tool
```

Batch 5 verification:

```bash
go test ./internal/tool -run 'TestValidateAnalysisInput_(DropsGenericEntities|EmptyBlocklistSkipsFilter|WhitelistKeepsVerifiedGenericEntity|WhitelistRejectsPathOnlyGenericEntity)$'
go test ./internal/tool
```

Batch 6 verification:

```bash
go test ./eval/telemetry
go run ./eval/telemetry --format markdown --top 8 ../customlogs
```

---

## 1. 背景：为什么这个 Phase 存在

### 1.1 触发 case（forensic）

`docs/design/analyzer_typed_resilience_forensic_log_20260518_172241.log`（runtime 原路径 `.codrax/logs/codrax-8ae69134/codrax-20260518-172241-000-1068941.log`，已 snapshot 入仓便于跨机审计）记录的 REPL session（2026-05-18 17:25 dispatch）：

```
[repl] dispatching request: 全面排查finalyzer阶段的各种重试是否真的必要，
       哪些其实是可以靠系统进行修复的，原则是…
```

用户原文是**代码审计 / 设计简化建议**（design audit），不是 runtime diagnose。LLM 主判断对：

- `intent=explain` ✓
- `question_kind=enumeration` ✓
- `complexity=complex` ✓
- `is_category_enumeration=true` ✓
- 10 个 `required_files` 全部解析到真实文件路径 ✓

但 typed profile 边角有 3 处自相矛盾或过度填充（log L820-L885）：

| 字段 | LLM 输出 | 问题 |
|---|---|---|
| `predicates.is_diagnostic_question` | `false` ← LLM 主判断正确 | — |
| `diagnostic_profile.is_diagnostic` | `true` ← 与上一行 mirror 矛盾 | LLM 错填 mirror 副本 |
| `error_granularity_profile.is_granularity_question` | `true` ← schema 描述明确限定 per-item/whole-batch 失败 scope，与本题无关 | LLM 套词（把"逐个 retry 评估"读成 per-item rejection） |
| `sub_topic[3].entities` | `["keep","remove","recommend","清单"]` | 4 个 token 都不是代码标识符，是答案形态描述词 |

下游放大：

- **Reconciler `internal/agent/analyzer_intent.go:120`** 用 `OR` 语义（任一 lane true 即升级），把 `intent=explain → root_cause`、`scenario=architecture_explain → root_cause`、`is_diagnostic_question → true` 全部对齐到 diagnostic 极性（log L899）。**用户做 audit，下游被推向 RootCauseTrace 形态。**
- **`error_granularity_profile.source_quotes` verbatim 硬卡**（`internal/tool/emit_analysis.go:1820-1830`）让 LLM 撞墙 3 次（log L821 / L840 / L859），共浪费 ~30s + 3 个 prescan round。emit_analysis 第 4 次才通过。
- **`sub_topic[3].entities` 完全不过滤**（`internal/tool/emit_analysis.go:2493-2505`），4 个污染 token 进 `rm.AnalyzerHints.Entities` merged list（`internal/agent/analyzer.go:1665-1678` log L900），下游 keyword_search 拿这些做 boost candidate（`internal/tool/keyword_search.go:301/396`）。

**主判断对但被边角字段拖偏 ≠ "LLM 答错了"。LLM 的"过度配合"成本本该由系统吸收。**

### 1.2 三类形态的代码侧根因

#### 1.2.1 Entity 污染：sub_topic 不过滤

- **路径**：`internal/tool/emit_analysis.go:2493-2505` `sanitizeSubTopics()` 只清 `Summary` 字段，`Entities []string` 字段原样穿透（commit `d21ac637` 后行为不变）
- **下游放大**：`internal/agent/analyzer.go:1670-1676` 把每个 `SubTopic.Entities` 项 dedup-append 到 `rm.AnalyzerHints.Entities`
- **顶层 entities 有过滤但只覆盖 EN 单数通用名词**：`internal/tool/analysis_limits.go:370-390` 的 `GenericEntityBlocklist` 是 38 项 EN 单复数对（agent, class, config, ...）。Chinese / verbs / 仿 snake_case 概念词全部漏过
- **现有 rescue 已在做 oracle-equivalent**：`internal/tool/analysis_limits.go:646-655` 在 blocklist 命中时跑 `genericEntityVerifiedInBlob(seenBlob, norm)`，命中 prescan 输出则 keep——这是 oracle resolution 的雏形，但仅作 rescue 不作主闸

#### 1.2.2 Mirror 字段冲突：OR 语义升级

- **字段定义**：`internal/types/analysis_ir.go:583-586` 注释明示 `DiagnosticIntentProfile.IsDiagnostic is the profile-level mirror of SemanticPredicates.IsDiagnosticQuestion. Either lane being true is enough to route into diagnostic root-cause handling.`
- **Reconciler `internal/agent/analyzer_intent.go:113-160`** 入口判断 `!rm.Predicates.IsDiagnosticQuestion && !rm.DiagnosticProfile.RequiresDiagnosticRootCause()` —— `RequiresDiagnosticRootCause()` 是 `IsDiagnostic || CurrentRisk || HistoricalRegression`（`internal/types/analysis_ir.go:622-624`）。即 mirror 字段任一 true 都启动升级
- **schema validator 不检 mirror 一致性**：`internal/tool/emit_analysis.go:1226-1244` 只检 `is_diagnostic_question=true → intent must root_cause`、`current_version_check=true → 必有 diagnostic 信号`、`intent=root_cause → 必有 diagnostic 信号`。**predicates=false + profile=true 这组合是合法通过的**（log L820 emit 第 1 次就过 schema，只栽在 granularity verbatim）

#### 1.2.3 Verbatim 硬卡：source_quotes 精确匹配

- **路径**：`internal/tool/emit_analysis.go:1820-1830`，对 `error_granularity_profile.source_quotes[i]` 跑 `strings.Contains(currentRequestText, quote)`
- **失败模式**：LLM 把 directive 转写（"按是否可系统修复原则评估"）当原文，与用户实际原文（"哪些其实是可以靠系统进行修复的"）有 token 重排——精确匹配失败 → reject
- **`is_granularity_question=true` 本身是 LLM 套词**（见 §1.1 表）。但即便 LLM 正确填了 quote，转写问题也会撞墙。这是个普适的 robustness 问题，不限于本 case

### 1.3 现有先例（强烈建议复用，不要重做）

| 现有 primitive | 文件:line | 模式 | 本设计如何复用 |
|---|---|---|---|
| **`d21ac637` 静默 drop + logging.Warning** | `internal/tool/emit_analysis.go:1456-1473` | LLM 误把 current_request 当 prior_conversation 时，parser 在 `parseConversationReferenceProfile` 内部静默 drop + 自动翻 `requires_prior_context=false→true` + Warning，**零 retry** | Phase 3 mirror auto-align 直接套用这个 pattern：parser 内对齐 `diagnostic_profile.is_diagnostic` 到 `predicates.is_diagnostic_question`，Warning 写 trace |
| **`reconcileQualifiedCodeSymbolConfigDrift`** | `internal/agent/analyzer_code_symbol_reconcile.go:41-91` | Post-emit reconcile：用 typed signal（IsCrossComponent + ≥2 SubTopics）+ oracle 解析 MentionedEntities 来翻转 Intent/Scenario/Kind/AnswerSubject/Axis/Predicates。**纯 read on rm + ctx** | Phase 1 entity provenance 跑同样的"oracle + typed signal"决策，但只写 metadata，不翻转 RequestModel 主字段 |
| **`scope_projection.go::projectPrimaryScopes / projectSubTopicScopes`** | `internal/agent/scope_projection.go:56-99` | COPY 语义（不动 `Entities`，写入 `Scopes` 副线）；空匹配返回 nil 保 omitempty；单仓 posture 短路 byte-identity | Phase 1 EntityProvenance 完全沿用：不动 `Entities` 列表，写入新的 `EntityProvenance` 并行字段。空 / 单仓行为 byte-identical |
| **`finalizer_auto_repair.go` 15 函数 pattern** | `internal/orchestrator/finalizer_auto_repair.go:24-393` | extractor（`finalizerAutoRepairFacetIDs` L56-79）→ allowlist gate（`isFinalizerAutoRepairableFacet` L81-96）→ mutator（`addFacetIDsToPrincipalAnswerBlock` L120）→ re-render → caveat（`appendFinalizerRecoveredDraftCaveat` L360-369） | Phase 2 analyzer typed-cleanup registry 同构兄弟：每条"已知 LLM 自相矛盾形态"配一个 typed auto-fix 函数 |
| **`FilterFinalizerRetryRootViolations`** | `internal/orchestrator/violation_root_cause.go:161-174` | soft violation 不进 retry，走 `AppendSoftContractCaveatsToAnswer`（`internal/orchestrator/repair_caveat_materializer.go:128-130`） | Phase 4 granularity quote 软化沿用：转写问题不再 reject，转化为 typed Warning + 静默 false |
| **`ViolKindSpec.SoftByDefault / Promotable / FallbackLocus`** | `internal/types/violation_registry.go:68-83` | 每个 ViolKind 都打 typed 标签决定 soft vs hard、可否 promote、归哪个 stage | Phase 5（可选 / 长期）：analyzer-side `AnalyzerCleanupSpec` 同构 |
| **`RequiredFileHint []Struct`** | `internal/types/analysis_ir.go:773-788` | `AnalyzerHints` 上已有的 struct 副线（Path / Confidence / Rationale），与 `Entities []string` 平级 | Phase 1 EntityProvenance 加在同一 struct，平级追加 |
| **`AnswerSubject 自动 fallback`** | `internal/tool/emit_analysis.go:334` schema desc + `internal/agent/analyzer_intent.go:285-311` `inferAnswerSubject()` | schema 描述 `"Leave unset when ambiguous; an automatic fallback infers from question_kind"`——已有"系统侧 derive，LLM 不必精填"的先例 | Phase 5 schema 文案修订沿用此措辞惯例 |

### 1.4 红线（必须遵守，违反 → 方案被拒）

| 编号 | 红线 | 源 | 在本设计中的体现 |
|---|---|---|---|
| **R1** | 精确信号才能驱动硬 gate；嘈声信号只能驱动软引导 | `docs/architecture.md` §1.5 | 所有新增 decision point 都基于 typed boolean / enum / oracle 真值；任何 grep-count / 相似度 / heuristic 仅作 noise_score 软信号 |
| **R2** | 系统侧硬 gate 必须给模型 typed escape | `docs/architecture.md` §1.6 | 新增 reject 必须配 typed waiver 字段；mirror auto-align 不算硬 gate（precedence 明确，无歧义可表态） |
| **R3** | LLM 只在边界出现，中间数据全是 typed struct | `docs/architecture.md` §1.2 | EntityProvenance 是纯 typed struct；不引入新的 prose 解析；下游消费走字段而非字符串匹配 |
| **R4** | 读模式 byte-preserved；read pipeline 不感知 write-mode 概念 | `docs/architecture.md` §1.4 | write_analyzer 路径独立 `WriteAnalysisIR`（`internal/types/write_analysis_ir.go:23-29`）；本设计零碰 write 路径 |
| **R5** | Fail-loud 而非静默兜底 | `docs/architecture.md` §1.3 | 仅"小不一致 + precedence 明确"才走 auto-align；degenerate / 必填字段缺失依旧 fail-loud；Warning 必入 trace log |
| **R6** | 不允许内部 pipeline 术语泄漏给 LLM | `feedback_no_internal_info_in_llm_prompts.md` | EntityProvenance 不进 LLM-facing prompt 字段；Schema 文案只描述用户语义（"entities 可以是答案形态词或代码符号"），不暴露 "use_for_search" / "noise_score" 这类内部术语 |

---

## 2. 设计原则（5 条，从用户原话 + 红线 + 现有先例推出）

> 用户原话（2026-05-18）：*"在 finalyzer 阶段要尽量让系统去自动修复，小错误由系统软化提醒，加补充说明即可，只有系统无法修复的严重错误才让 LLM 返工"*

把这条原则按 analyzer 上下文展开为 5 条工作准则：

### P1 — Entity 永不移除，只打标

`Entities []string` 是用户意图与 LLM 判断的混合体。系统职责是给每个 token 加 typed metadata 让下游消费方分流，**不是替用户决定哪些 token 不该出现**。

→ Phase 1 EntityProvenance 用 COPY 语义（沿用 `scope_projection.go:21-27` 模板）。`AnalyzerHints.Entities` / `SubTopic.Entities` 字节级不变。

### P2 — Mirror / 副本字段冲突按 precedence 静默对齐

当两个 schema-级声明的 mirror 字段冲突时（schema 注释明示 mirror 关系，如 `analysis_ir.go:583-586`），系统按 precedence 选一个权威源，另一个静默对齐 + Warning。**不算硬 gate**（R2 红线"自相矛盾不该用 escape"在 §1.6 第 95 行明确：模型自相矛盾不该用 escape 跳过——但 mirror 类是 precedence 明确而非歧义，归 auto-align 不归 escape）。

→ Phase 3 新增一个 post-parse typed normalizer（同时拿到 `predicates` 与 `diagnostic_profile` 后再处理），参照 `d21ac637` 的 `parseConversationReferenceProfile` 写法记录 Warning。不要在 `parsePredicates` 或 `parseDiagnosticProfile` 单侧读取另一侧，避免 parse order 变成隐性约束。

### P3 — Optional typed profile 的硬卡退化为软清空

`error_granularity_profile` 等 "advisory typed lane"（schema 描述本来就强调可选）不该让 LLM 撞墙重试。verbatim 之类的精度问题 → 系统做确定性 normalization（空白 / 标点 / 全半角等可逆规范化）后仍无法锚定时，静默清空字段 + Warning，profile 主 boolean 自动翻 false。不要用短子串命中作为保留 profile 的正信号。

→ Phase 4 软化 `internal/tool/emit_analysis.go:1820-1830`。

### P4 — 黑名单和 oracle 解析是正交两轴

- **解析度** (resolution)：token 是否对应仓内真实 code thing？— 用 `oracle.SymbolExistsFlat` / `graph.FileIndex` / `seenBlob` 命中决定。
- **噪声度** (noise)：token 作为搜索 anchor 会产生多少误命中？— 用 grep-match-count 或 blocklist 命中决定。

四象限里"高解析+高噪声"（如 `agent` 同时是真 type 名 + 通用词）和"低解析+高噪声"（连接词）都存在，单轴方案永远漏一边。两轴独立打标，下游消费方按需读。

→ Phase 1 EntityProvenance 同时记录两个字段；现有 `GenericEntityBlocklist` 保留但角色降为 noise_score 的快速近似（hit → 0.9）。

### P5 — 新增 hard reject 前必须先问"能否 typed cleanup？"

这是本设计的唯一红线。已有 reject 路径不轻易动；任何新的"LLM 输出形态不合预期"诉求，第一选择是 typed-cleanup function（参考 `finalizer_auto_repair.go` 的 15 函数 pattern），第二选择是并行 metadata lane（参考 `scope_projection.go`），最后才是新增 reject。Reject 必须配 typed waiver。

→ Phase 2 建立 analyzer 端 typed-cleanup 入口表（registry）；后续新形态发现一个加一条，不让任何一条进 retry 通道。

---

## 3. 设计：典型数据模型与决策树

### 3.1 EntityProvenance 数据模型

新增类型，位置 `internal/types/analysis_ir.go`（紧邻 `RequiredFileHint` L773 之后）：

```go
// EntityProvenance is the typed side-lane describing where each
// AnalyzerHints.Entities / SubTopic.Entities token came from and how
// downstream consumers should treat it. The Entities []string slice
// stays the source of truth for the LLM's emit; this lane only adds
// typed advisories without subtracting from existing flows.
//
// COPY semantics: written by post-emit projection
// (internal/agent/entity_provenance_projection.go::projectEntityProvenance),
// not edited by the LLM. Same posture as PrimaryScopes /
// SubTopic.Scopes in scope_projection.go.
//
// Single-repo / no-prescan posture: empty slice (omitempty preserves
// JSON byte-identity for legacy snapshots).
//
// Schema-private: this struct is NEVER described in any emit_*
// schema; LLMs do not see these field names. R6 red line.
type EntityProvenance struct {
    // Token is the entity surface as the LLM wrote it. Casing /
    // punctuation preserved verbatim for trace audit.
    Token string `json:"token"`

    // Origin classifies the LLM-provided semantic role of the token.
    // Inferred from typed signals (matched against MentionedEntities /
    // prescan seenBlob / oracle / FileIndex), never from raw request
    // text scan.
    Origin EntityOrigin `json:"origin"`

    // Resolved is true iff the token resolves to a real repo thing
    // (oracle.SymbolExistsFlat OR FileIndex.LookupBasename OR
    // seenBlob substring with token-boundary). This is the
    // PRECISE binary signal (R1 red line).
    Resolved bool `json:"resolved"`

    // NoiseScore is the heuristic noise estimate in [0,1]; 0 = unique
    // distinctive token, 1 = generic-noun-equivalent or blocklist hit.
    // Computed as 1 - 1/(1+seenBlob_hits) capped, or 0.9 for blocklist
    // hits short-circuit. NOISY signal (R1 — soft guidance only).
    NoiseScore float64 `json:"noise_score,omitempty"`

    // UseForSearch is the system's advisory verdict: should this token
    // be used as a keyword_search/grep anchor downstream? Default
    // policy: Resolved && NoiseScore < 0.5. Consumer may override.
    UseForSearch bool `json:"use_for_search"`

    // UseForShape is the system's advisory verdict: should this token
    // drive sub_topic title rendering / ERM category seeds? Default
    // policy: true unless Origin == OriginRetryHintLeak.
    UseForShape bool `json:"use_for_shape"`
}

type EntityOrigin string

const (
    // OriginPrescanAnchor: token matched a prescan grep / FileIndex hit
    // — typical for ordinary code entities (function name, type name,
    // file basename).
    OriginPrescanAnchor EntityOrigin = "prescan_anchor"

    // OriginOracleSymbol: token resolved via oracle.SymbolExistsFlat —
    // canonical code symbol, even if not seen in prescan blob.
    OriginOracleSymbol EntityOrigin = "oracle_symbol"

    // OriginUserVerbatim: token appears verbatim in rm.RawRequest after
    // NormalizeCodeKey — user-visible vocabulary even if not yet a
    // repo symbol (legitimate for design-discussion / future-target
    // entities). Computed via TermGraph LLM-objective surfaces.
    OriginUserVerbatim EntityOrigin = "user_verbatim"

    // OriginInferredConcept: token did not resolve via any of the above —
    // most likely an LLM-inferred answer-shape word (清单 / keep /
    // remove / system_fix). NOT a system error; legitimate as a
    // sub_topic shape hint.
    OriginInferredConcept EntityOrigin = "inferred_concept"

    // OriginRetryHintLeak: token EQUALS a known emit_analysis field
    // name from the retry-hint sanitizer's leak set (is_cross_component,
    // sub_topics, primary_entities, …). This IS a system-detectable
    // structural error — the only origin that defaults UseForShape=false.
    OriginRetryHintLeak EntityOrigin = "retry_hint_leak"
)
```

Wire 字段挂载（`internal/types/analysis_ir.go:649-743` `AnalyzerHints` 内部）：

```go
type AnalyzerHints struct {
    // ... existing fields ...

    // EntityProvenance is the typed side-lane for Entities. One
    // entry per Entities[] token in same order. Empty when no
    // prescan/oracle was available. SEE: type EntityProvenance.
    //
    // Read-mode produced by analyzer post-emit projection
    // (internal/agent/entity_provenance_projection.go). NEVER
    // serialised into LLM-facing prompts.
    EntityProvenance []EntityProvenance `json:"entity_provenance,omitempty"`
}
```

`internal/types/analysis_ir.go:844-858` `SubTopic` 内部相同字段（per-sub-topic 副本）。

### 3.2 黑名单 vs Oracle 解析的正交关系（详细论证）

P4 准则的代码级证明：

| Token | Blocklist 命中？ | Oracle 解析？ | seenBlob 命中？ | 当前行为 | 设计后行为 |
|---|---|---|---|---|---|
| `finalizer_auto_repair` | ❌ | ✓ | ✓ | keep | `Origin=OriginOracleSymbol`, `UseForSearch=true`, `UseForShape=true` |
| `agent` | ✓ | ✓ | ✓ | keep（rescue） | `Origin=OriginOracleSymbol`, `NoiseScore=0.9`, `UseForSearch=false`（高噪），`UseForShape=true` |
| `清单` | ❌ | ❌ | ❌ | keep（黑名单漏过！）| `Origin=OriginInferredConcept`, `UseForSearch=false`, `UseForShape=true` |
| `keep` | ❌ | ❌ | ❌ | keep（黑名单漏过！）| `Origin=OriginInferredConcept`, `UseForSearch=false`, `UseForShape=true` |
| `system_fix` | ❌ | ❌ | ❌ | keep（黑名单漏过！）| `Origin=OriginInferredConcept`, `UseForSearch=false`, `UseForShape=true` |
| `is_cross_component` | ❌ | ❌ | ❌（理论）| keep（黑名单漏过！retry 用） | `Origin=OriginRetryHintLeak`, `UseForSearch=false`, `UseForShape=false`（唯一会被下游忽视的 origin） |
| `count` | ✓ | ❌ | ❌ | drop | `Origin=OriginInferredConcept`, `NoiseScore=0.9`, `UseForSearch=false`, `UseForShape=true` |
| `gorilla/mux` | ❌ | ❌（外部库） | ❌ | keep | `Origin=OriginUserVerbatim`（用户原文有）, `UseForSearch=false`（不在仓内 grep）, `UseForShape=true` |
| `TaskGraph` | ❌ | ✓（types/task_graph.go） | ✓ | keep | `Origin=OriginOracleSymbol`, `UseForSearch=true`, `UseForShape=true` |

观察：
- 当前行为里**任何 token 都不会被 drop 除非命中 blocklist 且 prescan 没见过**
- 设计后**任何 token 也都不会 drop**（含 `count` 这种黑名单命中的）—— **黑名单只是 NoiseScore 的快速近似，不再 drop**
- 唯一会让下游真正忽视的 origin 是 `OriginRetryHintLeak`（structural error，已知 LLM 形态错误，参考 `retry_hint_sanitize.go` 的术语集）

### 3.3 三类形态的决策树

#### 3.3.1 Entity 污染（Phase 1+2）

```
LLM emit entities + sub_topics[i].entities
  │
  ├── parseEntities (emit_analysis.go ~L733): trim only, no filter
  ├── sanitizeSubTopics (emit_analysis.go:2493): only summary cleaned
  │
  ▼ (after analyzer.go:1665-1676 sub-topic merge into AnalyzerHints.Entities)
  │
projectEntityProvenance (NEW, internal/agent/entity_provenance_projection.go):
  for each token in (AnalyzerHints.Entities ∪ SubTopic.Entities):
    1. Resolved = oracle.SymbolExistsFlat(token)
                || graph.FileIndex.LookupBasename(token)
                || seenBlob.contains(token, word-boundary)
    2. UserVerbatim = TermGraph.surfaceCanon(rm.RawRequest).contains(NormalizeCodeKey(token))
    3. RetryHintLeak = retry_hint_sanitize.knownLeakSet().contains(token)
    4. NoiseScore = blocklist_hit ? 0.9 : min(0.9, hits_in_seenBlob / hits_threshold)
    5. Origin = first-match precedence:
       RetryHintLeak > OracleSymbol > PrescanAnchor > UserVerbatim > InferredConcept
    6. UseForSearch = Resolved && NoiseScore < 0.5 && Origin != RetryHintLeak
    7. UseForShape = Origin != RetryHintLeak
  │
  ▼ write to AnalyzerHints.EntityProvenance + SubTopic.EntityProvenance
  │
Downstream consumers (8 sites — see §1.4 audit + Phase 2 list):
  - keyword_search opts.Entities → filter by UseForSearch
  - explorer ERM evidence_closure → use Resolved as required-proof signal
  - ranker repoMapRank → score boost weighted by Resolved + 1/NoiseScore
  - sub_topic title renderer → no change (keep using all entities)
```

#### 3.3.2 Mirror 字段冲突（Phase 3）

```
LLM emit predicates.is_diagnostic_question = X
LLM emit diagnostic_profile.is_diagnostic   = Y
  │
  ▼ normalizeDiagnosticMirrorSignals(predicates, diagnostic_profile), after both parsers
  │
mirror_auto_align:
  if X == Y:
    no change                          # consistent, pass through
  else if Y == true && current_risk == false && historical_regression == false:
    # profile.is_diagnostic=true but predicates says false AND no
    # independent strong signal → align profile to false
    out.IsDiagnostic = X (false)
    logging.Warning("[emit_analysis] mirror auto-align: diagnostic_profile.is_diagnostic %v→%v to match predicates.is_diagnostic_question", Y, X)
  else if Y == false && X == true:
    # predicates says yes, profile says no → align profile to true
    out.IsDiagnostic = X (true)
    logging.Warning(...)
  else if Y == true && (current_risk == true || historical_regression == true):
    # independent strong signal active → flip predicate to true
    # (this is the rare "LLM forgot to set predicate but set the profile right" case)
    # — handled inside the post-parse normalizer with both structs visible
    [predicates.IsDiagnosticQuestion → true with Warning]
  else if current_version_check == true:
    # current_version_check alone is not diagnostic. It only becomes
    # current-status diagnostic when paired with is_diagnostic/current_risk/
    # historical_regression; never let it promote a config/exact lookup.
    no promotion
  │
  ▼ validateSelfConsistency now sees consistent pair → no new reject
  │
  ▼ reconcileDiagnosticQuestionProfile (analyzer_intent.go:120) input is now
     coherent. Tighten its gate to single-signal: predicates only
     (current_risk / historical_regression already pre-aligned by parser).
```

#### 3.3.3 Verbatim 硬卡（Phase 4）

```
LLM emit error_granularity_profile.is_granularity_question = true
LLM emit error_granularity_profile.source_quotes = [Q1, Q2]
  │
  ▼ parseErrorGranularityProfile (emit_analysis.go:1800-1832)
  │
current behavior (L1825):
  for each Q in quotes:
    if not strings.Contains(rm.RawRequest, Q):
      REJECT → LLM retry
  │
new behavior:
  for each Q in quotes:
    match = strings.Contains(rm.RawRequest, Q)
    if !match:
      match = normalizedExactMatch(rm.RawRequest, Q)  # whitespace/punct/fullwidth normalization
    if !match:
      keep := false (drop this quote)
      log.Warning
  collected = all matched quotes
  if len(collected) == 0:
    # no anchorable quote — softly downgrade entire profile
    out.IsGranularityQuestion = false
    out.SourceQuotes = nil
    logging.Warning("[emit_analysis] error_granularity_profile auto-softened: no verbatim source_quote found in current request; downgrading is_granularity_question=true→false")
  else:
    out.SourceQuotes = collected
  │
  ▼ no LLM-facing rejection ever for this field
```

---

## 4. Phased 实施计划

每个 Phase = 一个独立可 ship 的 commit cluster。Phase 间无强依赖（除 Phase 2 消费方必须在 Phase 1 producer 之后），可分 session 落地。

### Phase 1 — EntityProvenance Producer（最高 ROI，独立 session）

**目标**：建立 `EntityProvenance` 数据模型 + 唯一 producer。下游消费方暂不接，验证 zero-regression（产生的 metadata 不被任何人读，等同 dead code）。

**Files / changes**：

| 文件 | 操作 | 描述 |
|---|---|---|
| `internal/types/analysis_ir.go` | `+~80 LOC` | 新增 `EntityProvenance` struct + `EntityOrigin` enum + const 块（紧邻 `RequiredFileHint` L773） |
| `internal/types/analysis_ir.go` | `+1 LOC` | `AnalyzerHints` 加 `EntityProvenance []EntityProvenance`（line ~743 后） |
| `internal/types/analysis_ir.go` | `+1 LOC` | `SubTopic` 加 `EntityProvenance []EntityProvenance`（line ~857 后） |
| `internal/agent/entity_provenance_projection.go` | 新文件 `+~180 LOC` | `projectEntityProvenance(ctx, rm)` — 完全沿用 `scope_projection.go` 模板（COPY 语义、单仓短路、omitempty 保 byte-identity） |
| `internal/agent/analyzer.go:1758-1769` 之间 | `+2 LOC` | 调用 `projectEntityProvenance(ctx, &rm)` (after `projectSubTopicScopes`)，保证它在最末尾跑（subTopics 已 merged 入 Entities） |
| `internal/agent/retry_hint_sanitize.go` | `+~10 LOC` | 导出 `KnownLeakSet() map[string]struct{}` 让 projection 引用（当前 leak 词表在 L78-155 regex 里，需要抽出为 const map） |
| `internal/agent/entity_provenance_projection_test.go` | 新文件 `+~250 LOC` | 表驱动测试覆盖 §3.2 表中 9 个 token case + 空-prescan + 单仓 + multi-repo 路径 |

**Eval bar**：
- 全集 58 case × 1 round ZERO regression（`AnalyzerHints.EntityProvenance` 不被任何 downstream 读，差异只是 JSON 多出 omitempty 字段；如有任何 case 看 JSON 字段输出有断言，更新 fixture）
- 触发 case（finalyzer-retry-audit）跑通：log 应该看到 `[analyzer] entity provenance: prescan_anchor=X oracle_symbol=Y user_verbatim=Z inferred_concept=W retry_hint_leak=0` 一行

**风险**：低。新增 typed lane，无现有 reader。仅 multi-topic merged Entities 之后跑一次纯函数。

**Commit 拆分**（建议 4 commits）：
1. types: introduce EntityProvenance struct
2. agent: export retry_hint_sanitize known leak set
3. agent: projectEntityProvenance projection (no callers)
4. agent: wire projectEntityProvenance into analyzer.go post-process

---

### Phase 2 — Consumer 接线 + typed-cleanup registry（中 ROI，独立 session）

**目标**：把 Phase 1 产生的 metadata 接到下游消费点，让 `清单 / keep / remove` 等 token **不再驱动 keyword_search 但保留 sub_topic 形态信号**。

**Files / changes**（基于 Explore agent 已审计的 8 个消费点）：

| 文件 | 操作 | 描述 |
|---|---|---|
| `internal/agent/keyword_search.go:301` | 改 ~5 LOC | `repoMapRank(keywords, opts.Entities, ...)` 改为先 filter `opts.Entities` by `EntityProvenance.UseForSearch==true`。如果 ctx 没 provenance（旧 caller / 测试），全 keep（向后兼容） |
| `internal/agent/keyword_search.go:396` | 改 ~5 LOC | `entityBoostFactor` 同样 filter |
| `internal/agent/keyword_search.go:512` | 改 ~3 LOC | `anchorEntities` rescue path 同样 filter |
| `internal/agent/explorer.go:5683` `evidenceMatchesRequirementEntities` | 改 ~5 LOC | ERM evidence closure 的 hard shape count 先用 `UseForShape==true` 过滤；被标为 unresolved/inferred 的实体不再要求证据强制匹配 |
| `internal/agent/explorer.go:1402,2518,3719,3818,4178` | 不动 | ERM 收集所有 entity 到 flat map —— 保留；filter 发生在 5683 的 closure check |
| `internal/agent/explorer.go:9528` `entityBias dataflow re-rank` | 改 ~3 LOC | 只用 `UseForSearch==true` 的做 bias（其余不进 grep 路径无意义） |
| 其他 5 个消费点（analyzer.go:1367/1417/1442/1672/1758/15182/14658/559） | 不动 | 这些是 producer / 内部展开 / 计数器，不读 metadata |
| `internal/agent/entity_provenance_filter.go` | `+~70 LOC` | 新增 helper `filterEntitiesByProvenance(entities []string, prov []EntityProvenance, role string) []string`，role ∈ {"search", "shape"}，nil prov fallthrough；单个 entity 缺 provenance 也 fail-open |
| `internal/orchestrator/analyzer_cleanup_registry.go` | **DEFER** | 空 registry 不交付。当前 cleanup 形态不足以证明抽象收益；按 §6.4，等 cleanup spec ≥10 个再抽象，避免重复造轮子 |

**Eval bar**：
- 全集 58 case × 1 round：keyword_search 行为差异允许（更少的噪声 token 进 grep），但 evidence closure / 答案正确率 ZERO regression
- 触发 case：log 应看到 `[keyword_search] entities pre-filter=10 post-filter=6 (dropped 4 inferred_concept tokens)` 一行；最终答案不再被 `清单 / keep / remove` 拖去搜索全仓
- m1a + s1a + qf_arch × 4 跑：finalizer retry 次数不应增加

**风险**：中。涉及 keyword_search 和 explorer ERM。需要确保 nil-provenance 路径（旧测试 / 单测 fixtures）byte-identical。

**Commit 拆分**（建议 5 commits）：
1. agent: introduce filterEntitiesByProvenance helper + tests
2. tool: keyword_search filter entities by UseForSearch
3. agent: explorer ERM evidence_closure soft-skip on unresolved
4. agent: explorer dataflow entityBias filter
5. docs: mark analyzer_cleanup_registry deferred until cleanup volume justifies abstraction

---

### Phase 3 — Diagnostic Mirror Auto-Align（中 ROI，独立 session）

**目标**：消除 `diagnostic_profile.is_diagnostic` vs `predicates.is_diagnostic_question` 冲突时的 reconciler 升级误伤。沿用 `d21ac637` 的"parser silently align + Warning" 模式。

**Files / changes**：

| 文件 | 操作 | 描述 |
|---|---|---|
| `internal/tool/emit_analysis.go` | `+~60 LOC` | 新增 `normalizeDiagnosticMirrorSignals(preds, diag)`，在 `parsePredicates` 与 `parseDiagnosticProfile` 都完成后调用；normalizer 才能同时读取两边，避免 parse-order 隐性依赖 |
| `internal/tool/emit_analysis.go` call site | 改 ~10 LOC | 将 normalized `predicates` / `diagnostic_profile` 写回 payload 变量，再进入 `validateSelfConsistency` |
| `internal/agent/analyzer_intent.go:119-160` `reconcileDiagnosticQuestionProfile` | 改 ~5 LOC | 入口 `!rm.Predicates.IsDiagnosticQuestion && !rm.DiagnosticProfile.RequiresDiagnosticRootCause()` 收紧为 `!rm.Predicates.IsDiagnosticQuestion`。**注释解释**：parser-time mirror align 已保证两边一致，OR 形式冗余 + 残留误升级风险 |
| `internal/tool/emit_analysis_test.go` | `+~150 LOC` | 表驱动测试覆盖 4 种 mirror 组合 × 独立信号 on/off |

**Eval bar**：
- 全集 58 case × 1 round ZERO regression（特别关注：诊断类 case s1a / qf_perf_bottleneck / qf_panic_*）
- 触发 case：log 应看到 `[emit_analysis] mirror auto-align: diagnostic_profile.is_diagnostic true→false`，且 `[analyzer] diagnostic profile reconciled` 这行 **不应** 再出现（因为 mirror 已对齐，reconciler 入口条件不满足）
- m1a × 4 / s1a × 4 跑：accept rate 不下降

**风险**：中-高。涉及 diagnostic 主决策路径。重点 watch perf_triage / log_triage 联动（它们可能依赖 reconciler 自动升级 intent，需要确认不依赖）。

**Commit 拆分**（建议 4 commits）：
1. tool: add normalizeDiagnosticMirrorSignals (predicate-wins unless independent strong signal wins)
2. tool: wire diagnostic mirror normalization before self-consistency validation
3. agent: tighten reconcileDiagnosticQuestionProfile entry condition
4. tests: cover 4 mirror combinations × 2 strong-signal states

---

### Phase 4 — Granularity Quote 软化（高 ROI 且低风险，可与 P3 同 session）

**目标**：`error_granularity_profile.source_quotes` 转写 / 标点差异不再触发 LLM 重试。

**Files / changes**：

| 文件 | 操作 | 描述 |
|---|---|---|
| `internal/tool/emit_analysis.go:1820-1830` | 改 ~40 LOC | verbatim match 失败 → 跑 deterministic normalized exact match（空白 / 全半角 / 标点规范化）→ 全失败则静默 drop quote + Warning；所有 quote drop 后 IsGranularityQuestion 自动 false + Warning |
| `internal/tool/emit_analysis.go` 添加 helper | `+~30 LOC` | `normalizeForSourceQuoteMatch(s string) string`；不实现短子串通过逻辑 |
| `internal/tool/emit_analysis_test.go` | `+~80 LOC` | 表驱动覆盖：精确匹配 / 规范化匹配 / 全失败软化；短子串命中不得保留 profile |

**Eval bar**：
- 全集 58 case × 1 round：使用 granularity profile 的 case（u11b / m1b 等）行为不变
- 触发 case：emit_analysis 调用次数应从 4 降到 1

**风险**：低。仅放松校验，不引入新行为。

**Commit 拆分**（建议 2 commits）：
1. tool: normalized source-quote matching helper
2. tool: parseErrorGranularityProfile softened quote matching

---

### Phase 5 — Schema 描述文案 + 黑名单角色降级（延后到 telemetry 后）

**目标**：让 LLM 看到的 schema 描述真实反映系统能力（"系统会自动 derive，你不必精填"），同时把 `GenericEntityBlocklist` 从 drop 路径降级为 NoiseScore 快速近似。

**2026-05-19 修订**：Phase 5 不再建议先跑。Schema 文案已随 Batch 4 小步同步到已交付能力；`GenericEntityBlocklist` 的 drop→noise 降级必须等 EntityProvenance telemetry 跑过 forensic/eval 后再做。否则在 provenance consumer 未接线前改变 entity 流，会把一个"标注系统"问题变成 search/shape 噪声扩大问题。Batch 5 已先补一条 shadow telemetry：当前仍按旧逻辑 drop，但日志会记录这些被 drop 的 generic entity 在未来降级路径下默认会被标为 `inferred_concept` 且 `use_for_search=false/use_for_shape=false`，供真实日志证明安全性。

**Files / changes**：

| 文件 | 操作 | 描述 |
|---|---|---|
| `internal/tool/analysis_limits.go:633-690` `filterGenericEntitiesWithWhitelist` | **DEFER** | 不再 drop，全部 keep；返回值改为带 NoiseScore 的 tagging 结构。**延后到 EntityProvenance telemetry 证明安全后再做** |
| `internal/tool/analysis_limits.go:502-560` `validateAnalysisInput` | **DEFER** | DroppedEntities 永远空（保留 struct field 兼容旧测试）。同样延后到 blocklist 降级落地时修改 |
| `internal/tool/analysis_limits.go` `BlocklistShadow` | **DONE** | 保持现有 drop 行为；对 `DroppedEntities` 生成 log-only shadow metadata，不进入 `Warnings`，不改变 `FilteredEntities` |
| `internal/tool/emit_analysis.go` `blocklist_shadow` logging | **DONE** | `logging.Info("[emit_analysis] %s", val.BlocklistShadowSummary)`，便于 forensic/eval grep；不污染 REPL 分析结果 |
| `internal/tool/emit_analysis.go:344-357` `entities` schema desc | 改 ~5 LOC | 追加："entities can be code symbols, file paths, OR answer-shape vocabulary the user explicitly asked about (categories, outcomes, comparison axes). The system classifies provenance automatically." |
| `internal/tool/emit_analysis.go:344-358` `predicates / diagnostic_profile` schema desc | 改 ~10 LOC | 在 `is_diagnostic_question` 和 `is_diagnostic` 描述末尾追加：" These two fields are mirrors; the system auto-aligns the profile copy to match the predicate. Always set both consistently, but the system tolerates mirror drift." |
| `internal/tool/emit_analysis.go:473` `is_granularity_question` schema desc | 改 ~5 LOC | 追加："The system normalizes source_quotes against the current request; exact verbatim is preferred, and unanchored optional quotes may be ignored instead of retried." |

**Eval bar**：
- 全集 58 case × 1 round：DroppedEntities 测试 fixture 全部为空数组（更新 expected）；其他 ZERO regression

**风险**：中。Schema 文案不破 wire format；但 blocklist drop 行为变化会影响 downstream 候选集合，必须建立在 EntityProvenance telemetry + consumer fallback 已验证的基础上。本批不改 blocklist 行为。

**Commit 拆分**（建议 3 commits）：
1. tool: blocklist downgrade to NoiseScore fast-path (no drop)
2. tool: update entities + diagnostic_profile schema descriptions
3. tool: update error_granularity schema description

---

## 5. Backward Compat 与 Cross-mode 考量

### 5.1 Read mode byte-identity (R4 红线)

- `EntityProvenance []EntityProvenance` 用 `json:"entity_provenance,omitempty"` — 空 slice 序列化为缺字段，与未升级 baseline JSON 兼容
- 单仓 + 无 prescan posture：projectEntityProvenance 返回 nil（沿用 `scope_projection.go:30` 短路模式），AnalysisIR JSON 与升级前 byte-identical
- Phase 5 blocklist 降级：`DroppedEntities []string` 字段保留（兼容测试），但永远为空。不破 ABI

### 5.2 Write mode 路径（R4 红线）

- `WriteAnalysisIR`（`internal/types/write_analysis_ir.go:23-29`）独立类型，与 `AnalysisIR` 零字段重叠
- `write_analyzer` agent 跑 `emit_write_analysis`（独立 tool），不走 `emit_analysis` —— Phase 3/4 修改对 write mode 完全无影响
- Phase 1 `projectEntityProvenance` 仅在 `internal/agent/analyzer.go::buildAnalysisIR` 内调用，buildAnalysisIR 不在 write 路径
- **验证手段**：跑现有 write-mode test suite (`internal/agent/write_analyzer_test.go` 等) ZERO change

### 5.3 Multi-repo 路径

- `EntityProvenance.Resolved` 检查 oracle 时通过 `repomap.MultiGraphFromAgentContext(ctx).Oracle()`（沿用 `analyzer.go:717-722 analyzerOracleFromCtx` 现有 helper）
- Multi-repo active set 在 oracle/FileIndex 路径内已正确收口，本设计不引入新决策
- 单仓 fallback 使用 `repomap.NewSymbolOracle(graph)` —— byte-identical 与现有 reconcileQualifiedCodeSymbolConfigDrift 调用模式

### 5.4 Prescan empty / cold-start

- 无 prescan blob 场景（analyzer 跑 0 round prescan）：seenBlob == ""，`OriginPrescanAnchor` 永不命中
- oracle 仍可工作（graph 已在 startup 加载）
- 结果：所有未在 oracle 中的 token 落入 `OriginUserVerbatim` 或 `OriginInferredConcept`
- `UseForSearch` 自然变 false（NoiseScore 0 但 Resolved false）—— 与现有 cold-start 行为差异：现在这些 token 进 keyword_search，未来不进。**对 cold-start 行为有可见影响**，需 Phase 2 单测覆盖

---

## 6. Open Questions

### 6.1 `OriginUserVerbatim` 触发条件用 TermGraph 还是 typed request surfaces？

TermGraph (`internal/types/analysis_ir.go:860-` 起的注释) 是 normalizer 的 canonical 表，但仅在 `analyzer.go:1703 Normalize()` 之后填充。`projectEntityProvenance` 跑在 normalize 之后（`analyzer.go:1758` 之后），所以 TermGraph 可用。

**建议**：第一版不要用 raw request substring 驱动任何消费决策。`OriginUserVerbatim` 若要落地，只能作为 telemetry/advisory origin；search/shape consumer 仍以 prescan/oracle/typed hints 为准。若确需更准的用户显式性，优先用已构建的 TermGraph 或 analyzer typed request surfaces，而不是重新扫描用户问题文本。

### 6.2 `NoiseScore` 阈值 0.5？

`UseForSearch = Resolved && NoiseScore < 0.5` 中的 0.5 是猜的。需要 Phase 1 上线后用 telemetry log 跑分布看真实分布。

**建议**：用 `internal/config/runtime.go` 暴露 yaml 配置（`analyzer_noise_threshold_for_search: 0.5`），cli flag 不暴露（不在 §10 已批准的 cli override 清单内）。

### 6.3 `OriginRetryHintLeak` 是否需要硬 reject？

R5 红线说"fail-loud"。Retry hint leak 是已知 LLM 形态错误（不是数据丢失），但每次 leak 都浪费一次 emit。

**建议**：Phase 2 实施时先 UseForShape=false + Warning（软处理），跑一个月看真实频率，若 >2% 再升级为 reject。这与 `d21ac637` 的渐进式态度一致。

### 6.4 Phase 6 后续 — `AnalyzerCleanupSpec` typed registry

`finalizer_auto_repair.go` 的 15 函数 + `ViolKindSpec` typed lane 共同实现了 finalizer 的 cleanup registry。Analyzer 端目前只 5-8 个 cleanup 形态，独立 registry 是否值得？

**建议**：留待 Phase 1-5 SHIPPED 三个月后审。若届时已积累 ≥10 个 cleanup spec，再做 registry 抽象；否则就让它们散在各 parse* 函数里（参考 `parseConversationReferenceProfile` 的 inline 写法）。

---

## 7. Eval bar 与回归红线

### 7.1 全局 Eval bar（任意 Phase 必须满足）

- **58 case × 1 round** 全集回归测试：accept rate 不下降
- **触发 case** (finalyzer-retry-audit) **× 4 round**：(1) emit_analysis 调用次数 ≤ 2（vs baseline 4） (2) 最终答案不再 mention diagnostic root_cause 主线 (3) 答案保留 "建议保留/移除的 retry 清单" 形态
- **m1a / s1a / qf_arch / qf_config_precedence × 4 round**：accept rate 不下降，retry 次数不增加

### 7.2 Per-phase 红线

- **Phase 1 producer**：JSON 输出（AnalysisIR 序列化）byte-identical 与 baseline，除非 fixture 显式期待新 `entity_provenance` 字段
- **Phase 2 consumer**：keyword_search 命中 file 集合允许变（更少噪声），但 ERM evidence closure 触发的 retry 集合 ZERO change
- **Phase 3 mirror align**：诊断类 case（s1a / qf_panic / qf_perf）accept rate 不下降；非诊断类 case（qf_arch / qf_config）reconciler 不再误触发（log `diagnostic profile reconciled` 不出现）
- **Phase 4 quote 软化**：使用 granularity 的 case（u11b / m1b）行为 byte-identical（已 match 的继续 match）
- **Phase 5 blocklist 降级**：`DroppedEntities` 在所有测试 fixture 中应为空 slice（fixture 需同步更新）

### 7.3 Telemetry 入口

Phase 1 引入的 logging.Info 行（per buildAnalysisIR 一行）：

```
[analyzer] entity provenance summary: total=N prescan_anchor=A oracle_symbol=B user_verbatim=C inferred_concept=D retry_hint_leak=E use_for_search=F use_for_shape=G mean_noise=H
```

后续 Phase 通过这条行即可 grep 验证行为分布。

---

## 8. Work Breakdown

| Phase | Session | Commits | LOC（净增） | 风险 | Eval 触发 |
|---|---|---|---|---|---|
| Phase 4 (granularity quote 软化) | Session 1 | 2 | ~120 | 低 | 触发 case retry count |
| Phase 3 (diagnostic mirror align) | Session 1 | 3 | ~180 | 中 | 全诊断类 case |
| Phase 1 (EntityProvenance producer) | Session 2 | 4 | ~440 | 低 | 触发 case telemetry only |
| Phase 2 (consumer wiring + cleanup registry) | Session 3 | 5 | ~280 | 中 | 全 58 case |
| Phase 5 (blocklist downgrade + schema) | Session 4 | 3 | ~60 | 中 | DroppedEntities fixture |

**总计**：4 sessions, ~17 commits, ~1080 LOC（含测试），4 类 eval gate

**建议落地顺序**：Phase 4 → Phase 3 → Phase 1 → Phase 2 → Phase 5

**为什么这个顺序**：
- Phase 4/3 先：它们直接命中 forensic case 的重试与 root_cause 误路由，改动最小且不扩大 entity 候选集合
- Phase 1 producer 先于 Phase 2 consumer：dead code 阶段验证 telemetry 无副作用
- Phase 2 接 consumer 前必须有 Phase 1 telemetry，否则无法判断 search/shape 过滤是否误伤
- Phase 5 最后：blocklist 降级会改变现有 drop 语义，必须等 telemetry 和 consumer fallback 都稳定后再做

---

## 9. 反例 / 不在本设计范围

- **不删除 `GenericEntityBlocklist`**：保留作 NoiseScore 快速近似，不维护新词条但不删现有 38 项
- **不改 LLM-facing schema 必填字段集**：避免破现有 LLM 模型的生成习惯
- **不重写 retry_hint_sanitize**：仅导出已有 leak set 给 EntityProvenance 复用
- **不引入 write-mode EntityProvenance**：write_analyzer 独立路径，等真需要时单独设计
- **不触碰 reconcileScenario / reconcileIntent / inferAnswerSubject** 等其他 reconcile：本设计只处理 mirror class（明确 precedence），其他 reconcile 涉及语义判断不在 mirror 范畴
- **不处理 `predicates.is_count_question + is_scalar_answer` 类的硬一致性检查**：那些是 schema-level 必填一致性，应保持 fail-loud（degenerate 类）

---

## 10. 速查表（防 line drift）

代码 baseline 行号速查（实施前用 grep 对齐）：

| Symbol | File | Baseline Line |
|---|---|---|
| `GenericEntityBlocklist` | `internal/tool/analysis_limits.go` | 370 |
| `filterGenericEntitiesWithWhitelist` | `internal/tool/analysis_limits.go` | 633 |
| `validateAnalysisInput` | `internal/tool/analysis_limits.go` | 502 |
| `validateSelfConsistency` | `internal/tool/emit_analysis.go` | 1179 |
| `parsePredicates` | `internal/tool/emit_analysis.go` | 1309 |
| `parseDiagnosticProfile` | `internal/tool/emit_analysis.go` | 1357 |
| `parseConversationReferenceProfile` | `internal/tool/emit_analysis.go` | ~1430 |
| `parseErrorGranularityProfile` | `internal/tool/emit_analysis.go` | 1800 |
| `sanitizeSubTopics` | `internal/tool/emit_analysis.go` | 2493 |
| `reconcileDiagnosticQuestionProfile` | `internal/agent/analyzer_intent.go` | 119 |
| `reconcileQualifiedCodeSymbolConfigDrift` | `internal/agent/analyzer_code_symbol_reconcile.go` | 41 |
| Multi-topic merge | `internal/agent/analyzer.go` | 1665-1678 |
| `analyzerOracleFromCtx` | `internal/agent/analyzer.go` | 717 |
| `projectPrimaryScopes` | `internal/agent/scope_projection.go` | 56 |
| `projectSubTopicScopes` | `internal/agent/scope_projection.go` | 89 |
| `RequiredFileHint` struct | `internal/types/analysis_ir.go` | 773 |
| `AnalyzerHints` struct | `internal/types/analysis_ir.go` | 649 |
| `SubTopic` struct | `internal/types/analysis_ir.go` | 844 |
| `DiagnosticIntentProfile` mirror note | `internal/types/analysis_ir.go` | 583 |
| `RequiresDiagnosticRootCause` | `internal/types/analysis_ir.go` | 622 |
| `tryAutoRepairFinalizerAnswerDocument` | `internal/orchestrator/finalizer_auto_repair.go` | 24 |
| `FilterFinalizerRetryRootViolations` | `internal/orchestrator/violation_root_cause.go` | 161 |
| `AppendSoftContractCaveatsToAnswer` | `internal/orchestrator/repair_caveat_materializer.go` | 128 |
| §1.5 红线（precise hard gate） | `docs/architecture.md` | 62 |
| §1.6 红线（typed escape） | `docs/architecture.md` | 68 |

如 grep 不到 symbol，说明 baseline 漂移过远，**先跑** `git log 7ac49a35..HEAD -- <file>` 看历史，对齐当前状态后再实施。

---

**END of design doc.**
