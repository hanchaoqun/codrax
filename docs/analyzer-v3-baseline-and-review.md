# Analyzer v3 — Baseline 留底 & 模块 Review（B0 补充）

> 对应 HEAD：`c101dd1` (B4b 完成)
> 对应方案：`docs/analyzer-v3-refactor-plan.md`
> 目的：B5 破坏性改动前置条件——baseline 固化 + 三模块过审。

---

## 1. Baseline 留底

### 1.1 等价性论证

B1–B4b 的 6 个提交全部是**纯增量**：

```
d97bda8..c101dd1  25 files changed, 5082 insertions(+), 0 deletions(-)
```

全部 25 个文件都是新文件，且位于 `internal/types/analysis_ir.go` 和
`internal/analysis/{normalizer,compiler,risk,hdp,counterfactual,gate,contract}/`
之下。逐一核查，它们**没有被任何 runtime 路径 import**：

- `main.go` 未引用；
- `internal/orchestrator/*` 未引用；
- `internal/agent/*` 未引用；
- `internal/context/builder.go` 未引用；
- `internal/repl/*` 未引用。

因此，`codrax` 二进制在 `d97bda8` 与 `c101dd1` 下的运行时行为**字节相同**。
任何针对 df1/df3/t1–t5 的评测结果都可以直接跨越这两个 HEAD 引用。

### 1.2 HEAD `c101dd1` 基线数字

直接采纳 `d97bda8` 附近的最近一次完整评测（5 samples/case）：

| 案例 | 评测目录 | 5-run 通过率 |
|------|---------|------|
| df1 | `eval/results/df1-20260413-092900` | **5/5** |
| df3 | `eval/results/df3-20260413-093332` | **5/5** |
| t1  | `eval/results/t1-20260413-094136`  | **5/5** |
| t2  | `eval/results/t2-20260413-095714`  | **5/5** |
| t3  | `eval/results/t3-20260413-100717`  | **5/5** |
| t4  | `eval/results/t4-20260413-101714`  | **5/5** |
| t5  | `eval/results/t5-20260413-102535`  | **5/5** |

**基线 = 35/35 PASS**。B5 及后续任一破坏性批次若使该基线下降，立即回退
相应批次；若持平或上升，方可继续前推。

### 1.3 注意事项

- 所有案例均为 `EXPECT_CONTAINS`/`EXPECT_NOT_CONTAINS` 子串门，非 20 分精度
  rubric。PASS ≠ 答案精度 20/20。B5 后若基线仍是 35/35 但平均答案质量
  下降，需要靠新增的 analyzer v3 eval（见方案 §9）发现。
- 5 samples/case 足以过滤大部分 LLM 方差，但单个 FAIL 仍可能是 outlier。
  B5 后若首次评测出现 1 个 FAIL，**不立即回退**——先跑第二次 5-run 复核，
  两次都 FAIL 再触发回退。这一点与记忆中的"LLM variance is not a root
  cause excuse"原则并不冲突：方差允许一次复核，但不允许重复复核直到
  凑出 PASS。
- 若 B5 后需要完整重跑 35 次评测，预估耗时 ~35 分钟（单 case ≈ 5 min
  × 5 runs / 并行度），LLM 开销可观但可承受。

---

## 2. 三模块 Review

目标：在 B5 前发现纸面假设与本仓库真实语义的偏差，小补丁形式（B4c）
修复，避免把问题推到破坏性批次里放大。

---

### 2.1 `normalizer/rules_zh.go` — 翻译表 review

**当前 43 条条目**，按域分五类：agent 类（9 条）、流程类（14 条）、
运行时类（11 条）、风险类（6 条）、其它（3 条）。

#### 2.1.1 发现的问题

