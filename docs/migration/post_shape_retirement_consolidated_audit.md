> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# Block-Only 运行时收口审计终稿(P01-P38 + Batch B1-B6 整改计划)

> **归档元信息**
> - 状态:**FINAL**(已合并三方输入 + 代码二次确认)
> - 归档日期:2026-05-04
> - 代码基线:`origin/main@5e92dc5`(AnswerShape 终局 PR1-PR6 已落地)
> - 三方输入:
>   - **A. 实测数据**:8 runs eval(qf_architecture × 4 + m1a × 2 + u3a × 2),时间窗 2026-05-03 17:13–17:50
>   - **B. 用户审计需求**:`Block-Only 运行时缺口整改需求(中文版)`,审计时间 2026-05-04
>   - **C. 代码二次确认**:对每个 P 编号 grep + Read 真实代码,验证状态、修订误判、补漏网之鱼
> - 关联前置:[`answer_shape_terminal_retirement.md`](answer_shape_terminal_retirement.md)、[`block_only_carrier.md`](block_only_carrier.md)
> - 本文档负责回答两个问题:
>   1. **现状**:38 个具体问题各自的层级 + 状态(CONFIRMED/PARTIAL/...)+ 代码证据
>   2. **整改计划**:6 批(B1-B6)分批 fix 的文件/行号/前置/验收

---

## 0. 文档导读

| 节 | 用途 | 谁该读 |
|---|---|---|
| §1 | 当前已落地的事实 | 接手开发者首读;避免重复修已修 |
| §2 | 8 runs 实测矩阵 | QA / 验收时对照 |
| §3 | P01–P38 完整问题清单(分层 + 状态)| 整改时按 P# 查问题 |
| §4 | 多层根因表(L1 直接 / L2 结构 / L3 设计原则)| 设计 fix 时参考 |
| §5 | 根因聚类 A–I | 看哪些 P 应同批修 |
| §6 | 时间归因公式 | 性能优化决策 |
| §7 | 终态目标 | 验收基线 |
| §8 | **Batch B1-B6 分批修复计划**(含文件/行号/前置/验收)| 实施时按 batch 执行 |
| §9 | 强制审计清单 | 每个 PR 提交前自查 |
| §10 | PR 拆分建议(commit 边界)| 每 batch 内的 commit 切分 |
| §11 | 与 PR1-PR6(AnswerShape 退役)归责 | 区分历史遗留 vs PR3 引入 |

---

## 1. 当前已落地的事实(避免重复修)

### 1.1 V2 carrier(已落地)

| 已完成 | 代码锚点 |
|---|---|
| 最终答案 carrier 已 block-only | `internal/types/answer_document_v2.go`、`internal/tool/emit_answer_document.go`、`internal/tool/emit_answer_document_v2.go`、`internal/render/answerdoc.go` |
| V2 block-only contract oracle | `internal/orchestrator/contract_check_block.go::validatePrincipalClaimUse` 等 |
| Renderer 已 block-only(顶层 V1 字段在 runtime 已被拒绝) | `internal/render/answerdoc.go` |

### 1.2 V2 fallback policy(已部分落地)

| 已完成 | 代码锚点 |
|---|---|
| V2-specific violations 已正确映射 fallback target | `internal/orchestrator/fallback_policy.go:254-257` |
| `ViolPrincipalClaimUseMissing → FinalizerOnly` ✓ | line 255 |
| `ViolDiagramEdgeUnsupported → FinalizerOnly` ✓ | line 256 |
| `ViolUncertaintyBlockMissing → FinalizerOnly` ✓ | line 257 |
| `ViolBlockCoverageMissing → BackToExtract` ⚠️(过严,见 P36) | line 254 |
| `ViolFacetUncovered → BackToExplore` ⚠️ | line 218 |

### 1.3 think_aloud 配置(已部分落地)

| 已完成 | 代码锚点 |
|---|---|
| Per-agent think_aloud override | `cmd/root.go:2541-2559`、`orchestrator.go::SetThinkAloudMap` line 466、`agentCtx.ThinkAloud = ta` line 4407 |
| 但 retry-iteration 内动态降 think_aloud — **缺** | (无代码点) |

### 1.4 Semantic 底座(已落地)

| 已完成 | 代码锚点 |
|---|---|
| `QuestionFamily` 粗分类 | `internal/types/answer_semantic_view.go` |
| `FacetCoverageContract` | `internal/types/facet_plan.go` |
| `AnswerSemanticView` 编译 required/optional blocks | `internal/types/answer_semantic_view.go`、`answer_semantic_view_helpers.go` |
| `RenderedClaimUse` | `internal/types/rendered_claim_use.go` |

### 1.5 不要重复修(已修)

- ❌ ~~"V2 carrier 没落地"~~(已落地)
- ❌ ~~"renderer 还在按 shape 分支"~~(已 block-only)
- ❌ ~~"AnswerShape 仍是 active runtime core"~~(PR5 已删)
- ❌ ~~"think_aloud 是全局开关无 stage-aware"~~(per-agent override 已实现)
- ❌ ~~"机械新增 QFComparison"~~(必须由 semantic-view 表达不足驱动)

---

## 2. 数据基础(8 runs 实测)

| Run | 案例 | verdict | 总耗时 | claim_use 违规 | diagram 违规 | 其它违规 | retry 次数 |
|---|---|---|---|---|---|---|---|
| qf 171310 r1 | architecture | PASS | 5:07 | 5 | 0 | — | 1 → 干净 |
| qf 171310 r2 | architecture | PASS | 7:51 | 11 | 4 | authority_overreach×3 | 3/3 耗尽 + caveat 漏 stdout |
| qf 171516 r1 | architecture | PASS | 7:37 | 11 | 0 | — | 3/3 耗尽 + caveat 漏 stdout |
| qf 171516 r2 | architecture | PASS | 8:15 | 14 | 4 | CitationReq×1 | 3/3 耗尽 + caveat 漏 stdout |
| m1a r1 | call_chain 协作 | PASS | 6:13 | 0 | 0 | CitationReq×1 | 0 — 唯一干净 |
| m1a r2 | call_chain 协作 | PASS | 10:23 | 5 | 1 (block_coverage_missing diagram) | — | 多次 |
| u3a r1 | 对比类 ShouldStop | PASS | 9:15 | 6 | 0 | block_coverage(scalar=2 上限 1)×2 + CitationReq×4 | 多次 |
| u3a r2 | 对比类 ShouldStop | PASS | 15:38 | (含)| (含)| (含)| 3/3 耗尽 |

