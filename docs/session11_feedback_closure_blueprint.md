# Session 11 — Feedback-Closure Architecture 蓝图

> **Status**: planning · 2026-04-18
> **Trigger bug**: `explorer agent 默认用哪个 skill?` — log `codrax-20260418-144900-000-294289.log`
> **Symptom**: 答案最终正确（`explore-skill`）但 CGEC summary 显示 `chains_demoted=220 forced_reads=11 shape_swap=3 pre_complete_downgrades=2`，总 LLM 时间 ~50 秒；前一轮同问题 `forced_reads=0` 直接答错（`explorer.go:6328` fabricated citation）。

---

## 1. 问题陈述

端到端数据流审计揭示的不是"一个 bug"而是"一类系统性失效"：

- 每个 enforcer（`B2a shape_swap` / `D2 ghost-anchor` / `E2 forced-read` / `G1 pre-finalize dry-run` / `contract_check`）都在自己那一层 reject / swap / skip，**信息不汇总、根因不回流、hint 不互通**
- 下游发现分类可能错 → 无法告诉 analyzer 改
- 同类 violation 反复发生 → 没有聚合识别
- LLM 收到的 hint 往往是"wrong, retry" → LLM 只能在局部瞎撞

这是**信号丢失式的 retry**，不是真正的反馈。

---

## 2. 两条深层失效轴

| 轴 | 本质失效 | 现象 |
|---|---|---|
| **R — 结构语义盲** | 源码里用**结构化形式**表达的"默认/注册/映射"信号（map literal、struct field、const block、topology 声明），对基于 keyword/IDF 的 retrieval + 基于 literal match 的 chain ranker 完全不可见 | `topology.go` 被埋 + self-name 陷阱 + forced-read 拉错文件 |
| **C — 契约-反馈链路弱** | 上游契约（shape/subject/question_kind/axes）在 prompt 链路里 salience 不够、不做级联 reconcile；下游 enforcer 发现偏差只做事后兜底，不反哺上游 retrieval | subject 改了 question_kind 没改 + D2 skip 不 expand + shape_swap 反复兜底 |

---

## 3. 11 条用户需求

| # | 需求 |
|---|---|
| 1 | 端到端完整 |
| 2 | 前端修复优先 |
| 3 | 后端兜底 |
| 4 | 避免垃圾信息残留 |
| 5 | 避免大范围空转 |
| 6 | 泛化（不是 over-fitted） |
| 7 | 多点发力 |
| 8 | **后端反馈纠错机制** |
| 9 | 避免问题逐层传播 |
| 10 | **空转兜底消除** |
| 11 | **hint 完整精准** |

---

## 4. 四轴架构

```
前端预防                               后端反馈闭环
┌────────────────┐                   ┌────────────────────────┐
│ C0             │                   │ F1 ViolationLedger     │
│ Classification │                   │ F2 RootCauseAggregator │
│ Read           │◀─── IR patches ───│ F3 IRPatchEngine       │
└───────┬────────┘                   │ F4 HintComposer        │
        │ IR                         │ F5 ViolationBudget     │
        ▼                            └──────────▲─────────────┘
┌────────────────┐                              │ violations
│ R axis         │                              │ from every
│ R1 Rank boost  │──┐                           │ enforcer
│ R2 auto kw     │  │                           │
│ R3 axis demote │  │                           │
│ R4 evid filter │  │                           │
│ R5 D2 feedback │  │         Explore /         │
│ R6 fread dedup │  ├──▶  Extract / Finalize ───┘
└────────────────┘  │
┌────────────────┐  │
│ C axis         │  │
│ C1 cascade     │  │
│ C2 shape front │  │
│ C3 anti-self   │  │
│ C4 shape reject│  │
│ C5 literal form│  │
│ C6 retry diff  │  │
└────────────────┘  │
```

### 轴 C0 — ClassificationGrep（前端 + analyzer）
Analyzer 在 Round 2 **解禁** `grep files_only=true` 的硬约束（仍限制为白名单 path + 匹配数 + 总字节），拿到 line-level 匹配结果，用来锁 axis。**不新增 tool，不新增通道**；line-level 结果做 stage-aware 隔离，不进 `TurnAArtifacts.ToolResults`。堵住分类漂移。

