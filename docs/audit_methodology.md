# 系统级审计方法论(Audit Methodology)

> **目的**:把 2026-05-02 / 2026-05-03 累积的十一轮 AuthorityCeiling axis 审计经验固化为可复用的方法论。任何不熟悉系统的工程师按此文档可以独立完成全面审计,确保系统在迭代中不退化。
> 
> **适用范围**:codrax 整个产品体系,包括(但不限于)read-mode 答案管线、write-mode 计划管线、log/trace 接入、agent skill prompt、taxonomy 跨 Run 学习、AnswerDocument 渲染。
>
> **核心原则一句话**:**系统的每一个判断都必须能解释为"在追随用户问题意图,用单一真理源,跨场景对称,跨数据流闭环"**。任何违反这一原则的代码点都是潜在断裂。
> 
> **本文不是 checklist。它是思维方法。Checklist 在第 8 节,但只在你真正理解前面的原则之后才有意义。**

---

## 1. 第一性原理(First Principles)

### 1.1 系统侧"硬假定"是一切退化的根源

任何系统侧基于 artifact 内容形态(panic/crash 信号、文件后缀、行号是否齐全、stall vs frame、log vs trace)做出的"我应该如何响应"决策,都是**硬假定**。硬假定迟早会因为:

1. 用户附了系统没预料到的 artifact(timeout log,unsymbolicated trace)
2. 用户问了系统没预料到的问题(用户附 panic log 但问"trace this call path")
3. artifact 被截断 / 残缺 / 不规则 / 多份混合

而漏失。

**反"硬假定"原则**:系统对 artifact 内容形态完全不假定,只对"用户问题意图"做判断。用户问题意图的唯一来源是 LLM 的 `emit_analysis` 分类(`rm.Intent` enum),系统的所有响应分支必须 hook 在它身上。

### 1.2 单一真理源(Single Source of Truth)

每一个语义维度只能有一个权威源。例子:

| 维度 | 权威源 | 反模式 |
|---|---|---|
| 用户问题意图 | `rm.Intent`(LLM 分类) | 系统从 artifact 内容反推 intent |
| 漂移识别 | `EvidenceItem.Authority/Origin/DriftReason`(emit 时投影) | 老 `collectLogSourceAnchors` 在 finalize 时再算一遍 |
| 漂移 anchor | derived from emitted evidence | 平行的两套检测逻辑 |
| 系统注入标记 | `authorityCaveatTag`(私有 token) | 公开前缀 `Authority:` |

**红线**:任何"派生/平行"的二号真理源都必须最终消灭(refactor 成第一源的 derived view)。

### 1.3 跨场景对称(Cross-Scenario Symmetry)

某个能力在场景 A 工作,场景 B 必须等价工作。如果不行,要么是场景定义错(B 不该是 A 的对称),要么是代码缺了 B 的接入。

11 个对称维度(每个都查):

| 维度 | 对称单位 |
|---|---|
| **Artifact 通道** | log / perf trace / mixed / no-artifact |
| **多 artifact** | single bundle / multi-log / multi-trace / log+trace mix |
| **用户意图** | RootCause / Trace / Explain / Enumerate / ConfigQuery / ReturnValue / Unknown |
| **答案 shape** | step_list / list_of_symbols / value / config_value / boolean / explanation |
| **Frame 形态** | file+line / file only / symbol only / unresolved |
| **Signal 类** | panic / crash / oom / timeout / permission / db / network / validation / logic / performance / other |
| **Drift 状态** | None / LineDrift / TailRename / FileMoved / Unmappable / Unknown |
| **运行 phase** | first attempt / retry / IsZero fallback |
| **数据 lifecycle** | normal / cross-task / cross-run REPL turn |
| **语言** | English / Chinese (zh) / fallback |
| **agent 角色** | analyzer / explorer / extractor / finalizer / reviewer |

### 1.4 跨数据流闭环(End-to-End Closure)

数据从产生 → 投影 → 持久化 → 消费 → 渲染 → 反馈学习,**每一段都不能丢、不能漂、不能撞**。整段闭环七节:

```
Input(用户输入 + artifact)
    ↓ [identify]
Identity(StableEvidenceID 含所有语义字段)
    ↓ [project]
Projection(emit 时一次性投影到全部 axis 字段)
    ↓ [backfill]
Backfill(任何 bypass 路径在边界点补投影)
    ↓ [derive]
Derive(派生 view 而非重新计算)
    ↓ [render]
Render(消费 view,语言/术语/标记 intent-neutral)
    ↓ [strip]
Strip(下游 reviewer 看不到系统注入)
    ↓ [gate]
Gate(校验 invariant,不烧用户 retry 预算)
    ↓ [learn]
Learn(过滤 plumbing 噪音,只学 user-quality 信号)
```