**关键事实**:`principal_claim_use_missing` 在 7/8 runs 命中(87%);全部 PASS 不代表"没问题",只是 EXPECT_CONTAINS 命中。

---

## 3. 完整问题清单(P01–P38,分层 + 状态)

### 3.1 L1 — Skill Prompt 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P01** | `principal_claim_use_missing`:family 要求 block 上挂 `claim_use` 但 LLM 不知道 | **CONFIRMED**(更严)| `grep claim_use internal/skill/defaults.go = 0 hits`;validator 在 `contract_check_block.go:99-132` 严格执行 | 7/8 (87%) |
| **P02** | prompt 仍教 V1 payload 心智 | **CONFIRMED** | `defaults.go:117` 仍写 `BlockOrderedList → steps[]/symbols[]`;`evaluator.go:277` 仍说"emit as symbols[]" | 全部 retry runs |
| **P03** | `diagram.kind` 在 prompt 里语义混淆 | **CONFIRMED** | `emit_answer_document.go:97` `"Diagram kind: flow / sequence / architecture / call_dag"`,与 Mermaid 关键字共享词域 | 4/8 (50%) |
| **P04** | prompt 泄露内部实现术语 | **CONFIRMED** | `defaults.go:143-144 + evaluator.go:320/332/450/525/535` 多处写 `QFEnumeration/QFCallChain/QFRootCauseTrace` | 全部 |

### 3.2 L2 — Schema 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P05** | `claim_use` 在 schema 里定义过弱 | **CONFIRMED**(更严)| schema line 89:`"claim_use": {"type": "object", "description": "Optional per-item claim annotation (claim_form / surface_role / facet_id)."}` — 一句话,无何时必填、无 worked example | 与 P01 共因 |
| **P06** | `document_model` 合同跨层不一致 | **CONFIRMED** | `emit_answer_document.go` schema 注释 "missing → v2" 但 `executeAnswerDocumentV2` 又要求 `DocumentModel == "v2"` | 长期风险 |
| **P07** | `diagram.kind` 没和 Mermaid 语法层解耦 | **CONFIRMED** | 同 P03 | 与 P03 共因 |
| **P08** | V2 nested schema 嵌套深 4 层,缺紧凑 worked example | **PARTIAL** | `blocks→items→facet_ids→diagram:{}` 共 4 层 | 1/8 |

### 3.3 L3 — Retry / Hint / Routing 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P09** | hint composer 缺新 V2 violation case | **CONFIRMED but narrower** | **范围缩小**:`fallback_policy.go:255-257` 已正确映射 V2-specific violations 到 FinalizerOnly;真问题只是 `composer.go` 缺 case 生成 actionable repair text | 5/8 |
| **P10** | finalize-V2 失败被拉回 extract/explore | **PARTIAL — 重写** | (a) 单独 V2-schema violation 正确路由 FinalizerOnly,**不是**自动拉深;(b) **混合** violation 时 `FallbackTargetForViolations` 取最深 → `BlockCoverageMissing→BackToExtract` 拉整组下沉(P36 是 root);(c) FinalizerOnly 时 read-mode TaskGraph 仍可能 sibling 跑 extract(orchestrator.go:3704-3711) | 5/8 |
| **P11** | retry hint 仍保留 V1 字段组 | **CONFIRMED** | `evaluator.go:2182` 写 "preserve every other field — citations[], steps[], symbols[], summary, value, boolean, exact_resolution" | 全部 retry runs |
| **P12** | hint composer 自身仍用 `shape` 术语 | **CONFIRMED**(更严)| `composer.go:38` `AllowedShapeEnum AllowedKind = "shape"` + line 426/433 仍在用 | 全部 V2 violation runs |
| **P13** | finalizer-only repair 仍依赖整份 V2 payload 重发 | **CONFIRMED** | `evaluator.go:2182/2195` 多处 "paste the FULL previous payload byte-identical" | 全部 retry runs |
| **P14** | retry 不递增 — 3 retry 收同样文本 | **CONFIRMED with caveat** | `evaluator.go:2118` 注释"HintKey embeds the retry counter so the LoopPolicy's dedup" — **dedup 已实现**;但 hint **text 不递进** | 4/8 |
| **P15** | `cannot unmarshal string into ... blocks` | **PARTIAL** | 由 P05/P08 共因驱动 | 1/8 |

### 3.4 L4 — Render / Output 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P16** | `validation exhausted` 文本走到用户答案通道 | **CONFIRMED**(更严)| `appendViolationsToAnswer` 在 `orchestrator.go:3568, 3587, 3620, 3692` **4 处**调用 | 3/8 |
| **P17** | `authority_overreach` 自激发 | **CONFIRMED — 升级** | 实测 8 runs 中 3 runs 命中;`contract_check.go:327` detail 文案写"This usually means the prior emit_answer_document call failed schema validation and the system fell back to raw prose" — 即官方承认是自激发循环 | 3/8 |

### 3.5 L5 — Family / Semantic Design 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P19** | 比较类 bucket 提取不足时,掉进邻近 family,缺对称 scaffold | **CONFIRMED** | `compile_call_chain.go` 无 BlockScalar 要求(用户说对了 — QFCallChain 不要求 principal scalar);真问题是缺 dedicated comparison family | u3a 全部 |
| **P20** | reviewer / coherence 语义归一化弱 | **PARTIAL** | reviewer 兼容层在,但更多依赖 surface phrasing | qf 171310 r2 |
| **P21** | `block_coverage_missing` (scalar 上限 1) | **CONFIRMED but root is P19** | scalar 上限 1 是 family 选错的下游症状 | u3a 全部 |
| **P22** | `block_coverage_missing` (diagram 缺失) | **CONFIRMED**(更严)| `compile_call_chain.go:51-66` 写 `BlockDiagram MinCount:1 Required:true` | m1a r2 |

> **P18 已移除**:用户文档 4.5.1 "缺少 dedicated `QFComparison`" 是 design constraint(family 新增由表达不足驱动),不是问题;现移到 §9.5 强制审计清单。