**P1-1**（中）`"解释" → "explain"`：本仓库的 `IntentExplain` 是一个分析意图
类别，但在 normalizer 层把"解释"映射到 `en:explain` 会污染 TermGraph——
它不是搜索关键词，也不是代码符号。当用户问"解释 explorer 如何工作"，
normalizer 会同时产出 `zh:解释` + `en:explain` 两个 canonical term，后者
被 compiler.hintsFromRM 当 keyword 喂给 explorer，导致无意义检索。

  **建议修复**：从 `zhToEn` 中删除 `"解释"` 条目，让 `解释` 作为纯 concept
  留在 `zh:解释`（后续不会被 hintsFromRM 拉进 EntityIDs，因为它是 concept
  而非 symbol）。同理审查 `"停止"` 和 `"验证"`——前者是概念词，后者有可能
  对应代码符号也可能是概念词；保守策略：两者都保留 concept 形态，移出翻译表。

**P1-2**（中）缺失高频仓库术语：本仓库高频但未覆盖的中文术语包括
`"探索/探测"（probe）`、`"证据链"（evidence chain）`、`"跳数/跳步"（hop）`、
`"打分/排名"（rank）`、`"门控"（gate）`、`"召回/精度"（recall/precision）`、
`"兜底/回退"（fallback）`、`"线程安全"（thread safety）`、`"幂等"（idempotent）`。
这些词在 memory 的 feedback 条目和 project 条目里反复出现。

  **建议修复**：补充这 9 个术语到翻译表。每条都有 memory 记录支撑，不算
  "假设性"条目。

**P1-3**（小）`"数据完整" → "data_integrity"`：key 是断句，不是词。用户
更可能写 `"数据完整性"`（4 字），normalizer 的 Han run 提取会产出
`"数据完整性"` 而非 `"数据完整"`——翻译永远不会命中。

  **建议修复**：改 key 为 `"数据完整性"`。

**P1-4**（小）`"锁" → "lock"`：单字 Han 条目违反 `extractHanRuns` 的
`len(runes) < 2` 过滤——这条永远命中不了。

  **建议修复**：删除 `"锁"`，或改为 `"文件锁"/"互斥锁"` 这类双字组合。

**P1-5**（信息）`"流水线" → "pipeline"`：本仓库语境下 `pipeline` 是
orchestrator 的动名词特征，不是流水线硬件概念。翻译无问题，但要注意
alias graph 里 `流水线 → en:pipeline` 的 instanceof 边权重不宜过高，
否则会把无关搜索结果顶上来。当前 `buildAliasGraph` 给翻译边 0.85 置信度
是合理的。

#### 2.1.2 修复优先级

| 项 | 优先级 | 拟议动作 | B4c 必修？ |
|----|-------|---------|-----------|
| P1-1 | 中 | 删除 `解释/停止/验证` 3 条 | ✅ |
| P1-2 | 中 | 补 9 条 | ✅ |
| P1-3 | 小 | 改 key 为 `数据完整性` | ✅ |
| P1-4 | 小 | 删除 `锁` | ✅ |
| P1-5 | 信息 | 不改动 | ❌ |

---

### 2.2 `compiler/templates.go` — 六模板粒度 review

#### 2.2.1 节点粒度统计

| 模板 | 节点数 | 必备节点 | 推测的 ReAct 迭代预算 |
|------|-------|---------|-----|
| architecture_explain | 4 | probe, evidence, reconcile, finalize | 20 |
| root_cause | 5 | probe, evidence, validate, reconcile, finalize | 24 |
| security_audit | 5 | probe, evidence, validate, review, finalize | 26 |
| refactor_design | 5 | probe, evidence, design, review, finalize | 22 |
| config_trace | 4 | probe, evidence, validate, finalize | 14 |
| performance_bottleneck | 4 | probe, evidence, validate, finalize | 18 |
| generic | 3 | probe, evidence, finalize | 16 |

#### 2.2.2 发现的问题

**P2-1**（大）`architecture_explain` 遗漏 `validate` 节点：架构解释类问题
（df1/df3/t1–t5 全部属于此类）在本仓库的 S1/S2/S3 三层修复和 Phase 4
shape-gate 的记忆中都强调"必须校验符号集闭合"。当前模板 `probe → evidence
→ reconcile → finalize` 走的是"reconcile 直接收敛到 finalize"的路径，
没有假设证伪步骤，等同于回退到 pre-L0-1 的无验证终结。df1（5/5）能通过
只是因为当前 runtime 的 S3 symbol validation 在 finalizer 内部兜底，
B5 把 finalizer 改成纯翻译模式后这个兜底会消失。

  **建议修复**：在 `architecture_explain` 的 reconcile 前插入 `validate`
  节点，与 `root_cause` 同构。这样 B5 后 finalizer 失去 S3 retry loop
  时，DAG 层的 validate 节点会接棒。