**红线**:每一节的输出都是下一节的输入。任何节的输出格式或语义改变,所有后续节都必须连带审视。

### 1.5 LLM 看到的信息必须清晰准确

LLM 是系统的另一个用户。它读 prompt、看消息历史、emit 工具调用、按 retry hint 调整。**LLM 视角和系统视角必须一致**:

- 系统注入 LLM 的标记(hedge marker / tag),LLM 看到必须知道是系统所有,不是它写的
- 系统给 LLM 的 retry hint 必须是 LLM-actionable(命名 tool / shape / required field),不是内部组件名
- 系统在 conversation 历史中保留的内容,如果可能被 LLM 误读为"我之前写过这个",必须 strip
- 系统在 skill prompt 中告诉 LLM 的契约必须与代码现状一致(comment-vs-code drift)

---

## 2. 审计的优先级维度(Priority Dimensions)

按用户口头红线:**优先审视以下五维度**,从最高优先到最低:

### 2.1 答案不丰富(Answer Richness Regression)

**症状**:答案变薄、变模糊、变保守。具体表现:

- LLM 在 retry 中缩短 mechanism 解释("反正系统已经 hedge 过了")
- 用户合法散文被 system strip 误剥(如"(in older Go versions) X happens"被当 stale system reason 砍掉)
- Hedge marker 重复 N 次稀释正文比例
- Symbol 表格 / 步骤列表的 Rationale 被插入过长 hedge 文字撑爆列宽
- Bypass-path 项目(concrete_value / mechanism_scan / bridge_literal)Authority="" 被当 factual 等同算,洗白漂移证据
- Tier-1 floor 把 Authority=Illustrative 项算入 → 满足 floor 但其实是 illustrative 没价值
- Filter 把"无行号"的 frame / Func / Signal / Residue 整段丢弃,信息被压制

**审计角度**:对每条 LLM-facing prose 字段(Summary / Step.Description / Symbol.Rationale / Boolean.Rationale / Caveats[]/ Diagram fence),问"这条字段在 N 个跨场景中是否仍能容纳同等丰富的 user-question-relevant 内容?"

### 2.2 轮次空转浪费(Wasted Retry Rounds)

**症状**:retry 触发但每轮无实质进步。具体表现:

- Strict gate fire,但 LLM 没有 actionable handle 修复,retry 也只是重复同样错误
- Reviewer dispatch fire 但 LLM 输入是 plumbing 噪音,LLM 学不到东西
- Salvage 路径把上一轮 polluted draft 拉回当前轮,再被 strip,再被 inject,无限循环
- Idempotency 判定过严(如 alreadyHasHedgePrefix)阻止 ceiling upgrade,导致 stale label 持续到下一轮被 reviewer 判矛盾再触发新 retry
- Threshold 计数包含了 plumbing 类违反,导致 plumbing-only 也跨阈值触发然后 abort

**审计角度**:对每个 retry 触发点,问"这次 retry 给 LLM 的 hint 是它能照做的吗?LLM 照做后会产生与上轮不同的输出吗?如果不会,这次 retry 就是空转。"

### 2.3 闸门前后矛盾导致模型无法自愈(Gate-vs-Gate Contradictions)

**症状**:多个独立 gate 互相冲突,LLM 满足一个就违反另一个。具体表现:

- gate A 用 OLD 检测系统判别,gate B 用 NEW 检测,两者结论不一致
- Reviewer 看到的 prose 含系统注入 marker,把 marker 算作 LLM 内容差异 → 误判矛盾 → 触发 retry → 重新 inject → 再被判 → 死循环
- Token-match check 看到 hedge body 中的 "log/perf/drift" 字符,假阳性认为 LLM 已 decode bundle
- citation_floor relaxation 走 OLD 漂移系统,hedge 走 NEW Authority,两边对同一答案给反方向判断
- Salvage 在 hedge 之前从 conversation 拉 polluted text,hedge 后再 inject;下轮 LLM 看到自己"写过"marker,在新 emit 中也写

**审计角度**:列出所有 gate / reviewer / 校验 / 阈值 / strip 点,做矩阵图——每对组合是否使用了同一真理源?是否使用了同一术语?是否在同一时机点读取数据?

### 2.4 闸门没对齐用户问题意图(Intent-Misaligned Gates)

**症状**:gate 基于 artifact 内容形态或 system 派生 Scenario 触发,而不是 user 意图。具体表现:

- `Scenario == ScenarioRootCause` 硬卡(Scenario 部分系统派生)→ 用户 IntentTrace 但 Scenario 不是 RootCause → drift mode 不触发 → 用户损失意图最相关信息
- `LogBundle.Meta.Signals contains panic` 触发某种处理 → 用户问的不是 panic 类问题 → 系统强加 panic-related 处理
- `frame.Line > 0` 才参与 derive → 用户问"哪个函数被调",frame 只有 Func 没 Line 也是有效的 entity → 系统按行号过滤丢掉
- gate 用 LogBundle.IntentHint 覆盖 rm.Intent → 系统 hint 压过用户实际意图

**审计角度**:每个 gate / 分支 / 渲染分流,问"这个判断的输入是 `rm.Intent` 吗?如果不是,它派生自什么?这个派生是否会与 `rm.Intent` 不一致?如果会,以谁为准?"

### 2.5 闸门过窄压制了用户最相关信息(Over-Narrow Filters)

**症状**:gate 阈值/容量上限 / 黑白名单太严,正常用户场景被 filter 在外。具体表现:

- frame cap 16 → 用户附 200 行 panic stack,只看前 16 → 用户最相关 frame 被截断
- hedge sentinel grep 用 prefix 匹配 → LLM 写的同 prefix 内容被误当系统 marker
- isPlumbingFailureViolation 列表只有一个 member,新加的 plumbing kind 不在内会被算入 learning
- `isLLMEmittable` 白名单遗漏新加的 EvidenceKind → emit 被 schema 拒绝
- 系统术语 hard-coded ("panic", "dereference", "this is a [X]")在非 panic 场景措辞失真
- LogSignal 11 种但 prose 语法只适配 panic/crash/oom 三种,其他 8 种读起来别扭

**审计角度**:对每个 filter / cap / 白名单,枚举它"漏掉了什么"。把跨场景矩阵的每个格子代入,看是否有格子的最相关信息被这个 filter 砍掉。

---

## 3. 审计的方法学(Audit Method)

### 3.1 数据流追踪法(Data Flow Tracing)

针对一类数据(如 EvidenceItem),追踪它在系统中的全生命周期:

1. **产生点**:谁第一个创建这个数据?有几条产生路径?
2. **投影点**:产生后哪些字段被加工?加工是否所有路径都覆盖?
3. **持久化点**:数据写入哪里?跨 task / 跨 run 的 reset 如何?
4. **消费点**:谁读这个数据?读时数据已经经过哪些加工?
5. **渲染点**:用户/LLM 看到的形式是什么?
6. **反馈点**:数据是否进入跨 Run 学习?学习筛选后的内容是否准确?

**关键检查**:
- 每条产生路径是否都触达完整投影?(bypass paths!)
- 每个消费点读到的数据是否携带产生时的全语义?(被 strip / 截断 / dedup 了吗?)
- 跨 task / 跨 run 时数据是否被正确 reset 或保留?

### 3.2 跨场景矩阵法(Cross-Scenario Matrix)

把第 1.3 节的 11 个对称维度做笛卡尔积。**实际审计中不必每个格子都验证**,但要对每个维度选 2-3 个有代表性场景跑一遍数据流。

例子:针对"答案不丰富"维度审计,挑这些场景:

| Test Case | Artifact | Intent | Shape | Frame 形态 |
|---|---|---|---|---|
| C1 | log only(panic 类) | RootCause | step_list | file+line |
| C2 | log only(timeout 类) | Explain | explanation | file+line |
| C3 | perf only | Trace | explanation | symbol only |
| C4 | log+perf | RootCause | list_of_symbols | mixed |
| C5 | no artifact | RootCause | explanation | N/A |
| C6 | log only | ConfigQuery | config_value | file+line |
| C7 | perf only(unsymbolicated) | Trace | step_list | symbol only |

每个场景都跑数据流追踪,问"答案丰富度有损失吗?在哪一节?"

### 3.3 红线对照法(Red Line Audit)

把已知红线(项目 CLAUDE.md 中、用户 feedback 中、十一轮累积的)逐条作为 query,grep 代码:

- "no system hard assumption"
- "align with user intent"
- "preserve information without line"
- "no keyword matching"
- "system-private vs user-written"
- "comment-vs-code drift"

红线是"绝对不允许的代码模式"。grep 找潜在违反点,每个点核对意图是不是 vs 红线冲突。

### 3.4 名字-行为对照法(Name-vs-Behavior Audit)

代码标识符的名字暗含语义假定。当行为发生变化但名字没变,**未来读者会被名字误导**。

具体检查:
- 函数名是否仍精确描述当前行为?(如 `RenderDriftBoundedCurrentRootCauseSummary` 在 R9 后也用于 IntentTrace,name 含 "RootCause" 但 behavior 不限于 RootCause)
- 注释 comment 是否描述当前实现?(如 violation.go 中说 gate 检查 sentinels,但 R6 后 gate 检查 caveat tag — 注释陈旧)
- 字段名 `LogSourceDriftAnchor` 在 R8/R9 后也包含 perf-derived anchor → name 误导
- 常量 / 变量 名 是否暗示了过期的语义?