### 3.6 L6 — Grounding / Consistency 层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P23** | 缺 cross-citation single-locus consistency oracle | **CONFIRMED** | `grep cross.?citation\|locus.?consistency contract_check.go = 0 hits` | 1/8 |
| **P24** | `ClaimForm` 注释与 runtime 合同不一致 | **CONFIRMED**(更严)| `claim_form.go:43` 仍写"LLM never sees ClaimForm"作为 red line;但 `RenderedClaimUse + emit_answer_document.go schema:89/102` 实际让 LLM emit `claim_form/claim_uses` | 长期风险 |

### 3.7 L7 — 性能 / 可观测性层

| # | 标识 | 状态 | 代码证据 | 受影响 runs |
|---|---|---|---|---|
| **P25** | think_aloud 在 finalizer-retry 阶段无法关 | **PARTIAL — 范围缩小** | `cmd/root.go:2541 + orchestrator.go::SetThinkAloudMap` 表明 **per-agent 已实现**;真问题是 finalizer retry-iteration 内未动态降 think | 全部 retry runs |
| **P26** | 总耗时 5-15 min/run | **CONFIRMED** | LLM 调用 14-22s × 30-44 次 | 8/8 |
| **P27** | analyze 阶段固定 1.5-3.5 min | **CONFIRMED** | Round 1 + Round 2 pre-scan 重复发同 system prompt | 8/8 |
| **P28** | iteration 计数混合内部 pass 与 LLM turn | **PARTIAL — 范围缩小** | qf 171310 r2 finalizer log 显示 `iter=0/1/2/3` 共 4 个 iter = 4 次 LLM turn,**日志层已分清**;真问题在 `eval/run.sh::write_metrics` 计数粗放 — eval 工具问题,不是 codrax | eval 测试看到的"假数据" |
| **P29** | facet softening 致 richness 静默退化 | **CONFIRMED — 升级** | `facet_plan.go:393-395` 写 hard→soft 静默降级;`grep "logging.Warning\|logging.Info" facet_plan.go = 0 hits` — 完全无 telemetry | 长期风险 |

### 3.8 文档 / 注释 / 测试残影

| # | 标识 | 状态 | 代码证据 |
|---|---|---|---|
| **P30** | `docs/architecture.md` 仍保留 shape 时代描述 | **CONFIRMED** | `grep -ic shape docs/architecture.md = 36`;line 738/1400/1692/1693/1708/2050/2052 仍提 `RequiredAnswerShape/AnswerShape` |
| **P31** | `answer_document_v2.go` 注释把空 `document_model` 描述成可接受 legacy path | **CONFIRMED** | 同 P06 |

### 3.9 漏网之鱼(代码二次确认时新发现)

| # | 标识 | 状态 | 现象 | 触发位置 |
|---|---|---|---|---|
| **P32** | `appendViolationsToAnswer` 4 处调用,无统一策略点 | **CONFIRMED** | FailLoud / 中途 retry / 最终 retry 用尽 / cap 触发 4 种情况都各自调,无单一开关;后期想统一改成走 logger 时改 4 处易漏 | `orchestrator.go:3568, 3587, 3620, 3692` |
| **P33** | `prependFailLoudWarning` 与 caveat 双层叠加 | **CONFIRMED** | line 3693 在 caveat 之后**再**前缀 "this answer's flagged issues cannot be repaired by retry";用户面板可能看到两段 noise,无去重 | `orchestrator.go:3692-3694` |
| **P34** | violation kind ↔ hint composer case 缺 enumeration completeness lint | **CONFIRMED** | `composer.go` 加新 case 完全靠开发者记得;**没有任何测试**保证"新增 ViolationKind 同时新增 hint case",这是 P09 的真正根因 | `internal/analysis/hint/composer.go` + `internal/types/violation.go` |
| **P35** | facet hard→soft 降级**完全无 telemetry** | **CONFIRMED** | `facet_plan.go:388-405 CompileFacetCoverage` 没有任何 logging;实测 trace grep "facet.*soft\|hard.*soft" = 0 hits;richness 静默退化无观测信号 | `internal/types/facet_plan.go:393` |
| **P36** | `ViolBlockCoverageMissing → BackToExtract` 错责 | **CONFIRMED** | `fallback_policy.go:254` 把"finalizer 漏发 block 类型"和"上游缺证据导致无 block 可发"混为一谈,都拉回 extract;**前者 finalizer 自己就能修**,这是 P10(b) 的最危险映射 | `fallback_policy.go:254` + `compile_call_chain.go:51` |
| **P37** | `evaluator.go` retry hint 文案中 V1 字段引用 = 149 处 grep 命中 | **CONFIRMED** | 远超 P11 描述程度;`grep -c "(citations\[\]|steps\[\]|symbols\[\]|value|boolean|exact_resolution|summary)" = 149`。即使 V1 carrier 物理上已删,**文案上 149 处仍把 LLM 往 V1 心智推** | `evaluator.go` 全文 |
| **P38** | docs/architecture.md 还有 7 处错的 RequiredAnswerShape 引用 | **CONFIRMED** | line 1400/1692-1693/1708/2050-2052 不只是历史描述,而是把它当**当前机制**讲;尤其 line 2050 "添加新 AnswerShape" 一节会教坏新开发者 | `docs/architecture.md` |

---

## 4. 多层根因表