**P2-2**（中）`config_trace` 的 validate 与 evidence 之间没有
`validation_feedback` 边：当 validate 发现追踪链断裂时无回溯路径，会
直接 fail-through 到 finalize。

  **建议修复**：补一条 `validate → evidence` 的 `EdgeValidationFeedback`
  边，guard=`chain_incomplete`。

**P2-3**（中）`refactor_design` 的 `AcceptanceTests` 硬编码
`contains_symbol: rollback`：本仓库的 refactor 有时是纯重命名，没有
rollback 概念（Analyzer v3 本次重构就是一例——无历史兼容，不需要
rollback 方案）。这个 acceptance 会误伤合法答案。

  **建议修复**：把 acceptance 改为"包含以下任一词：rollback | revert |
  回滚 | 回退 | 撤销"——或者删除硬编码，让 LLM 的 RequestModel 层决定
  acceptance。保守选择：删除硬编码。

**P2-4**（小）`defaultBudget` 的 `MaxToolCalls = MaxReactIters × 2`：
`MaxReactIters` 是 ReAct 迭代数，每次迭代可能产出**多个** tool call
（典型 1–3 个），×2 系数偏紧，容易在 complex 问题上触发工具预算耗尽。
当前运行时 `pipelineMaxSteps=15` 是单独的 step 预算，MaxToolCalls 未来
若接入 orchestrator 会成为第二把锁。

  **建议修复**：`MaxToolCalls = MaxReactIters × 4`，留出余量。

**P2-5**（信息）七个模板的 `SourceMix` 权重（grep/repomap/read）都是手
填的"大致合理"数字，未与现有 explorer 的实际工具使用分布对齐。不是
bug，但 B5 接入 orchestrator 前值得把现有 explorer 一轮跑下来的工具分布
作为模板的 calibration 数据。

  **建议修复**：B5 之后做 calibration，不在 B4c 处理。

#### 2.2.3 修复优先级

| 项 | 优先级 | 拟议动作 | B4c 必修？ |
|----|-------|---------|-----------|
| P2-1 | 大 | `architecture_explain` 补 validate 节点 | ✅ |
| P2-2 | 中 | `config_trace` 补 validation_feedback 边 | ✅ |
| P2-3 | 中 | 删除 rollback 硬编码 acceptance | ✅ |
| P2-4 | 小 | `MaxToolCalls = MaxReactIters × 4` | ✅ |
| P2-5 | 信息 | 延后到 B5 后 calibration | ❌ |

---

### 2.3 `risk/matrix.go` — 风险关键词表 review

#### 2.3.1 当前 41 条关键词分类

| 维度 | 条目数 | 最高 Level |
|------|-------|-----------|
| security | 11 | 4 |
| data_integrity | 8 | 4 |
| compatibility | 5 | 4 |
| performance | 7 | 3 |
| ops | 3 | 3 |
| compliance | 4 | 5 |

#### 2.3.2 发现的问题

**P3-1**（大）`en:migration` → `data_integrity=4`：问题在于"migration"
在本仓库语境下**有两种完全不同的含义**：

  1. 数据库/schema 迁移（真风险，level 4 合理）；
  2. Analyzer v3 refactor 方案中的"**迁移批次 B1–B9**"（零运行时风险——
     这是重构步骤）。

  当前表会把所有讨论 "migration batches" 的请求当作数据完整性高风险处理，
  触发 `require_verify=true`，让一个纯文档规划请求跑完整 verify 阶段。

  **建议修复**：把 `en:migration` 降到 `data_integrity=2`，或者改为
  `{dim: data_integrity, level: 2}` + 附加条件"当且仅当 term graph 同时
  包含 `en:schema` / `en:database` 时升到 4"。简化实现：仅降到 2，复合
  条件推到 B5 的 LLM 风险推理层。

