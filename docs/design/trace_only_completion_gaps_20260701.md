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