| # | L1 直接 | L2 结构 | L3 设计原则 |
|---|---|---|---|
| P01 P05 P22 | family → required-block 映射只走 contract checker,不走 prompt 编译 | 同一份契约规则只有验证出口,没有 LLM 入口 | **同一份契约规则必须有两个出口**:LLM 入口(skill prompt)+ 验证出口(contract checker) |
| P02 P11 P13 P37 | skill/evaluator/retry hint 仍写 V1 字段(149 处)| runtime carrier 迁了,LLM-facing 合同没迁完 | **runtime 合同迁移必须同步 LLM-facing 合同**(prompt/schema/retry/checklist 五件事并行) |
| P03 P07 | `diagram.kind` enum 与 Mermaid 关键字共享 token | schema 字段命名与 LLM 已知词汇冲突 | **typed-field 名称必须和 LLM 已知 well-known 词汇错开** |
| P04 | prompt 写 `QFEnumeration / QFCallChain` Go 内部类型名 | 内部抽象暴露在外部契约 | **prompt 不暴露内部类型名,只暴露 LLM 必须 emit 的 JSON 字段名** |
| P06 P31 | schema/executor/type 注释三层关于 `document_model` 各说各话 | 迁移期分叉 | **tool 入口契约必须保持四层一致**:tool description / JSON schema / executor / type-level 注释 |
| P08 P15 | V2 schema 嵌套深 4 层,无 worked example | LLM 序列化深 schema 错误率非线性 | **JSON schema 嵌套深度 ≥ 3 层时必须配 schema-by-example + flat-mode 宽容兜底** |
| P09 P12 P34 | hint composer switch 缺 case;`AllowedShapeEnum` 残留;无 lint 保证同步 | 新 violation kind 只加在 contract checker;共享基础设施旧术语未清理 | **violation kind ↔ retry-hint 必须 1:1 映射**;**共享基础设施不允许遗留旧术语** |
| P10 P36 | `FallbackTargetForViolations` 取最深;`ViolBlockCoverageMissing → BackToExtract` 不分场景 | fallback 选择按"最深 violation"而非"真正的 repair locus"走 | **fallback 必须按 repair locus 选**:能在 Stage-N 修的不应回到 Stage-N-1 |
| P14 | 同一 violation 重复 retry 收同样 hint text | 缺 escalation 概念 | **retry 必须信号递进**:第 N 次 retry 必须比 N-1 更具体或更约束 |
| P16 P32 P33 | `appendViolationsToAnswer` 4 处 + `prependFailLoudWarning` 叠加 | render 层 caveat 与 final answer 共用 stdout 通道,无单一策略点 | **用户面板 = 用户可读 + 系统自洽**;系统的"我没修好"应该走 logger |
| P17 | render-time transform 与 post-render validator 异步 | 两 pass 互相打架 | **同字段的 emitter 与 validator 必须共享 finalize-pass** |
| P19 P21 | 不要因 testcase 表面形态机械新增 family | 比较/对比类问题在 bucket 提取不足时退化到邻近 family,缺对称 scaffold | **family 新增必须由 semantic-view 表达不足驱动** |
| P20 P23 | reviewer 比对字面陈述;grounder 逐条校验 | 缺跨 citation/语义归约 | **一致性 ≠ 字面匹配**;**citation pool 必须在 closure 维度有一致性约束** |
| P24 | 注释说"LLM never sees ClaimForm",实际 LLM 可写 | 注释滞后 | **代码注释必须反映当前架构现实** |
| P25 | finalizer agent ThinkAloud 默认 true,retry 阶段无动态降 | 配置层 per-agent 但无 per-iteration | **think_aloud 应 stage + retry-iteration 双重 aware** |
| P26 P27 | LLM 调用次数是头号成本 | 同 stage multi-round 没共享 prompt cache | **LLM 调用次数 = 头号成本**;减少调用比加速调用收益高 10× |
| P28 | eval 工具 grep -c 计数粗放 | 不是 codrax 问题,是 eval/run.sh 设计问题 | **遥测必须分层**:外部 LLM turn 与内部策略 pass 必须独立计数 |
| P29 P35 | `CompileFacetCoverage` 在缺证据时静默 hard→soft 降级,无 logging | 无 hard→soft 降级的可观测性 | **richness 优化目标必须与 correctness 目标并列**;hard→soft 降级必须有可观测信号 |
| P30 P38 | docs/notes 滞后 | 迁移工作未到注释清理阶段 | **若某概念已不在当前 runtime contract,就不能继续残留在用户可见 prompt / 架构文档 / 公共 helper 类型名 / 测试名** |

---

## 5. 根因聚类(L3 原则视角)

| 集群 | 涵盖 P# | 共享 L3 原则 |
|---|---|---|
| **A. LLM 接口面契约缺口** | P01 P02 P04 P05 P11 P13 P22 P37 | "typed-field 必须配 prompt-side 教学" + "runtime 合同迁移必须同步 LLM-facing 合同" |
| **B. 命名 / 术语冲突** | P03 P04 P07 P12 | "typed-field 名称与 LLM 已知词汇错开" + "共享基础设施不残留旧术语" |
| **C. 跨层契约不一致** | P06 P10 P17 P24 P31 P36 | "tool/schema/executor/注释 必须四层一致" + "fallback 按 repair locus" |
| **D. retry 信号不收敛** | P09 P11 P13 P14 P34 | "失败信号最近边界回灌 + 信号递进 + 1:1 映射" |
| **E. 用户面板泄漏** | P16 P17 P32 P33 | "系统自洽失败只走 logger" |
| **F. 答案语义一致性弱验证** | P20 P23 | "一致性是 closure-级跨 citation 不变式" |
| **G. V2 schema 嵌套过深** | P05 P08 P15 | "depth ≥ 3 配 schema-by-example + flat-mode 兜底" |
| **H. Family 设计驱动力 + Richness** | P19 P21 P29 P35 | "family 新增由表达不足驱动 + richness 与 correctness 并列 + hard→soft 必须可观测" |
| **I. 性能与可观测性** | P25 P26 P27 P28 | "LLM 调用次数 = 头号成本 + 遥测必须分层" |
| **J. 文档残影** | P30 P38 | "退役概念不能残留在用户可见 prompt/文档/类型名" |

---

## 6. 时间归因公式

总耗时 ≈
- **基础**:analyze 2 min + explore 3 min + finalize-happy 30 s ≈ **5.5 min**
- **+ V2 contract retry × N**:每次 retry **45-90 s** → N=3 满 retry 加 **3-5 min**
- **+ family 契约死锁**(u3a 比较 / m1a r2 call_chain diagram):**额外 3-7 min**

→ **慢 run = 基础 5.5 min + retry 3-5 min + 死锁 0-7 min = 8-15 min**(实测吻合)

---

## 7. 终态目标

### 7.1 运行时真理源(已基本就位)
`QuestionFamily / FacetCoverageContract / AnswerSemanticView / RenderedClaimUse / AnswerDocumentV2`

### 7.2 模型可见契约
模型只被教:required blocks / required facets / allowed claim annotations / diagram family semantics / uncertainty / richness obligations

