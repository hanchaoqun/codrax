# Trace-only 完成/引用门 gap 记录(2026-07-01,客户 record_trace…ftrace 场景)

> 客户第二个反馈,与"大 trace"那份(`trace_large_trace_gaps_20260701.md`)不同——这次是**完成门/引用门**在纯 trace 分析("不分析代码")上的两个 gap,导致模型**反复重开调查、永不收敛**,并被**逼着走代码分析流程**。本文档先记录(附 REPL 证据 + 代码指针 + 修复方向),随后定位并修复。

## 场景

```
codrax --repo "/home/xingneng/codrax_test/test_trace" -r "分析东湖Trace: record_trace_...sys.ftrace
里面 这一帧 Choreographer#doFrame 1984465 (帧号1984465, UI线程31552, 渲染线程31788,
时间范围42196.87979s至42197.016526s) 因Sleep状态耗时长-被其他进程唤醒导致丢帧原因, 不分析代码"
[运行时附件] - trace record_trace_...sys.ftrace 0 B runtime trace; referenced in request
```

- 小仓 `test_trace`(46 文件,索引 2ms 缓存命中——无 OOM,但仍建了图)
- 用户明确 **"不分析代码"**,纯 trace 丢帧根因
- 模型正确把请求分类为 runtime artifact,并设 `external_observation_policy.current_source_mode=exclude`

---

## Gap A(严重)— 完成调查被引用门硬拦 → 调查反复重开 → livelock,永不出答案

**REPL 证据**:模型在探索阶段**至少 4-5 次**整轮重开("→ 正在收集证据"反复出现),每轮末尾调 `emit_investigation_complete`,**每次都被拒**:
- 反复调 `emit_evidence`(6-8 条 trace 行),每次都收到"**外部观测 N 条(未作为源码证据记录)…系统未将其当作当前源码引用,不要重试 emit_evidence**"。
- 模型自述:"The system requires **at least 2 citation(s) from source files**… this is purely a runtime trace analysis task - there's no source code to analyze."
- 模型试图用 `evidence_floor_waiver` 逃生,但:①它自我怀疑"trace 文件 IS in the repo…so I should NOT use evidence_floor_waiver"(被"文件物理上在 --repo 目录里"误导);②它实际填的 waiver reason 是 **`runtime_artifact_only`——这不是合法枚举值**(合法值见 `internal/types/evidence_floor_waiver.go`:`external_only_log`/`external_only_trace`/`no_repo_intersection`/`informational_runtime_only`),必然被拒;③即便偶尔填对,完成门仍反复重开。
- 结果:调查**反复重开** 4-5 轮(探查→校验→归纳→再收集证据→再探查…),消耗巨量 token,**始终没有产出最终答案**,直到用户放弃。

**根因**(已定位):
- 完成/答案契约有**引用底线** `CitationReq.MinCitations`(通常 ≥2 条**当前源码**引用,`internal/types/analysis_ir.go:1532`)。纯 trace 调查唯一的证据是 **runtime_artifact** trace 行,不被计入 cite-eligible 当前源码引用(`emit_evidence` 明确把它们归为"external observation")。
- **关键盲点**:analyzer 已经把请求判为 `external_observation_policy.current_source_mode=exclude`(typed 字段,`analysis_ir.go:212`),但**完成门/引用门代码完全没有消费这个 typed 信号来放宽引用底线**——`internal/orchestrator/{contract_check_block.go,completion_obligation_lane.go,cgec_enforcers.go}` 里 grep 不到 `current_source_mode`/`ExternalObservationAllowsCurrentSource`/`RuntimeArtifactObservationOnly`。于是对一个显式"不看代码"的请求,门仍然强要当前源码引用。
- `evidence_floor_waiver` 机制**存在且有正确的枚举**(`external_only_trace`/`informational_runtime_only`),但它是**让模型手动声明**的逃生阀,而不是系统在"已知 current_source_mode=exclude + 附了 trace"时**自动放宽**——把一个纯确定性可判定的豁免(精确 typed 信号)错误地压给模型去猜 reason,模型猜错(用了非法 `runtime_artifact_only`)+ 被"trace 在 repo 目录里"误导 → 死循环。
- **次生安全问题**:完成门在被拒后**无上限地重开调查**,没有"N 次拒绝后硬失败/降级接受"的收敛边界 → livelock。

