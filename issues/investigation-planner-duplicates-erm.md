# 回信：d17e404 Adaptive Investigation Planner 不合入

## 决定：不合入 —— 与现有 ERM 子系统重复

感谢细致的工作。但经审计，此 commit 暂不合入。Adaptive Investigation Planner
与仓库里已有的 Evidence Requirement Matrix（`internal/agent/explorer_erm.go`，
约 1000 行，已接入 explorer 主循环）有约 80% 的能力重复。

## 重复能力对照

| 新 Planner 能力 | 已有 ERM 对应项 |
|---|---|
| `InvestigationStep{Kind, Entities, Status, ...}` | `EvidenceRequirement{Kind, Entities, Status, Reason}`（explorer_erm.go:38） |
| `InvestStrategy` 枚举（7 值） | `RequirementKind` 枚举（8 值，`internal/types/requirement_kind.go`）—— 一一映射：locate+trace_chain ⇆ ReqMechanism / ReqCallChain；enumerate ⇆ ReqEnumeration；verify_condition ⇆ ReqConditional |
| `DecomposeQuestion(...)` | `extractEvidenceRequirementsWithHint(question, entities, declaredKind)`（explorer_erm.go:76） |
| 单值扁平的 per-strategy 阈值 | `thresholdForKind(kind, complexity)`（explorer_erm.go:365）—— **二维表，per-kind × per-complexity，18 个配置值** |
| `CheckStepSatisfaction(...)` | `checkRequirementSatisfaction(reqs, notes, evidence, complexity)`（explorer_erm.go:408） |
| Step-gap hint 渲染 | `ermUnsatisfiedGaps(reqs)`（explorer_erm.go:645） |
| Status 状态机 | ERM 已有 `Status: unsatisfied → partial → satisfied` |

## 另一个问题：阈值表退化

新的 per-strategy 阈值是扁平单值（`TraceChain >= 3`、`Enumerate >= 3`、
`VerifyCondition >= 2`）。ERM 的 `thresholdForKind` 已经编码了
**complexity-scaled** 阈值：mechanism 在 complex 下需要 4，moderate 下只需 2；
call_chain 在 complex 下需要 3，moderate 下需要 2。直接合入会把这张二维表
退化成一维，丢掉 CLAUDE.md 里记录的 T1c（complexity-aware ERM 阈值）。

## 红线审计命中（与重复问题独立）

- `investigation_planner.go:218-223` —— decomposer 里硬编码 13 条 ZH/EN
  relational-verb cue 表
- `investigation_checker.go:163-168` —— verify_condition checker 里硬编码
  12 条 ZH/EN verdict-pattern cue 表
- `investigation_checker.go:130-133` —— 注释原话承认 `StrategyTraceChain >= 3`
  是针对失败 query 的 2-hop A→B→C 结构校准的。命中红线 #6
  （阈值为已知失败 query 专门调的）

## 真正需要的是：~60 行 ERM 增量 patch

你这个 PR 里相比 ERM 真正新增的只有两条能力，值得保留：

1. **sequential step ordering** —— 给 `EvidenceRequirement`
   （explorer_erm.go:38）加 `DependsOn []string` 字段，explorer 循环
   按 ready 状态依次 dispatch，不再一次性并列。
   预计 struct 上 10 行 + dispatch loop 20 行。

2. **retry-triggered refinement** —— 给 `EvidenceRequirement` 加
   `RetryCount int`；扩展 `checkRequirementSatisfaction`，在 retry > N
   时把 stalled requirement 拆成 per-entity 的子 requirement（共享同一
   `Kind`）。~40 行，复用已有阈值表。

加起来 ~60 行叠在 ERM 之上。不需要新枚举、不需要新数据模型、不需要新文件，
也不退化阈值表。

## 测试

你写的测试用例保留 —— 它们覆盖了 ERM 测试没有覆盖到的场景（multi-step
satisfaction、replan 触发）。请把它们迁移到 ERM API 上
（`extractEvidenceRequirementsWithHint` + 增补字段后的
`EvidenceRequirement`）。测试结构和断言都留着，只换构造函数和字段名。

## 下一步

请基于 `explorer_erm.go` + `requirement_kind.go` 重写。如果 ERM 缺了你需要的
概念，欢迎扩展现有 enum/struct，不要做平行子系统。你已有的测试能兜底 ——
如果 ERM 撑不住新流程，测试会暴露出来，这也是有用信号。

这个分支上最近两次合入（f9b267c 六项修复、6fdff2e conditional-enumeration
hint）都是直接 patch 到既有链路上的。这次也请延续这个增量模式，不要
平行新建。