模型**不**被教:V1 payload 分解 / 内部 Go 类型名 / 仅迁移期实现术语

### 7.3 修复行为
每种 V2 violation 必须有:精确 fallback routing(repair locus 驱动)+ actionable retry text + 不退回不必要上游 + 信号递进

### 7.4 用户可见输出
运维/失败细节不再悄悄混入答案正文(除非产品明确选 fail-loud UX)

### 7.5 Richness(强制不变量)
hard-required 仍 correctness gate / optional richness 追踪 / hard→soft 降级有 observable signal / **优化必须衡量 useful answer retention**

---

## 8. Batch 分批修复整改计划(B1–B6)

> **核心原则**:每个 batch 内部的 fix **互不前置**,可任意顺序提交;**batch 之间有严格前置**,必须按 B1→B6 顺序。

### Batch 概览

| Batch | 名称 | 目标聚类 | 涉及 P# | 难度 | 预期 ROI(慢 run 节省)| 前置 |
|---|---|---|---|---|---|---|
| **B1** | LLM 接口面 V2 词汇统一 | A | P01 P02 P04 P05 P11 P13 P22 P37 | 中 | 一发命中率 +30%,慢 run -3 min | 无 |
| **B2** | Hint composer V2 violation 1:1 映射 | D | P09 P12 P14 P34 | 中 | retry 信号收敛,慢 run -1-2 min | 无(可与 B1 并行) |
| **B3** | 跨层契约一致性 + repair locus | C | P06 P10 P31 P36 | 中-高 | 修复 fallback 错责 | 无(可与 B1/B2 并行) |
| **B4** | 用户面板通道分离 | E | P16 P17 P32 P33 | 低 | 立即消除最严重用户可见问题 | 无 |
| **B5** | Schema worked example + Family 表达力 + Richness telemetry | G H | P08 P15 P19 P21 P29 P35 | 中-高 | 长期 retry 减少;richness 可观测 | B1(prompt 已 V2 化) |
| **B6** | Consistency oracle + 文档残影 + 性能 | F I J | P20 P23 P24 P25 P26 P27 P28 P30 P38 | 中 | 长期质量保障 | B1-B5 全部 |

### B1: LLM 接口面 V2 词汇统一(集群 A)

**目标**:模型按 blocks/facets/claim_use/diagram_family 思考,不按 V1 payload 心智。

**Fix 列表**:

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B1-F1** | `internal/skill/defaults.go` | 重写 OutputFormat:删 `BlockOrderedList → steps[]/symbols[]` 等映射段(line 117);加专门 `claim_use` 教学段(说明何时必填、block-vs-item 选择规则、`claim_form` × `surface_role` 合法组合);移除 `QFEnumeration/QFCallChain/QFRootCauseTrace` 内部标签(line 143-144),改用户可理解描述 | P01 P02 P04 P05 | `grep "BlockOrderedList -> steps\|QFEnumeration\|QFCallChain" defaults.go` 应 = 0 |
| **B1-F2** | `internal/agent/answer_document_evaluator.go` | 重写 `renderAnswerDocSubmissionChecklist/renderAnswerDocBlockRequirements`;删 line 277 "emit as symbols[]";所有 retry hint 把"preserve citations[], steps[], symbols[], summary, value, boolean, exact_resolution"改成 V2 词汇(blocks[]/claim_use/facet_ids/surface_role);改写 line 322/327/334/361/388/389/536/861 等 V1 字段引用 | P02 P11 P13 P37 | `grep -cE "citations\[\]\|steps\[\]\|symbols\[\]\|preserve.*value\|preserve.*boolean" evaluator.go` 应 ≤ 10(允许极少 schema 字段名引用,但不允许"preserve V1 field bundle" instruction) |
| **B1-F3** | `internal/tool/emit_answer_document.go` | 强化 `claim_use/claim_uses/facet_ids/diagram.kind` schema 描述(line 89/97/102);加每种 family 一份 worked example(BlockSummary alone / OrderedList over symbols / OrderedList over steps / Scalar / Decision);明确 `diagram.kind` = 语义 family,Mermaid 语法属于 `language/body` | P03 P05 P07 P22 | schema description grep `worked example\|Example` 应 ≥ 7;`diagram.kind` description 应包含 "semantic family, NOT Mermaid block type" 类似免歧义 |

**B1 验收**(跑 8 runs eval 回归):
- `principal_claim_use_missing` 命中率 87% → ≤ 30%
- 平均 finalizer iters 5-12 → ≤ 4
- 慢 run 平均时长 8-15 min → 5-7 min

### B2: Hint composer V2 violation 1:1 映射(集群 D)

**目标**:每种 V2 violation 有专属 actionable repair text;新增 violation 必须有 lint 兜底。

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B2-F1** | `internal/analysis/hint/composer.go` | 重命名 `AllowedShapeEnum → AllowedBlockKind`(或 `AllowedSemanticKind`),同步更新 line 38/426/433;删 shape 时代注释 | P12 | `grep -n "shape\|Shape" composer.go` 仅注释中保留迁移历史描述,无 active code |
| **B2-F2** | `internal/analysis/hint/composer.go` | 在 `summariseExactFix` switch 加 4 个 case:`ViolPrincipalClaimUseMissing`(教 LLM 加 `claim_use`)、`ViolDiagramEdgeUnsupported`(教改 `diagram.kind`)、`ViolUncertaintyBlockMissing`(教加 caveat block)、`ViolClaimFormUnsupported`(教 swap claim_form);每个 case 同步在 `buildAllowedSet` 加 allowed payload | P09 | `grep "ViolPrincipalClaimUseMissing\|ViolDiagramEdgeUnsupported\|ViolUncertaintyBlockMissing\|ViolClaimFormUnsupported" composer.go` 至少 4 条 case |
| **B2-F3** | `internal/analysis/hint/composer_test.go` | 新增**枚举完整性测试** `TestComposer_AllViolationKindsHaveCase`:遍历 `types.AllViolationKinds()`,断言每个 kind 在 composer 中有专属 case 或显式跳过(白名单标注) | P34 | 新增 violation kind 时 CI 必失败,逼开发者同步加 case |
| **B2-F4** | `internal/agent/answer_document_evaluator.go` | 加 retry escalation 概念:第 N 次 retry hint text 必须比 N-1 更具体或更约束;在 `renderRejectHint` 类函数读 `LoopObservation.Iteration`,根据 iter ≥ 1 加 "this is your retry N — focus only on these field(s)" 之类递进措辞 | P14 | retry text 在 iter=0 与 iter=1 不应字符串完全相等(可加 unit test) |