### 3.5 LLM 视角模拟法(LLM Perspective Simulation)

读 LLM 看到的全部 prompt + conversation history + retry hint + tool result,模拟 LLM 在 retry 中怎么思考:

1. LLM 看到的 prior assistant turn 含什么?是否含 LLM 没写过的 system 注入?
2. LLM 收到的 repair hint 是 actionable 还是 plumbing 名词?
3. LLM 在 emit 时看到的 schema 是否与系统 silently 注入的字段冲突?
4. LLM 收到的 caveat/marker 是否清楚标识为系统所有?

**测试方法**:把所有 LLM-facing 字符串(skill prompt / repair hint / status message / tool description)放在一起读,问"如果我是 LLM,我读完这些会做什么?"

### 3.6 闭环差分法(Closure Diff)

每次系统重大变更(新加 axis、新加 enum 值、新加 stage、新加 tool)后,对比变更前后的 7 节闭环每节是否都同步更新:

| 7 节 | 变更前 | 变更后 | 是否同步? |
|---|---|---|---|
| Identity | StableEvidenceID 字段集 | + Authority + Origin | ✓(R4) |
| Projection | ComputeForEvidence 返回三元组 | + DriftReason | ✓(P1) |
| Backfill | AppendEvidence 钩子 | 同左 | ✓ |
| Derive | OLD detector 直接计算 | NEW derive from projection | ✓(P2/P3) |
| Render | hedgeSteps 用 marker | + hedgeSummary + hedgeBoolean + caveat | ✓(R2/R5) |
| Strip | LLM 输入 strip | + reviewer / Caveats / ParseOutput | ✓(R3-R6) |
| Gate | per-marker 检查 | caveat-tag 检查 | ✓(R6) |
| Learn | 全 violation 学 | filter plumbing | ✓(R6/R10) |
| User-intent | Scenario 硬卡 | rm.Intent | ✓(R9) |

**任何一节未同步 = 断裂点**。

---

## 4. 审计的具体维度清单(Dimension Catalog)

每次审计至少覆盖以下维度。维度顺序按"哪里最容易藏 bug"排列。

### 4.1 数据 Identity 维度

- [ ] 数据的 stable hash / dedup key 是否覆盖所有语义字段?新加字段后 hash 是否更新?
- [ ] 跨 task / cross run 的 reset 是否同步覆盖新字段?
- [ ] 跨进程序列化(JSON / blob)是否保留新字段?

### 4.2 投影 Projection 维度

- [ ] 数据产生路径有几条?所有路径是否都触达投影?
- [ ] Bypass / fast path / 测试 fixture 路径是否需要 backfill?
- [ ] 投影是 pure function 吗?同输入是否同输出?
- [ ] 投影读取的依赖(graph / bundle / config)在投影时刻是否已就绪?

### 4.3 Derive / View 维度

- [ ] 是否存在两套独立的"派生路径"?(老系统 + 新系统并存)
- [ ] 派生函数是否从单一真理源读取?
- [ ] 派生函数的输入数据 cap 是否过窄?(如 16 frame cap)
- [ ] 派生函数对零输入 / 不规则输入(无 file / 无 line / 残片)的行为是否符合用户意图?

### 4.4 Render 维度

- [ ] Render 措辞是否 intent-neutral?
- [ ] Render 措辞是否 artifact-neutral?(log/perf/mixed/none)
- [ ] Render 措辞是否 signal-neutral?(11 种 LogSignal 都顺口)
- [ ] Render 输出长度对答案 token budget 是否合理?(hedge body 不挤压正文)
- [ ] 多语言 prose(zh/en)是否对应一致?
- [ ] 函数名 / mode 名 是否反映泛化后的行为?

### 4.5 Strip 维度

- [ ] 系统注入的 token 在所有下游消费点都被 strip?(reviewer / 学习 / 持久化 / conversation)
- [ ] Strip 是 system-match-only 还是宽松匹配?(宽松匹配会误剥用户散文)
- [ ] Strip 在多语言下都覆盖?(中英括号 / 长度限制)
- [ ] Strip 后下游消费点看到的 prose 是否仍能完成它的判断?

### 4.6 Gate 维度

- [ ] Gate 的输入是 single source of truth 吗?
- [ ] Gate 触发条件与下游 repair / dispatch 的语义是否一致?(threshold semantic = dispatch semantic)
- [ ] Gate fire 时给 LLM 的 hint 是 LLM-actionable 还是 plumbing?
- [ ] Gate 是否会与其他 gate 在同一答案上给互斥判断?
- [ ] Gate 是 strict 还是 soft?strict 是否会无限循环?
- [ ] Gate 退路(escape hatch)是否完备?(IsZero fallback / soft 模式)