**为什么 grep 够**：分类只需验证 axis 存在（map literal 结构 + literal suffix 形态），不需要读函数体/注释/导入 — 读整个文件反而污染 LLM 判断。

### 轴 R — Retrieval & Anti-trap（前端 + explore/extract）
| # | 内容 |
|---|---|
| R1 | `keyword_search` 加 `DeclarativeBoost`：文件名词表 + 小文件声明密度 |
| R2 | Analyzer 自动补充 `topology/registry/defaults` 到 keywords |
| R3 | Chain ranker axis-aware：self-name chain 降权 |
| R4 | `emit_evidence` 入口过滤 self-reference literal |
| R5 | CGEC D2 ghost-anchor 聚合 → 主动 expand_search |
| R6 | E2 forced-read 去重 + declarative 优先 |

### 轴 C — Contract（契约全链路前置化）
| # | 内容 |
|---|---|
| C1 | E1 override subject 后级联 reconcile question_kind / shape（被 F3 吸收） |
| C2 | Finalizer system prompt：RequiredAnswerShape 顶级前置 |
| C3 | Extractor system prompt：self-reference negative rule 顶级前置 |
| C4 | Finalizer shape violation：reject-no-rescue，删除 B2a 救援 |
| C5 | G1 pre-finalize dry-run：加 literal form 检查 |
| C6 | retry budget 按 violation kind 差异化 |

### 轴 F — Feedback Closure（后端反馈）
| # | 内容 |
|---|---|
| F1 | `ViolationLedger`：所有 enforcer 结构化写入，附 `suspected_root` |
| F2 | `RootCauseAggregator`：跨事件模式识别 → `IRPatchRequest` / `HintEnrichment` |
| F3 | `IRPatchEngine`：deterministic 修正上游 IR（不重跑 analyzer） |
| F4 | `HintComposer`：6 字段精准 hint（what/why/did/fix/allowed/forbidden） |
| F5 | `ViolationBudget`：单调性 + retry yield kill + fail-loud default |

---

## 5. 完整方案 = 四轴 19 组件一次 ship

**重要决策变更（2026-04-18，由用户提出）**：放弃"最小闭环 + 遗留到 session 12"的渐进策略。一次性把 19 个组件全部落地，原因：
1. 组件之间强耦合 — 例如 F3 IRPatchEngine 没做，C1 cascade reconcile 必须单独手写；F2 Aggregator 没做，F1 ledger 只能做 observability。拆 session 会产生大量临时代码
2. "最小闭环 + 后续扩展" 的心智模型往往演变成"最小闭环跑完就算完" — 遗留工作从未真正跟进（参考 session 9 承诺没兑现，session 10 才补完）
3. 这次 bug 暴露的是架构级失效，半拉子修复等于没修

**19 组件完整列表 × 11 条需求覆盖矩阵**：

| # | 组件 | 覆盖需求 |
|---|---|---|
| C0' | ClassificationGrep（Round 2 line-level） | #2 #5 #6 |
| R1 | DeclarativeBoost 文件 ranker | #6 #9 |
| R2 | analyzer auto-keywords | #6 |
| R3 | axis-aware chain demote（self-ref 降权） | #4 #9 |
| R4 | emit_evidence self-ref 入口过滤 | #4 |
| R5 | CGEC D2 ghost-anchor → expand_search 反馈 | #3 #8 |
| R6 | E2 forced-read 去重 + declarative 优先 | #5 #10 |
| C2 | Finalizer `shape` prompt 顶级前置 | #4 #9 |
| C3 | Extractor self-ref negative prompt 顶级前置 | #4 #9 |
| C4 | Shape violation reject-no-rescue（删 B2a 救援） | #4 #10 |
| C5 | G1 pre-finalize literal form check | #4 |
| C6 | retry budget 按 violation kind 差异化 | #10 |
| F1 | ViolationLedger（contract.Violation 扩展） | #3 #7 #8 |
| F2 | RootCauseAggregator（聚合模式识别） | #8 |
| F3 | IRPatchEngine（deterministic 反向修正，吸收 C1） | #8 #3 |
| F4 | HintComposer（6 字段精准 hint） | #8 #11 |
| F5 | ViolationBudget（yield kill + fail-loud） | #5 #10 |

**11 条需求 ✓ 覆盖率 11/11**。