**B2 验收**:
- `composer.go` 4 个新 case 加齐
- 8 runs 跑下来,V2 violation retry 收敛速度提升(同 violation 不连续 retry 3 次)

### B3: 跨层契约一致性 + repair locus(集群 C)

**目标**:`document_model` 四层一致;fallback 按 repair locus 选择,不被混合 violation 拉深。

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B3-F1** | `internal/tool/emit_answer_document.go`、`emit_answer_document_v2.go`、`internal/types/answer_document_v2.go` | 选定 `document_model` 唯一事实(推荐:必须显式 `"v2"`,禁空);schema enum 改 `["v2"]`;executor 严格拒空;`AllowedDocumentModels()` 返回 `["v2"]`;type 注释更新 | P06 P31 | 三层 grep `\["", "v2"\]\|missing.*v2\|empty.*v2` = 0;executor 单测覆盖空字符串拒绝 |
| **B3-F2** | `internal/orchestrator/fallback_policy.go` | 引入 `repair_locus` 概念:violation 类型分类为 `finalizer_local`(只需 finalizer 重 emit)、`extract_local`、`explore_local`、`mixed`;`FallbackTargetForViolations` 改为 "primary repair locus" 选择(取数量最多的 locus,而非最深 stage);为 `ViolBlockCoverageMissing` 加 sub-classification(diagram-缺失 → finalizer_local;evidence-缺失 → extract_local) | P10 P36 | qf 171310 r2 重跑应不再触发 `dispatching agent=extractor`;`fallback_policy_test.go` 新增 mixed-violation 路由用例 |
| **B3-F3** | `internal/orchestrator/orchestrator.go:3704-3711` | 重写 read-mode TaskGraph 中 finalize requeue 时的 sibling 处理:FinalizerOnly 时**不**重跑 extract sibling | P10 | 实测 FinalizerOnly retry 路径 LLM 调用数 -1 次/retry |

**B3 验收**:
- `document_model` 三层一致(grep 验证)
- 混合 violation 不再拉深 extract;qf 171310 r2 类型 trace 显示 retry 只重 finalize

### B4: 用户面板通道分离(集群 E)

**目标**:运维/失败细节不悄悄混入答案正文。

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B4-F1** | `internal/orchestrator/orchestrator.go` | 把 4 处 `appendViolationsToAnswer` 调用统一到一个策略函数 `decideViolationOutputChannel(state, settings)`;新加 yaml 旋钮 `pipeline_violation_user_visible` (default=`false`);`false` 时只 logger.Warning,不写 `out.FinalAnswer`;`true` 时维持当前行为 | P16 P32 | `grep "appendViolationsToAnswer" orchestrator.go` 调用点合并为 1 处;实测 default 配置下 stdout 不再含 "validation exhausted" |
| **B4-F2** | `internal/orchestrator/orchestrator.go:3692-3694` | `prependFailLoudWarning` 在 caveat 已存在时去重(共享同一 channel-decision 路径);避免双层 noise 叠加 | P33 | 实测 FailLoud 答案前缀**最多一段**警告 |
| **B4-F3** | `internal/orchestrator/contract_check.go::appendViolationsToAnswer` | 该函数改名 `formatViolationsForLogger`,返回 string 不直接写答案;调用方决定走哪个 channel | P16 P32 | 函数名 grep + 调用方代码审查 |
| **B4-F4** | `internal/render/...` | 定义 `userPanelOnly` vs `loggerOnly` channel constant;新增系统 prose 必须显式选(default = logger),作为 lint/code review 红线写到 CLAUDE.md | P17 | 文档归档 + code review checklist 添加 |

**B4 验收**:
- 实测 8 runs 重跑,`grep "validation exhausted" eval/results/*/run-*.out` = 0(default 配置)
- yaml 设 `pipeline_violation_user_visible: true` 时旧行为可恢复

### B5: Schema worked example + Family 表达力 + Richness telemetry(集群 G/H)

**目标**:LLM 一发命中率提升;family 表达力评估机制建立;richness 静默退化可观测。

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B5-F1** | `internal/tool/emit_answer_document.go` | 加 schema-by-example:tool description 直接贴 1 个 minimal happy-path V2 emit JSON(每种 family 一个 5-block 完整示例);加 flat-mode 宽容路径:`blocks-as-string` detect → re-parse → 接受 + WARN log | P08 P15 | LLM emit 错误率 -20%;`grep "blocks-as-string\|escaped JSON" tool/*.go` 应 ≥ 1 处宽容处理 |
| **B5-F2** | `internal/types/facet_plan.go:393-395` + `internal/types/answer_semantic_view.go` | 在 `CompileFacetCoverage` hard→soft 降级处加 `logging.Warning` + 写 `bus.Mutable.RichnessTelemetry()` 字段;新增 yaml 旋钮 `pipeline_richness_softening_warn`(default=true) | P29 P35 | 实测 trace `grep "facet hard.*soft\|softening" eval/results/*/run-*.logs/*.log` ≥ 1 |
| **B5-F3** | `internal/types/facet_plan.go::ResolveQuestionFamily` + `compile_*.go` | 引入 family 表达力评估机制:对比较类问题(`QuestionStructure.Buckets ≥ 2`)在 `ResolveQuestionFamily` 加 explicit branch;若 `bucket_count ≥ 2` 但落到 `QFCallChain/QFGeneric` → 触发 `richness_telemetry.family_underrepresented` 信号(只 telemetry,不强行新建 family) | P19 P21 | u3a 类问题 trace 出现 `family_underrepresented=true` 信号;但**不**自动新建 QFComparison(遵循 design constraint) |

**B5 验收**:
- 跑同样 8 runs,LLM emit 字符串化 blocks 错误率 1/8 → 0
- u3a r1/r2 trace 显示 family-underrepresented telemetry
- Run 中至少出现 1 次 hard→soft 降级 logging

### B6: Consistency oracle + 文档残影 + 性能(集群 F/I/J)

**目标**:答案质量长期投资 + 文档清零 + 性能优化。