### 4.7 Learning Loop 维度

- [ ] 进入 cross-Run 学习的 violation/event 是否过滤了 plumbing-class?
- [ ] 学习数据持久化后,下次 Run 注入到 analyzer 时是否仍 actionable?
- [ ] 学习触发阈值的计数是否与 dispatch 实际处理对象一致?

### 4.8 用户意图对齐维度

- [ ] 系统响应分支是否 hook 在 `rm.Intent`?
- [ ] 是否有分支 hook 在 system-derived `Scenario` 而 Scenario 与 Intent 不完全等同?
- [ ] 是否有分支 hook 在 artifact-derived signal(如 `bundle.IntentHint`)?
- [ ] 用户问题意图分类已有 enum,是否被新功能复用而不是新增分类?

### 4.9 Conversation / 状态 lifecycle 维度

- [ ] 跨 task / cross-run 状态是否被正确 reset?
- [ ] LLM 在 conversation 历史中能看到的 prior turn 是否被清洁?
- [ ] Salvage 路径从 prior turn 拉的数据是否被 strip?
- [ ] REPL 多 turn 的 memory 持久化是否包含系统注入?

### 4.10 Schema / contract 维度

- [ ] LLM-facing schema(emitXXX 工具的 JSON schema)是否隐藏系统私有字段?
- [ ] DisallowUnknownFields 是否保护所有写入点?
- [ ] 注释 / 文档 / 测试是否描述当前实现而非历史实现?

### 4.11 Cross-scenario 维度

- [ ] artifact 通道 4 种(log only / perf only / mixed / none)各自是否完整?
- [ ] artifact 多份(multi-log / multi-trace)是否支持?
- [ ] frame 形态 4 种(file+line / file only / symbol only / unresolved)是否都识别?
- [ ] signal 类 11 种各自是否被自然语言 prose 适配?
- [ ] 用户意图 7 种(Intent enum 减 Unknown)是否都有合理响应?
- [ ] retry 状态 3 种(first / retry / IsZero fallback)是否都安全?

### 4.12 名字 / 注释 / 术语一致性维度

- [ ] 函数名是否反映泛化后的行为?
- [ ] 注释是否描述当前实现?
- [ ] LLM 看到的术语(skill prompt / repair / caveat)是否前后一致?
- [ ] 同一概念在不同位置是否同名?(如"drift-bounded"vs"drifted"vs"log-derived"是否需要统一)

---

## 5. 已知红线汇编(Known Red Lines — DO NOT VIOLATE)

由项目 CLAUDE.md memory + 用户 feedback + 十一轮累积。每条红线后括号是它的来源 / 适用范围。

### 5.1 系统行为类红线

1. **不允许系统侧硬假定**:系统行为分支不基于 artifact 内容形态(panic/crash 信号、文件后缀、行号是否齐全)硬决定。(用户红线,R9)
2. **必须对齐用户问题意图**:LLM 已分类的 `rm.Intent` 是单一真理源。(用户红线,R9)
3. **触非用户问题明确命中,否则不能系统硬假设**:重渲染 mode / drift surface / 特殊 prose helper 只在 LLM 把用户问题分类为相关 Intent 时触发。(用户红线,R9)
4. **无行号也不能丢**:log/trace 中无行号的不规则输出/打印,如果对齐了用户意图,在后续流程不能被简单丢弃,必须有合理处理。(用户红线,R9)
5. **不能系统压制用户意图最相关信息**:filter / cap / 阈值不能过窄。(用户红线,R10)

### 5.2 数据流类红线

6. **No-keyword-matching**:不能用关键词列表 / regex 关键词去 hook 用户问题分类。(`feedback_no_custom_keyword_matching.md`,R6)
7. **No-OLD-NEW平行**:同一语义维度只能有一个权威源,不允许两套独立计算逻辑。(R8 Plan A)
8. **No comment-vs-code drift**:注释必须描述当前实现,代码 refactor 后注释同步更新。(R6)
9. **No internal pipeline info in LLM prompts**:LLM-facing repair hint 只用 LLM-actionable 术语(tool name / shape / required field),不暴露内部组件名。(`feedback_no_internal_info_in_llm_prompts.md`,R6)

### 5.3 完整性类红线

10. **每条产生路径都需投影**:bypass / fast path / 测试 fixture 必须有 backfill 兜底。(R3)
11. **跨场景对称必须验证**:新增能力时,所有 11 个对称维度都要 grep 一次。(R8/R9/R11)
12. **System-private vs user-written 必须可区分**:系统注入的 token 必须用 LLM 不可能自然写出的 sentinel(如 zero-width-space tag)。公开 prefix 容易被 LLM 撞车。(R4)