**为什么一次 ship 可行**：
- session 10 是 +1500 产品 / +700 测试 / 9 组一次 ship，成功
- 本方案估 +4200 产品 / +2800 测试 / 19 组一次 ship，约 4-5× session 10 规模
- 组件边界清晰、接入点明确、每组都有独立测试；atomic ship 比跨 session 分期更安全（无中间半成品状态）

---

## 6. Prior-art 审查结论（2026-04-18）

对七个候选组件逐个审查仓库里的近邻实现，判断"扩展 vs 新造"。

| 组件 | 判决 | 落点 |
|---|---|---|
| ViolationLedger (F1) | **扩展** | 现有 `contract.Violation` + `EvidenceClosure.Repairs` 已构成基础 ledger；只需加 `SuspectedRoot` 字段 + 把 enforcer 的 log-only 路径改为写 ledger |
| DeclarativeFileSet / R1 | **扩展** | `keywordFileScore` + `RankGraph.queryBoostMultiplier` 已有 structural boost 机制；加 `FileKind` 枚举 + 小文件折扣即可 |
| HintComposer (F4) | **新造** | 现有 `renderViolations()` / `renderWindowHint()` 只是字符串拼接，无 6 字段结构。新建 `internal/analysis/hint/` |
| ViolationBudget (F5) | **扩展** | `PipelineSettings.MaxRetriesPerStage` + `graphState.retryUsed` + `LoopPolicy.idleStreak` 已构成 budget 基础；加 `yieldCheck(delta)` 钩子 |
| ClassificationGrep gate (C0) | **扩展** | `validateAnalyzerPrescanToolCall` 已在 `internal/agent/agent.go:1094-1123`；加 Round-aware 分支：Round 1 强制 `files_only=true`，Round 2 + trigger 打开时允许 `files_only=false` 但强制 `-c / --max-count` 和 total bytes 上限 |
| ClassificationGrep 隔离 | **扩展** | tool result post-processing 里加 stage-check：analyzer Round 2 的 line-level 结果不写 `TurnAArtifacts.ToolResults`，只进临时 `ClassificationObservations`（仅 reconciler 可见）。改动局限在 `internal/agent/analyzer.go` 的 tool result 处理 |
| IRPatchEngine (F3) | **新造** | IR 是 invariant（analyzer 唯一 writer），反向修正违反不变量；需新建 `internal/analysis/patcher/` 维护幂等性 + 审计 — **不在最小闭环** |

**最小闭环（C0 + F1 + F4 + F5）只需 1 个新子包（`internal/analysis/hint/`）+ 1 个新工具包候选（`internal/analysis/declarative/`，R1 共享）**。其他全是扩展现有代码 — 不新造 tool、不新造 evidence 通道、不动 tool registry。这是"避免重复造轮子"的直接收益。

C0 关键决策（2026-04-18 用户提出后采纳）：**grep line-level 即足够，不再新造 read_file 类工具**。Round 2 解禁 `files_only=true` 约束 + stage-aware 结果隔离，比新造 `classification_read` 轻量一个数量级，且复用 analyzer 已熟悉的 grep 语义。

---

## 7. 物理落点矩阵（完整方案 — 19 组件）

| 修改点 | 文件 | 操作 |
|---|---|---|
| F1 Violation.SuspectedRoot | `internal/analysis/contract/checker.go` | 扩展现有 struct |
| F1 EvidenceClosure.Violations | `internal/types/evidence_closure.go` | 加字段 + AppendViolation |
| F1 enforcer hookups | `internal/orchestrator/cgec_enforcers.go` / `internal/tool/emit_answer_document.go` / `internal/agent/explorer_erm.go` | 每个 reject 点加 ledger.Write |
| F4 HintComposer | `internal/analysis/hint/composer.go` | 新建 |
| F4 hint 接入点 | `internal/orchestrator/contract_check.go` / `internal/orchestrator/scheduler.go` | 把 renderViolations 换成 composer.Render |
| F5 yield check | `internal/orchestrator/scheduler.go` | 在 retry 前加 yieldCheck |
| F5 budget fields | `internal/types/config.go` | 加 `PatchBudget`, `MinRetryYield` |
| C0 grep gate Round-aware | `internal/agent/agent.go::validateAnalyzerPrescanToolCall` | 扩展：Round 1 强制 files_only=true；Round 2 + trigger 打开允许 files_only=false 但强制 max-count 和 byte cap |
| C0 ClassificationObservations 隔离 | `internal/agent/analyzer.go` (tool result post-processing) | 扩展：Round 2 line-level 结果不写 `TurnAArtifacts.ToolResults`，只进临时 sidecar 供 reconciler 消费 |
| C0 Classifier | `internal/analysis/declarative/classifier.go` | 新建（R1 共享；最小闭环不硬依赖，先 stub） |
| C0 Reconciler 接入 | `internal/agent/analyzer.go::buildAnalysisIR` | 插入 step 1.5；消费 ClassificationObservations reconcile IR 字段 |
| C0 trigger gate | `internal/agent/analyzer.go` | Round 2 prompt-time 判断是否注入 "grep line-level permitted" 提示 |