**修复方向**(本轮修):
1. **系统侧自动放宽**:当 `external_observation_policy.current_source_mode=exclude`(或等价的 runtime-artifact-only typed 信号,和 Gap 1 同一信号)且附了 runtime artifact 时,完成/引用门应**自动**把当前源码引用底线降为 0 / 自动视为已豁免,接受 runtime_artifact 观测(trace 行)作为答案证据基底——不需要模型手动填 `evidence_floor_waiver` reason。这是精确 typed 信号驱动的确定性放宽,符合"精确信号做硬门"红线。
2. **重开收敛边界**:完成门重开调查必须有硬上限(N 次后硬失败或降级接受当前证据),杜绝 livelock。
3.(可选)`evidence_floor_waiver` 非法 reason 应回显合法枚举列表(可能已有,需核实),并在 skill 里澄清"trace 文件物理位置在 --repo 目录内 ≠ 它是当前源码;runtime artifact 的豁免看的是 origin 不是路径"。

---

## Gap B — 用户明说"不分析代码"却被逼走代码分析流程

**REPL 证据**:
- `1/4 正在统计仓库索引 test_trace 文件…46 个文件`——仍建了 repomap 图(同上一场景 Gap 1,只是小仓无 OOM)。
- 更严重的下游后果:引用门(Gap A)**逼着模型去找源码引用 / 把 trace 当源码读**——"I should read the specific lines from the ftrace file that document the frame drop… then reference those trace entries directly"、"if there are any actual source code files in this repo that might be related"。用户明确"不分析代码",系统却把它推回代码分析姿态。

**根因**:与 `trace_large_trace_gaps_20260701.md` 的 Gap 1 同根(analyzer 对 trace-only 请求仍建 repomap 图),但本例的严重面在**完成门**:即使跳过建图,只要引用底线还要当前源码引用,模型就会被迫回到"读代码/找源码引用"。所以 **Gap B 的彻底修复 = Gap 1(跳过建图)+ Gap A(引用门对 runtime-artifact-only 自动放宽)** 一起,让"不分析代码"的 trace 请求端到端都不碰当前源码机制。

**修复方向**:并入 Gap A 的修复(引用门放宽)+ 已排队的 Gap 1(跳过建图)。二者合起来即"runtime-artifact-only 请求端到端绕过当前源码机制"。

---

## 与既有排队项的关系

- Gap A / Gap B 与 `trace_large_trace_gaps_20260701.md` 的 **Gap 1**(analyzer 建图)共享同一个 typed 信号(`current_source_mode=exclude` / runtime-artifact-only)。三者应作为一个连贯改动:**该 typed 信号必须同时驱动 (i) 跳过 repomap 建图、(ii) 完成/引用门自动放宽当前源码底线**。
- 本轮先修 Gap A(livelock,最severe)+ Gap B 的引用门部分;建图跳过(Gap 1)按原排期。

---

## 修复结果(2026-07-01)

### Gap A 引用门部分 + Gap B 引用门部分 —— 已修复(root-cause,精确 typed 信号驱动)

**根因确认**:`internal/tool/emit_investigation_complete.go` 的完成前引用底线预检 `preCompleteContractCheckWithPreflight` 通过 `completionGroundingBypassLabel` 决定是否放宽 `CitationReq.MinCitations` 当前源码底线。原来的三个放宽分支全部**依赖运行时证据的具体形态**:
- `traceQueryRuntimeObservationCompletionBypassLabel` —— 只在模型用过 `trace_query`(`TraceQueryRuntimeObservationCount()>0` 或结果里有硬 runtime observation)时触发;
- `originSpecificCompletionBypassLabel` —— 只在有带 typed origin 的 aggregate_facts 时触发;
- `repoGroundingBypassLabel` —— 仓库侧。

客户的模型用 **裸 `read_file`/`grep`** 读 ftrace(没走 `trace_query`、也没产出 typed aggregate_facts),于是三个分支全不触发,底线仍强要 ≥N 条当前源码引用——而这是一个 `current_source_mode=exclude`(用户明说"不分析代码")的请求,永远拿不到当前源码引用,`emit_investigation_complete` 每轮被拒→调查反复重开→livelock。**关键盲点:完成门此前完全没有把 `ExternalObservationPolicy.ExcludesCurrentSource()` 这个精确 typed 信号当作放宽依据**,尽管读侧 tier-1 底线 `readLocalizerTier1CurrentSourceRequired`(`internal/orchestrator/tier1_floor.go:170`)早就把它当作"当前源码非必需"。