| Fix | 文件 | 具体修改 | 解决 P# | 验收 |
|---|---|---|---|---|
| **B6-F1** | `internal/orchestrator/contract_check.go`(新增 oracle 子集)| 加 cross-citation single-locus oracle:同 symbol/function 多条 citation 的 `(file, line)` 对必须一致(若不一致 → `ViolCrossCitationConflict` SOFT-by-default) | P23 | 单测覆盖 m1a 类(extractor.go:114 vs :135)情况 |
| **B6-F2** | `internal/orchestrator/answer_reviewer.go` | reviewer prompt 加语义归约能力:识别"含 pre-stage 总数 6 vs 主链 4-stage 设计"是同一事实;识别同名实体多种数值表达 | P20 | qf 171310 r2 类自相矛盾在 reviewer 命中 |
| **B6-F3** | `internal/types/claim_form.go:43` | 注释更新:删"LLM never sees ClaimForm" red line;改写为"LLM-emitted ClaimForm via RenderedClaimUse + system-validated by ClaimFormOf" | P24 | grep `LLM never sees ClaimForm` = 0 |
| **B6-F4** | `internal/orchestrator/orchestrator.go::SetThinkAloudMap` 周边 | 加 retry-iteration aware:当 `agent=finalizer ∧ iter ≥ 1` 时 dynamic 关 think_aloud(不影响首次 iter 思维链)| P25 | finalizer iter ≥ 1 LLM response 不含 `<think>` 段 |
| **B6-F5** | `eval/run.sh::write_metrics` | 把"finalizer_iters"从 `count_pattern "diag finalizer"`(含多事件)改为 `count_pattern "diag finalizer.*ASSISTANT"`(每 iter 一行)| P28 | metrics 显示真实 LLM turn 数,与 log 中 iter=N 对齐 |
| **B6-F6** | `internal/agent/analyzer.go`(pre-scan)| 共享 prompt cache:Round 1 + Round 2 用同一个长 system prompt,只增量发用户问题;或合并为单 wide-context 调用 | P27 | analyze 阶段时长 1.5-3.5 min → 1-1.5 min |
| **B6-F7** | `internal/agent/answer_document_evaluator.go` | 实现 retained-draft retry path:服务端保留上一版 payload,LLM 只发 block-id list of changes + new content;若 V2 schema diff 太复杂可降级到全量重发 | P13 P26 | 慢 run 平均 token 输出 -50% |
| **B6-F8** | `docs/architecture.md` | 删/重写 36 处 shape 残留(line 738/1319/1400/1692-1693/1708/2050-2052);特别 line 2050 "添加新 AnswerShape" 章节整段删 | P30 P38 | `grep -ic "AnswerShape\|RequiredAnswerShape" docs/architecture.md` ≤ 5(只允许迁移历史叙述) |

**B6 验收**:
- 8 runs 重跑 finalizer 平均时长 < 30s/iter(retained-draft 见效)
- analyze 阶段时长压缩 1-2 min
- 文档 grep 归零矩阵通过(同 `answer_shape_terminal_retirement.md` §12.17 模式)

---

## 9. 强制审计清单(每个 PR 必过)

### 9.1 Prompt 要求

- [ ] 除非模型必须字面 emit 某个 JSON 字段,否则不出现内部 Go 类型名
- [ ] 不允许再用退役的 V1 payload 心智作为主 instruction surface
- [ ] skill prompt、evaluator prompt、tool schema 之间不得互相矛盾
- [ ] 不允许把模型无法行动的项目内部术语直接暴露给模型
- [ ] 必须给模型足够信息让它无需猜测隐藏的 runtime 不变量

### 9.2 Schema 要求

- [ ] 任何 validator 依赖的字段都必须在 schema 或 prompt 里讲清楚
- [ ] schema 描述不得与 executor 行为冲突
- [ ] 语义 family 与传输语法必须清晰分层
- [ ] 嵌套深度 ≥ 3 必须配 worked example

### 9.3 Runtime 要求

- [ ] 任何 fallback path 都不能悄悄重新引入 V1 语义
- [ ] 任何 retry path 都不能再保留或教授退役字段 bundle
- [ ] 每新增一种 violation kind,必须同步补齐:fallback policy + actionable repair language + tests + composer hint case
- [ ] 不允许两个并行 gate 对同一失败面给出互相冲突的修复方向
- [ ] fallback 选择按 repair locus 而非最深 stage

### 9.4 Richness 要求

- [ ] 减少 retries 不能降低答案完整度
- [ ] required diagrams 仍然必须保留
- [ ] mechanism 类型问题必须保留机制本体
- [ ] enumeration 类型问题必须保留 completeness honesty
- [ ] hard→soft 降级必须有 telemetry

### 9.5 泛化要求

- [ ] 不能把某个仓的具体文件名写成逻辑规则
- [ ] 不允许只假设 YAML
- [ ] 不允许 testcase-name coupling
- [ ] 不允许硬编码 branch-depth / layer-count 上限
- [ ] **family 新增必须由 semantic-view 表达不足驱动,而非 testcase 表面形态**(P18 已废弃为问题,提升为强制约束)

### 9.6 死代码与迁移残影要求

- [ ] runtime-facing 代码不允许再出现退役的 `shape` 术语
- [ ] 不允许保留描述相反行为的旧注释
- [ ] 共享基础设施里不允许遗留旧兼容标识符(除非有明确说明 + 保留窗口)

### 9.7 端到端消费要求

V2 合同里保留的每个字段都必须能回答:谁产 / 谁消费 / 如何验证 / 如何在用户答案面体现。回答不清的字段要么删,要么补主链消费。

---

## 10. PR 拆分建议(每 batch 内的 commit 边界)

### B1 拆分(3 commits)

| commit | 范围 | 文件 |
|---|---|---|
| B1-c1 | F1 skill prompt 重写 | `defaults.go` |
| B1-c2 | F2 evaluator retry hint V2 化 | `answer_document_evaluator.go` |
| B1-c3 | F3 tool schema 强化 + worked examples | `emit_answer_document.go` |

### B2 拆分(3 commits)