**新子包计数**：2 硬需（`internal/analysis/hint/` + `internal/analysis/declarative/`）+ 1 新造（`internal/analysis/patcher/` 承载 F3 IRPatchEngine）+ 1 新造（`internal/analysis/aggregator/` 承载 F2）。其余全是扩展现有代码 + 扩展现有子包。

## 7b. 物理落点增量（R/C/F 剩余 12 组件）

| 组件 | 落点 | 操作 |
|---|---|---|
| R1 DeclarativeBoost | `internal/agent/keyword_search.go` + `internal/tool/repomap/retrieve/rank.go` | 扩展 scoring；加 FileKind |
| R2 auto-keywords | `internal/agent/analyzer.go::buildAnalysisIR` step 2 后 | 扩展 — declarative 候选触发时补 keyword |
| R3 axis-aware chain demote | `internal/agent/explorer_erm.go` chain scorer | 扩展 — 加 selfRefDemote 规则 |
| R4 evidence self-ref filter | `internal/tool/emit_evidence.go::Execute` 入口 | 扩展 — 加 `isSelfRefLiteral(subject, primary_entity)` 前置 check |
| R5 D2 → expand_search | `internal/orchestrator/cgec_enforcers.go::recordGhostAnchor` | 扩展 — 聚合同 file 阈值到则 append 到 ScannedSet + 标脏 |
| R6 forced-read dedup | `internal/orchestrator/cgec_enforcers.go::E2ForcedRead` | 扩展 — 加 per-file once + Classifier 优先级排序 |
| C2 Finalizer shape front | `internal/skill/answer_document_skill.go` system prompt | 扩展 — 把 RequiredAnswerShape 从 retry-hint 前移到 system prompt 顶部 |
| C3 Extractor self-ref negative | `internal/skill/extract_skill.go` system prompt | 扩展 — 加 negative rule |
| C4 shape reject-no-rescue | `internal/tool/emit_answer_document.go::shapeSwapB2a` | 删除 `can_correct=true` 救援分支，改纯 reject；F4 合成精准 hint |
| C5 G1 literal form check | `internal/tool/emit_answer_document.go::dryRunG1` | 扩展 — 基于 answer_subject.kind 验证 literal 形态（e.g. `skill_name → must end with "-skill"`） |
| C6 retry budget diff | `internal/types/config.go::PipelineSettings` + `internal/orchestrator/scheduler.go` | 扩展 — 按 ViolationKind 分桶 budget |
| F2 RootCauseAggregator | `internal/analysis/aggregator/aggregator.go` | 新造 — 消费 F1 ledger，产出 `IRPatchRequest` / `HintEnrichment` |
| F3 IRPatchEngine | `internal/analysis/patcher/engine.go` | 新造 — deterministic patch + audit + 幂等 |

---

## 8. 分期 ship 顺序（完整方案 — 分 7 组原子 PR，全部在一个 session 内 ship）

每组是一个 atomic PR，不跨 session 遗留。每组内部可并行开发，组间有依赖。