### 5.4 闸门类红线

13. **Gate threshold semantic = dispatch semantic**:阈值过滤逻辑与 dispatch loop 过滤逻辑用同一谓词。(R10)
14. **Strict gate 必须有 escape hatch**:IsZero fallback / soft 模式 / retry 预算上限。(R5)
15. **Plumbing-class violation 不能进 cross-Run 学习**:plumbing 失败信号(渲染绕过 / structured-emit 失败)不应作为答案-质量 pattern 持久化。(R6)

### 5.5 信息呈现类红线

16. **Hedge body 不挤压正文**:per-step 用 marker only,prose 长度只在 doc-level Caveat 一次性出现。(R6)
17. **Strip 必须 system-match-only**:剥除 stale reason 时只匹配系统已知 token,不剥除任意 `(...)` 括号(否则误剥用户散文)。(R7)
18. **Render 措辞 intent/artifact/signal-neutral**:函数名可以保留历史称谓,但 prose 内容不能写死特定 scenario 的词汇。(R11)

---

## 6. 审计 PR 模板(Audit PR Template)

每次审计完成后,提交一个或多个 commit,commit message 用以下结构:

```
fix(<area>): round-N — <一句话总结>

Round-N <审计角度>(用户红线/红线名)发现 M 处<违反类>:

(F1 — <严重性级别>)
<现象描述,具体到代码 file:line>
<为什么这是断裂>

修复:<动作>

(F2 ...)
...

Tests added:
- TestXxx (描述)

Round-N 也核验了以下不变量(不退化):
- ... (列出本轮没动但有依赖的 invariant)

go test ./... + go vet ./... + make all clean.

Co-Authored-By: ...
```

**关键**:每次 commit 描述都要列出"本次未动但依赖的 invariant",这样未来审计可以 grep commit message 还原本轮验证过的对称性矩阵。

---

## 7. 跨轮经验集(Lessons from 11 Rounds)

### 7.1 看似小改其实是结构性改动

- R8 把 perf 加入 derive 看似只加几行,但需要重新设计 Origin 标记传递、render 文案、跨 frame cap → 涉及 4 个文件 + 6 个新测试。
- R9 把 hard-coded `Scenario` 换成 `rm.Intent` 看似只改一行,但需要审视 30+ callsite 是否都该跟着换 → 涉及 3 个文件 + 4 个新测试。

**经验**:任何修改"判断分支条件"的改动都先做跨场景矩阵,确认所有同源判断点都要跟着换。

### 7.2 Strip 是隐藏的 bug 集中地

R7 / R11 显示 strip 函数最容易引入"过宽"或"过窄"问题:

- 过宽 strip 误剥用户合法 prose(R7)
- 过窄 strip 漏剥系统注入(R6 — 系统多语言注入但 strip 只覆盖英文)
- strip 边界(byte limit / 括号匹配)需要适配宽字符(zh)

**经验**:任何 strip 函数必须有"系统注入的 N 种形态全列举"测试 + "用户合法 prose 不被剥"测试。

### 7.3 LLM 视角与系统视角分裂是死循环根源

R5 / R6:LLM 看到自己 prior turn 含系统 marker → 复制 → 系统 strip → 再 inject → ... 死循环。

**经验**:任何系统 inject 的 token 都必须满足:(a) LLM 不可能自然写;(b) 系统在 LLM 看到之前 strip;(c) skill prompt 明确告知 LLM "不要自己写这些"。三者缺一不可。

### 7.4 Plumbing 失败 vs 答案质量失败

R6 / R10:把渲染绕过 / structured-emit 失败这类 plumbing 信号也算入 cross-Run 学习,会污染 analyzer 的 "Known answer pitfalls" 注入。

**经验**:每个 ViolationKind 都明确标记是 "plumbing" 还是 "content"。前者只用于本轮 retry,不进 cross-Run 学习。

### 7.5 Round-1 设计决定的延伸成本

R1 引入 4 值 ClaimOrigin 看似简单,但后续轮要让每个 origin 在 derive / render / strip / gate 各层都有一致的处理(R8 让 perf 是真正一等公民,R9 让 cross_source / log / perf 三个 origin 在 frame-level matching 中正确传播)。

**经验**:基础数据结构变更要穷举所有对称维度,不能只验证 happy path。

### 7.6 多 artifact 的待办

11 轮没有完整解决 multi-log / multi-trace 场景。`MutableState.SetLogTriage(b *LogBundle)` / `SetPerfTrace(b *PerfBundle)` 都是单 setter,后写覆盖前写。Multi-bundle 输入需要 List 化 setter + 全 derive/render/finalizer 链路适配。