| commit | 范围 | 文件 |
|---|---|---|
| B2-c1 | F1 + F2 hint composer 改造(rename + 4 个 case) | `composer.go` |
| B2-c2 | F3 enumeration completeness lint | `composer_test.go` |
| B2-c3 | F4 retry escalation 实现 | `answer_document_evaluator.go` |

### B3 拆分(3 commits)

| commit | 范围 | 文件 |
|---|---|---|
| B3-c1 | F1 document_model 三层统一 | 三个文件 |
| B3-c2 | F2 repair locus 引入 + ViolBlockCoverageMissing 拆分 | `fallback_policy.go` |
| B3-c3 | F3 read-mode TaskGraph FinalizerOnly 不跑 sibling extract | `orchestrator.go` |

### B4 拆分(2 commits)

| commit | 范围 | 文件 |
|---|---|---|
| B4-c1 | F1 + F3 channel 决策点统一 + 函数 rename | `orchestrator.go`、`contract_check.go` |
| B4-c2 | F2 + F4 prependFailLoudWarning 去重 + render channel constant | `orchestrator.go`、`render/...` |

### B5 拆分(3 commits)

| commit | 范围 | 文件 |
|---|---|---|
| B5-c1 | F1 schema worked examples + flat-mode 宽容 | `emit_answer_document.go` |
| B5-c2 | F2 facet softening telemetry | `facet_plan.go`、`answer_semantic_view.go` |
| B5-c3 | F3 family 表达力评估 telemetry(不新增 family) | `facet_plan.go::ResolveQuestionFamily` |

### B6 拆分(8 commits,可分两轮提交)

| commit | 范围 |
|---|---|
| B6-c1 | F1 cross-citation oracle |
| B6-c2 | F2 reviewer 语义归约 |
| B6-c3 | F3 ClaimForm 注释更新 |
| B6-c4 | F4 finalizer retry-iter think_aloud 动态关 |
| B6-c5 | F5 eval/run.sh metrics 修 |
| B6-c6 | F6 analyzer pre-scan prompt cache |
| B6-c7 | F7 retained-draft retry path |
| B6-c8 | F8 docs/architecture.md 重写 |

---

## 11. 与 PR1–PR6(AnswerShape 退役)归责

| 由 PR3 引入 | 与 PR1-PR6 无关(预存) |
|---|---|
| P01(claim_use 教学缺) | P03 P07 (diagram.kind 命名冲突,V2 设计期就埋) |
| P02(retry hint 仍带 V1 字段组) | P04(prompt 暴露 QF*) |
| 一半 P09(hint composer 没同步新 V2 violation) | P05 P08(V2 schema 嵌套 + 教学弱) |
| P12(`AllowedShapeEnum` 残影 — PR3 漏清) | P06 P31(document_model 跨层不一致) |
| P34(部分 — 缺 enumeration completeness lint) | P10 P36(fallback 取最深 + ViolBlockCoverageMissing 错责) |
| | P11 P13 P37(retry 字段组 + 全量重发) |
| | P14(retry 不递增) |
| | P15 P17(schema fallback / authority transform 异步) |
| | P16 P32 P33(validation exhausted 漏 stdout + 双层警告) |
| | P19 P21 P22(family/契约/比较类对称 scaffold) |
| | P20 P23(reviewer/grounder consistency) |
| | P24 P30 P38(注释/文档残影) |
| | P25 P26 P27 P28 P29 P35(性能 + 可观测性 + richness softening) |

PR3 真正引入的只有 **P01 + P02 + 一半 P09 + P12 + 一半 P34**。其余在 V2 carrier 落地时(更早)就埋了。

**最优起点**:**B1 + B2 并行**(都是 PR3 真正遗留的清扫,可同 batch 内 commit;影响面小、快速验证)。

---

## 12. 状态修订日志(从 draft 到 final 的差异)

记录代码二次确认时的修订,供后续审计追溯:

| 项 | draft 状态 | final 状态 | 修订原因 |
|---|---|---|---|
| P09 | CONFIRMED | **CONFIRMED but narrower** | fallback policy 已映射,真问题是 composer 缺 case |
| P10 | CONFIRMED | **PARTIAL — 重写** | V2-specific violations 已正确路由 FinalizerOnly;真问题在混合 violation 取最深 + read-mode sibling extract |
| P14 | CONFIRMED | **CONFIRMED with caveat** | dedup key 已实现,但 hint text 不递进 |
| P17 | PARTIAL | **CONFIRMED — 升级** | 实测 3/8 命中,文案明确写 "self-激发循环" |
| P18 | PARTIAL | **删除作为问题** | 移到 §9.5 强制约束 |
| P21 | CONFIRMED | **CONFIRMED but root is P19** | 是 family 误选下游症状 |
| P24 | CONFIRMED | **CONFIRMED — 严** | 注释 line 43 "LLM never sees ClaimForm" 与 schema 暴露明确冲突 |
| P25 | CONFIRMED | **PARTIAL — 范围缩小** | per-agent 已实现;真问题是 retry-iteration 内动态降 think |
| P28 | CONFIRMED | **PARTIAL — 范围缩小** | codrax 日志层已分清;问题在 eval/run.sh 计数粗放 |
| P29 | PARTIAL | **CONFIRMED — 升级** | hard→soft 降级**完全无 telemetry** |
| **新增 P32-P38** | — | **CONFIRMED** | 代码二次确认新发现 |

---

## 13. 待办

- [x] 合并 8 runs 实测 + 用户审计需求 + 代码二次确认三方输入
- [x] 修订状态标签矩阵(P09 P10 P14 P17 P18 P21 P24 P25 P28 P29)
- [x] 补漏网之鱼(P32–P38)
- [x] 写 Batch B1-B6 分批修复整改计划
- [x] 写 PR 拆分建议(每 batch 内 commit 边界)
- [x] 写状态修订日志
- [ ] 收到执行授权后启动 **B1 + B2 并行**(PR3 真正遗留的清扫)
- [ ] 每个 Batch 落地后跑同样 8 runs 回归,对照 §8 各 batch 的"验收"指标
- [ ] B1 落地后做一次 `principal_claim_use_missing` 命中率快照,确认从 87% 降到 ≤30%
- [ ] B4 落地后确认 `validation exhausted` 不再出现在用户可见 stdout(default 配置)
- [ ] B6 后做 grep 归零审计 — 同 `answer_shape_terminal_retirement.md` §12.17 模式