| 组 | 内容 | 依赖 | 验收指标 |
|---|---|---|---|
| **G1** | **基础总线**：F1 Violation 扩展 + EvidenceClosure.Violations 字段 + 8 处 enforcer hookup（带 SuspectedRoot） | 无 | `TestViolationLedger_AllEnforcersWriteSuspectedRoot` 通过；`violations_by_field` 出现在 CGEC summary |
| **G2** | **聚合 + 反向修正**：F2 Aggregator 新包 + F3 IRPatchEngine 新包 + IR mutation audit trail | G1 | 模拟 3 条 ShapeMismatch violation → F2 产出 IRPatchRequest → F3 应用 patch → IR 相应字段更新且 audit log 记录 |
| **G3** | **精准 hint**：F4 HintComposer 新包 + 5 处接入替换字符串拼接 + strict_mode=false 灰度 | G1 | `TestHintComposer_CoversAllViolationKinds`；现有 retry hint 升级到 6 字段 |
| **G4** | **预算与 fail-loud**：F5 ViolationBudget 字段 + yield check + fail-loud prepend + C6 retry budget 差异化 | G1 | `TestViolationBudget_YieldKillStopsLoop`；退化构造问题触发 fail-loud 且 warning 前置 |
| **G5** | **前端分类**：C0' grep gate Round-aware + ClassificationObservations sidecar + Reconciler 接入 buildAnalysisIR + declarative Classifier 新包（R1 共享） | 无（与 G1-G4 并行） | 本 bug Round 2 trigger + reconcile `skill_name/value/return_value` |
| **G6** | **ranker + LLM 阶段防护**：R1 DeclarativeBoost + R2 auto-keywords + R3 axis-aware demote + R4 evidence self-ref filter + C2 shape prompt 前置 + C3 self-ref negative prompt | G5 (共享 Classifier) | declarative 文件进 Top-20；self-ref chain 降权；本 bug evidence 不再含 SubExplorer.Name() trap |
| **G7** | **反馈路由 + 清理**：R5 D2 → expand_search + R6 forced-read dedup + C4 删 B2a 救援 + C5 G1 literal form check + 删 C1 手写级联代码（被 F3 吸收） | G2 (F3) + G3 (F4) | 本 bug 10 次跑 `forced_reads ≤ 2 & shape_swap = 0 & 耗时 < 20s` |

**落地顺序约束**：
- G1 必须第一（总线）
- G2 / G3 / G4 可在 G1 后并行
- G5 独立（不依赖 F 轴），可与 G2/G3/G4 并行
- G6 依赖 G5（共享 Classifier）
- G7 最后（需要 F3 + F4 都就绪）

**为什么这是一次 ship 而不是 7 个 session**：7 组都在一个 feature branch，按组 merge，最后合 main。中间状态若无 F3 但有 C4 会破（C4 删了 B2a 救援需要 F4 精准 hint 替代 — 如果 F4 没就绪就破）。atomic ship 避免这种半成品 bug。

---

## 9. 预期指标收益

对比本 bug 当前指标：

| 指标 | 当前 (14:49 log) | 最小闭环后预期 |
|---|---|---|
| `chains_demoted` | 220 | < 20 |
| `forced_reads` | 11 | 0–2 |
| `shape_swap` | 3 | 0 |
| `pre_complete_downgrades` | 2 | 0 |
| retry 轮次 | 5+ | 1–2 |
| LLM 总耗时 | ~50 s | ~12–15 s |
| hint 信息密度 | 1–2 事实 | 6 字段结构化 |

---

## 10. 配置旋钮（`codrax.yaml`）

```yaml
# F1 ViolationLedger
violation_ledger_enabled: true
violation_ledger_min_confidence: 0.5     # 低于此置信度的 suspected_root 不入 ledger

# F4 HintComposer
hint_composer_strict_mode: true           # true=6 字段缺失 reject；false=warn 后通过
hint_composer_max_allowed_set: 10         # 枚举上限
hint_composer_max_forbidden_patterns: 5

# F5 ViolationBudget
violation_budget_max_patches: 4           # per-Run IRPatch 总数
violation_budget_per_field_patches: 2
violation_budget_min_retry_yield: 1       # Δforced_reads + Δpatches + Δnew_evidence ≥ N
violation_budget_fail_loud_enabled: true

# C0 ClassificationGrep
analysis_classification_grep_enabled: true
analysis_classification_grep_max_calls: 3            # Round 2 至多这么多次 line-level grep
analysis_classification_grep_max_matches_per_call: 20 # --max-count
analysis_classification_grep_max_total_bytes: 8192
analysis_classification_grep_min_llm_subject_conf: 0.8

# C0/R1 shared DeclarativeFileSet
declarative_filename_patterns:
  - topology
  - defaults
  - registry
  - routes
  - wire
  - init
  - manifest
  - schema
  - enum
declarative_max_lines_for_small: 60
declarative_literal_block_ratio: 0.6
```