**留作未来工作**:见 ROADMAP 章节(本文 9.2)。

---

## 8. 审计 Checklist(实操清单)

> 仅在你已经理解第 1-7 节后使用此清单。

### 8.1 启动阶段

```
[ ] 拉最新 main,确认 git status 干净
[ ] go test ./... 全绿基准
[ ] 阅读 CLAUDE.md 中 axis / drift / Authority 相关章节
[ ] 阅读最近 5-10 个 commit message,理解最新改动
[ ] 确认本轮审计的"用户红线焦点"(若用户给出口头指示)
```

### 8.2 数据流追踪(每个核心数据类型)

```
对 EvidenceItem / AnswerDocument / LogBundle / PerfBundle / Violation 各做一次:

[ ] 列出所有产生点(grep "构造结构体" + "Append" + "Set")
[ ] 列出所有消费点(grep 字段名 + "EmittedXxx" + "Get")
[ ] 列出所有 reset 点(grep "Reset" + "= nil")
[ ] 检查每条产生路径是否触达投影
[ ] 检查每个消费点读取数据时该数据已经过哪些加工
[ ] 检查 reset 是否覆盖所有新加字段
[ ] 检查 JSON 序列化是否保留新字段
```

### 8.3 跨场景矩阵核验

```
针对本轮新引入的能力,挑 7 个有代表性场景(参见 3.2 节):

[ ] 场景 1:log only + RootCause + step_list — 端到端跑数据流
[ ] 场景 2:log only(非 panic 类) + Explain + explanation
[ ] 场景 3:perf only + Trace + explanation
[ ] 场景 4:log+perf mixed + RootCause + list_of_symbols
[ ] 场景 5:no artifact + 任意 Intent
[ ] 场景 6:log + ConfigQuery + config_value
[ ] 场景 7:unsymbolicated trace + Trace + step_list
```

### 8.4 5 大优先维度逐一审视

```
[ ] 答案不丰富(2.1):跨场景每条 prose 字段是否仍能容纳同等丰富内容?
[ ] 轮次空转(2.2):每个 retry 触发点的 hint 是否 LLM-actionable?
[ ] 闸门矛盾(2.3):gate 矩阵图,跨 gate 是否同源?
[ ] 意图对齐(2.4):每个分支判断 hook 在 rm.Intent 还是 system-derived?
[ ] 闸门过窄(2.5):每个 cap / threshold / 黑白名单的"漏掉了什么"
```

### 8.5 红线 grep

```
[ ] grep "Scenario ==" - 是否还有 hard-coded scenario 判断
[ ] grep "Signal" - 是否硬假定 panic/crash signal
[ ] grep "frame.Line <= 0" / "frame.Line == 0" - 是否仍 filter 无行号
[ ] grep "\.IntentHint" - 是否仍读 system-derived intent
[ ] grep "TODO" / "XXX" / "FIXME" - 历史欠债
[ ] grep "panic" 在 prose 字符串中 - 是否硬编码 panic 措辞
[ ] grep "log " 在 perf 相关代码 - 是否 artifact 名错
[ ] 注释 grep "(commit N)" - 是否陈旧引用
```

### 8.6 LLM 视角模拟

```
[ ] 阅读 internal/skill/defaults.go 中所有 prompt 字符串
[ ] 阅读 internal/orchestrator/contract_check.go 中所有 violation Repair 字段
[ ] 阅读 status / event / 日志中所有 LLM-facing 字符串
[ ] 模拟 retry:LLM 看到 prior + hint 后能否产生与上轮不同的输出?
[ ] 模拟 conversation 历史:LLM 看到的 prior assistant content 是否含系统注入?
```

### 8.7 闭环差分

```
[ ] 7 节闭环(Identity → Project → Backfill → Derive → Render → Strip → Gate → Learn)
[ ] 本轮变更是否每一节都同步?
[ ] 列出本轮"未动但依赖的 invariant",写入 commit message
```

### 8.8 收口

```
[ ] go test ./... -count=1
[ ] go vet ./...
[ ] make
[ ] 所有 round-N 测试都加 round 标签命名,便于将来 grep
[ ] commit message 按 §6 模板撰写
[ ] git push origin main
[ ] (可选)如发现"留作未来"事项,加入 §9 ROADMAP
```

---

## 9. ROADMAP(留作未来工作)

记录已知但本轮未解决的事项,避免被遗忘:

### 9.1 Multi-Artifact 支持