**P3-2**（中）`en:delete` → `data_integrity=3`：本仓库代码里 "delete"
是高频普通词（`delete` 操作、`delete` 方法、`Delete` RPC）。把它设为 3
会导致**任何**涉及删除操作的 refactor 都被误判为数据完整性风险。

  **建议修复**：降到 `data_integrity=1`，仅作为弱信号。真正的数据丢失
  风险由 `drop` / `truncate` / `purge` 这类强破坏性词承担。

**P3-3**（中）`en:audit` → `compliance=2`：本仓库的 `audit` 一词更多出现
在"audit trail"/"审计日志"/"对 explorer 做审计"这类用法里，是**调查
行为**而非合规要求。当前映射会让任何讨论审计的请求挂上 compliance 风险。

  **建议修复**：降到 `compliance=1`，或者要求必须和 `gdpr|pii|sox|hipaa`
  之一共现才触发 compliance 维度。简化实现：降到 1。

**P3-4**（中）`en:api` → `compatibility=2`：过宽。API 讨论 95% 情况下
是在问 API 如何工作，不是在改 API。

  **建议修复**：只有 `en:api` + `en:version` 共现才升级 compatibility。
  当前无共现逻辑——简化实现：降到 `compatibility=1`。

**P3-5**（小）`en:backup` → `data_integrity=3, ops=2`：本仓库几乎不会
讨论 backup；但即使讨论，`backup` 通常是**缓解**数据完整性风险的手段，
不是风险本身。逻辑反了。

  **建议修复**：删除该条目。

**P3-6**（小）缺失 security 高频词：`sql injection`/`xss`/`csrf`/
`path traversal`/`race condition` 这些现代安全审计的主力词未覆盖。

  **建议修复**：B4c 暂不补，等 B8 的 analyzer v3 eval 发现真实 miss
  再加。避免"凭空增加"触发 overfit 审计。

**P3-7**（小）中文对等覆盖不全：security 维度只有 `认证/密码/令牌` 3 条
中文对等，data_integrity 只有 `迁移/删除` 2 条，其它维度（compatibility/
performance/ops/compliance）**完全没有中文条目**。

  **建议修复**：B4c 最小补齐：`兼容性/升级/回滚/监控/审计/合规`。

#### 2.3.3 修复优先级

| 项 | 优先级 | 拟议动作 | B4c 必修？ |
|----|-------|---------|-----------|
| P3-1 | 大 | `en:migration` 降到 2 | ✅ |
| P3-2 | 中 | `en:delete` 降到 1 | ✅ |
| P3-3 | 中 | `en:audit` 降到 1 | ✅ |
| P3-4 | 中 | `en:api` 降到 1 | ✅ |
| P3-5 | 小 | 删除 `en:backup` | ✅ |
| P3-6 | 小 | 暂不补，等 eval 发现 | ❌ |
| P3-7 | 小 | 补 6 条中文 | ✅ |

---

## 3. B4c 补丁 scope 总览

**Review 一共发现 14 个可行动项**，其中 **12 个列为 B4c 必修**：

| 来源 | 必修项数 | 预计代码改动量 |
|------|---------|---------------|
| rules_zh | 4 项 | ~15 行（+/- 合计） |
| templates | 4 项 | ~30 行 |
| risk matrix | 6 项 | ~15 行 |
| **合计** | **14 项** | **~60 行** |

所有改动都在已存在的 B2/B3/B4a 文件内，**不新增文件、不触碰 runtime**。
B4c 完成后 35/35 baseline 应仍然成立（因为这些包仍未接入 runtime）。

B4c 提交后即可进入 B5。

---

## 4. B5 前置检查清单

| 项 | 状态 |
|----|------|
| baseline 35/35 固化在文档中 | ✅（§1.2） |
| baseline 等价性已证明 | ✅（§1.1） |
| `normalizer/rules_zh.go` review | ✅ 本文 §2.1 |
| `compiler/templates.go` review | ✅ 本文 §2.2 |
| `risk/matrix.go` review | ✅ 本文 §2.3 |
| B4c 补丁 scope 与预估 | ✅ §3 |
| 用户明确授权破坏性改动 | ⏳ 待确认 |

以上全部打钩后方可进入 B5。