所有旋钮 `enabled=false` 时回到现有行为。灰度/回滚成本 = 改一个配置。

---

## 11. 验收标准

最小闭环 ship 完后，同一个问题 `explorer agent 默认用哪个 skill?` 跑 10 次：

- [ ] 10/10 次答案正确（`explore-skill` + 正确 citation）
- [ ] 10/10 次 `forced_reads ≤ 2`
- [ ] 10/10 次 `shape_swap = 0`
- [ ] 10/10 次 `pre_complete_downgrades = 0`
- [ ] 10/10 次 LLM 总耗时 < 20 s
- [ ] hint 里出现 `what_system_did: "analyzer read X, Y to lock axis"`
- [ ] F5 kill switch 有单元测试覆盖（mock "retry 无 yield" 场景）

泛化验收（另 5 个问题，都应走 C0 路径）：
- [ ] `codrax 里注册了哪些内置工具?` — 应读 `internal/tool/defaults.go`
- [ ] `哪个 agent 绑定 analysis-skill?` — 应读 `topology.go`
- [ ] `extractor 的默认 skill?` — 应读 `topology.go`
- [ ] `有哪些 DeclarativeKind?`（新加枚举后自测） — 应读 `declarative/classifier.go`
- [ ] `MCP 路由表在哪?` — 应读 `routes.go` 类

---

## 12. 本方案 vs 未来扩展

**本方案 19 组件一次 ship，完整覆盖 11 条需求 — 不遗留到 session 12**。

可预见的未来扩展（不属于本 session）：
- LLM 模型升级带来的分类/推理能力提升（正交于架构）
- 通用 semantic search 层（替代 keyword/IDF，需要 embedding 基础设施，独立项目）
- 跨 Run 的持久化 ViolationLedger（当前是 per-Run 内存结构，跨 Run 学习是独立主题）

以上三者都是"另一个 session 的课题"，不属于本 bug 类的架构性失效修复。

---

## 13. 风险 + 护栏

| 风险 | 护栏 |
|---|---|
| F1 结构化 violation 改出 bug，破坏现有 CGEC 路径 | F1 期是纯 observability — enforcer 的 reject 动作一字不改，只加 ledger.Write |
| F4 严格模式下 hint 字段缺失导致整个 retry hint 不下发 | F4 灰度：strict_mode=false 先跑一周观察哪些调用点字段缺，补齐后再打开 |
| F5 yield kill 误杀合理 retry | yield 阈值可调；首期 min_retry_yield=1（最宽松），观察 fail-loud 触发率，逐步紧 |
| C0 ClassificationGrep 被 LLM 滥用深挖 | 四重硬 gate（Round-gate / max-count / total-bytes / trigger gate），任何一条被绕过都 tool fail |
| C0 line-level 结果污染下游 | stage-aware 隔离：Round 2 line-level 结果只进 `ClassificationObservations` sidecar，不写 `TurnAArtifacts.ToolResults`，CGEC G1 / citation grounder 看不见 |
| C0 让 analyzer token 涨导致成本失控 | +1–2 KB per analyze × 触发率 < 30%（仅分类有歧义时触发）= 平均 +400 tokens。换 ~35 秒下游 LLM 节省，净赚 |
| 新子包 `hint/` 和 `declarative/` 和现有 `analysis/*` 风格漂移 | 强制 mirror `analysis/criterion/` 的包组织（一个入口函数 + 一个测试文件 + 一个 README header） |

---

## 14. 本蓝图之外

- 不解决 LLM 的"读完仍然想错"问题 — 靠模型本身进步或换模型
- 不解决 keyword_search 对完全无 keyword match 的语义盲区（如"这个系统最复杂的流程是什么"）— R 轴的 R1 只 boost declarative 类，不解决通用 semantic search
- 不替代现有的 CGEC 四不变量 I1–I4 — F 轴是在 CGEC 之上再加一层，不动 CGEC 语义

---

**状态**：蓝图定稿，进入最小闭环综合设计阶段。

**参考**：
- 触发 bug log：`.codrax/logs/codrax-8ae69134/codrax-20260418-144159-000-292990.log`（错答）
- 对照 log：`.codrax/logs/codrax-8ae69134/codrax-20260418-144900-000-294289.log`（对但绕）
- Session 10 CGEC 基础：`docs/architecture.md §8`