**现状**:`SetLogTriage` / `SetPerfTrace` 单 setter,后写覆盖前写。
**目标**:支持用户附多个 log + 多个 trace 同 Run。
**变更面**:Mutable.LogTriageList() / PerfTraceList() / 整个 derive/render/finalizer 改为循环 N 个 bundle / cross-bundle dedup。
**预计**:~5-8 commits,~1500 LOC。

### 9.2 Frame Cap 自适应

**现状**:`collectArtifactFramesWithOrigin` 固定 cap 16。
**目标**:基于用户 Intent + artifact 大小动态调整。
**变更面**:cap 算法 + yaml 旋钮 + 用户意图 hook。
**预计**:~2-3 commits,~300 LOC。

### 9.3 跨语言 prompt

**现状**:skill prompt 仅英文。
**目标**:对应 -lang=zh 的中文 skill prompt(LLM 在 zh 模式下用 zh skill prompt 提升一致性)。
**变更面**:prompt template 系统;每个 prompt 双语版本;按语言切换。
**预计**:~3-5 commits,~600 LOC。

### 9.4 完全删除 OLD 命名

**现状**:`RenderDriftBoundedCurrent**RootCause**Summary` / `AnswerSummarySurface**DriftBoundedRootCause**` 等仍含 RootCause 词,即使行为已 generalize 给 IntentTrace。
**目标**:重命名为 intent-neutral(如 `RenderDriftBoundedCurrentSummary` / `AnswerSummarySurface**DriftBoundedDiagnostic**`)。
**变更面**:30+ callsite + tests。
**预计**:1 commit,~500 LOC 但纯 rename。

---

## 10. 附录:十一轮 Audit 总览

| Round | 焦点 | 主要修复 | 新增测试 |
|---|---|---|---|
| R1 | 接入 axis | ClaimOrigin/AuthorityCeiling enums + EvidenceItem.Origin/Authority/AuthorityReason | 6 |
| R2 | 渲染层覆盖盲区 | hedgeSummary + hedgeBoolean + 渲染层 hedge 注入完整性 | 8 |
| R3 | 三投影一致性 | Tier-1 floor 兼容 Authority + Highest/Lowest 跳过 Unknown + Recovered 无 log 时不 fake drift + 投影写回 | 7 |
| R4 | 身份完整性 | StableEvidenceID 含 Origin+Authority + 系统私有 caveat tag(zero-width)+ doc.Caveats[] strip | 12 |
| R5 | fallback 路径 + skill 准确性 | IsZero fallback 合成 caveat + Caveats dedup-all + skill prompt 简化"不写 markers" | 4 |
| R6 | 术语对齐 + 学习反馈环 | 术语统一"drift-bounded" + LLM-actionable repair hint + isPlumbingFailureViolation 过滤 plumbing 入 taxonomy | 5 |
| R7 | strip 精度 | upsertHedgePrefix 改为 system-match-only + knownSystemTerseReasons drift-prevention 测试 | 6 |
| Plan A P1-P3 | OLD/NEW 平行系统统一 | NEW 成为单一真理源,删除 350 LOC legacy detector | 14 |
| R8 | trace 漂移对称 | PerfBundle 进入 derive + symbol-axis fallback + Origin 标签传递 | 6 |
| R9 | 用户意图对齐 | rm.Intent gate(支持 IntentTrace)+ 无行号 frame 保留 + Residue 进 observation seed + 跨 artifact frame source | 4 |
| R10 | trigger threshold 一致性 | closureViolationCount 过滤 plumbing | 7 |
| R11 | render prose 跨场景中性 | drift summary 去 panic-specific tail + perf 专用 prose helper + 非 panic signal 语法适配 | 3 |

**累计**:~80 个新测试 + 大量 invariant 修复 + 系统从无 axis 到完全闭环。

---

## 11. 总结

这份方法论的本质是:**审计不是 checklist 走流程,是用户红线对照思维实践**。

第 1 节的五条第一性原理是审计的方向。第 2 节的五个优先级维度是审计的眼光。第 3 节的六种方法是审计的手脚。第 4 节的十二个具体维度是审计的体力。第 5 节的红线是不可逾越的边界。

**任何一次审计,如果你只能做一件事,做这件**:把当前的 PR / 本轮改动拉到所有 11 个对称维度上,问一遍"在这个维度的边界条件下,系统行为还正确吗?"如果你不能在不读代码的前提下回答,代码就还有断裂点。

最后,把这份文档当成活的项目资产。**每次审计后**,如果发现新的红线 / 新的方法 / 新的反模式,**回写到这份文档**。这样系统的"经验密度"会随时间增长,不会因为人员流动而蒸发。

> 文档作者:Claude (codrax 项目 2026-05-02 / 2026-05-03 十一轮审计后整理)
> 复审周期建议:每三轮重大变更后回顾一次
> 反馈渠道:本仓 issue 或直接 PR 此文档