**修法(已落地)**:新增 `explicitCurrentSourceExclusionCompletionBypassLabel(ctx)`,当 `rm.ExternalObservationPolicy.ExcludesCurrentSource()` 为真(=`current_source_mode=exclude` **且** `exclusion_kind=explicit_user_exclusion` **且** 有 verbatim `source_quotes`,三段式)时,直接放宽完成引用底线,返回标签 `explicit_current_source_exclusion`。它:
- 被 `completionGroundingBypassLabel` **最先**咨询(优先于三个形态依赖分支);
- 在预检里也**独立于 `ctx.Mutable`** 提前咨询一次(exclude 是纯 typed 请求信号,不依赖任何可变观测状态,即便 Mutable 缺失也必须放宽);
- 是**精确 typed 信号做硬门**,符合"精确信号才能用作硬约束"红线——任何缺锚点/无 runtime artifact 载体的 exclude 都已在上游 `promoteInvalidExternalObservationExcludeToAllow` / `normalizeExternalObservationPolicyForCurrentSourceExplanation` 被降级成 `allow`,能走到完成门为真的 exclude 必然是"用户显式不看代码"的锚定请求。

这样纯 trace 分析(不管 trace 是 `trace_query` 读的还是 `read_file`/`grep` 读的)在 exclude 请求下**首轮 `emit_investigation_complete` 即成功**,不再需要模型手填 `evidence_floor_waiver`,也不再把模型逼回"读代码/找源码引用"(Gap B 引用门部分随之解决)。

**测试**:
- `TestEmitInvestigationComplete_ExplicitCurrentSourceExclusionBypassesCitationFloor` —— exclude 请求 + 空证据 + MinCitations=2 + 无 waiver → 完成成功、reason 落库。
- `TestEmitInvestigationComplete_CitationFloorHoldsWithoutExplicitExclusion` —— 窄控制:同样空证据但 default 模式 → 底线仍生效、不落库(证明放宽严格 key 在 exclude 精确信号上,不松动普通当前源码问题)。
- `TestExplicitCurrentSourceExclusionCompletionBypassLabel` —— 三段式 `ExcludesCurrentSource()` 谓词单测(exclude 缺 quote / default / nil 均不触发)。
- 修正既有 `TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorBlocks`:它此前用一个**合法 exclude**("只分析日志")做 setup 却断言底线仍 block——恰好把 Gap A 的 buggy 行为钉成了回归测试。改为 default 模式(其"证据空则 block"的真实意图保留,由 default 模式承载),exclude 放宽由新测试覆盖。

### Gap A "重开硬上限"部分 —— 已存在,无需新增(避免噪音信号硬门)

完成门的下调**已经**经 `preCompleteDowngradeConverges(ctx, DowngradeLaneContractChain)` 走低-delta 收敛硬上限(`downgradeConvergenceHardThreshold` 默认 3):同一 lane 连续 N 次无进展下调后,**force-complete 并附 caveat**,而非无限重开。客户 livelock 之所以绕过它,是因为模型每轮都在**churn**(读不同文件、试不同 waiver reason `runtime_artifact_only`),使"无进展指纹"不断变化、把收敛计数重置。

**不新增"总重开次数"硬上限**:那会是"嘈声信号(重开/churn 计数)做硬门",可能对**真正需要多读几轮**的当前源码调查提前 force-complete 出欠接地答案,违反红线。root-cause 修法(exclude 首轮即成功)已从源头消除本 case 的 livelock;既有同-blocker 收敛上限继续兜底真正的原地打转。

### Gap A fix direction #3(可选的 waiver reason 回显)—— 对 exclude 请求已 moot

exclude 请求现在完成门自动放宽,模型**根本不需要**手填 `evidence_floor_waiver`,所以"非法 reason `runtime_artifact_only` 被拒 + '文件在 repo 目录里'误导"这条支线对 exclude 请求不再触发。非 exclude 的 runtime-artifact-only 请求仍走既有 waiver 枚举通道(不在本轮改动面)。

### 仍排队(HEAD 复核后收窄为 pre-emit carrier gap)

- **Gap 1 / Gap B 建图部分**:引用门和 post-emit `analyzerGraphForNormalize` 已消费 `current_source_mode=exclude` 并绕过当前源码底线/repomap eager load;剩余风险只在 analyzer 首轮 `buildAnalyzerRepoOverview` 的 **PRE-emit** 阶段。该阶段发生在 `emit_analysis` 之前,不能读取 `RequestModel.ExternalObservationPolicy`,因此需要 `trace_large_trace_gaps_20260701.md` 记录的 typed pre-analyzer runtime-artifact carrier。修完后"不分析代码"的 trace 请求才真正端到端不碰当前源码机制。
