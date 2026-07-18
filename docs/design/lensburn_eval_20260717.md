# LENSBURN 评估(§29.122 立案两件 P2;排查+立案素材,不实施)

评估基线:main=4b90fd27f(活树只读)。立案原文:账本 §29.122 立案段(real_trace_campaign_20260705.md:3075)。
TOOLWIN-FIX 修形上下文:§29.122 病灶链八环(环0 合成 profile → 环5 完成门 4 轮断路器),修形A 已治「窗面工具缺席」,残余两件 = 本评估对象。

---

## 病A:合成守卫大 trace 失明

### 一句诊断

`HasObservationOnlyRuntimeArtifact` 依赖 perf_triage 产出的 `rm.PerfTrace` bundle 判定「观察-only 运行」,而大 trace(>512KiB)在 perf_triager 被尺寸门整段跳过、bundle 恒 nil,守卫恒 false——trace-only run 照常合成 source_inventory_profile,lens 窗成形、义务铸入、进入病B烧轮。

### 机制链(file:line)

1. **尺寸跳过**:`internal/agent/perf_triager.go:218-221` — `len(ctx.AttachedHitrace) > LLMMaxBytes(512KiB 默认, :81)` → `a.skipped(...)`,**任何 bundle 都不写**(两步路径 :224 在尺寸门之后,>512KiB 永不可达)。设计意图正确:「detailed trace analysis is delegated to trace_query」——但下游守卫没有跟上这个委托。
2. **bundle 缺席传导**:`internal/tool/emit_analysis.go:1608` → `attachRuntimeArtifactsToRequestModel` (:4019-4029) 只从 `ctx.Mutable.PerfTrace()` 取,skip 后取到 nil → `rm.PerfTrace == nil`。
3. **守卫恒 false**:`internal/types/request_traits.go:1154-1162` `HasExternalOnlyRuntimeArtifact` 只认 `rm.LogTriage.IsExternalSource() || rm.PerfTrace.IsExternalSource()`;:1324-1329 `HasObservationOnlyRuntimeArtifact` = 前者 ∧ policy.ExcludesCurrentSource() → **即使 analyzer 正确发射了 excludes-current-source policy,大 trace 下守卫仍恒 false**。
4. **失明后果面(两个盲守卫)**:
   - 合成守卫 `internal/tool/emit_analysis.go:1893`(`synthesizeSourceInventoryProfileForTypedEnumeration` 内)不触发 → 枚举形请求白铸 profile(:1896-1907,Confidence 0.45)。
   - 撤销守卫 `emit_analysis.go:2939-2948` `dropSourceInventoryProfileForObservationOnlyRuntime` 同样失明 → **模型自发 profile(§29.122 h6 双来源形)也拦不住**。
5. **下游成窗**:`orchestrator.go:4921` `env.SourceInventoryProfileActive` → `orchestrator.go:5109` + `source_inventory_lens_first.go:9-28` lens 窗优先调度 → lens 义务铸入完成门 → 病B。

### 失明条件(精确)

attached trace > `perf_triage_llm_max_bytes`(默认 512KiB)∧ 无 --log 附件(或 log 同样不成 bundle)∧ 请求呈 typed 枚举形(`IsTypedSourceEnumerationShape`)。真实客户 trace(berlin 1104MiB、donghu、runnable 系)**全部**超 512KiB——即失明是真实 trace 的常态而非边角。

### 关键既有资产(修向素材)

- `emit_analysis.go:3955-3975` `emitAnalysisObservationOnlyRuntimeArtifact(ctx, policy)` 已示范 ctx-aware 合并 Mutable bundle 的形——但同样只认 bundle,不认 preflight。
- **Run-entry typed preflight 同载体**(账本修向指名):`BusContext.RuntimeArtifactPreflight`,`orchestrator.go:1726` 建于 Run 入口,`runtime_artifact_preflight.go:16` 确定性(附件在场性+字节数,零 LLM);`types/runtime_artifact_preflight.go:110` `HasTraceArtifact()`。TOOLWIN 修形已把它立为咽喉级 typed 门(`read_loop_next_action.go:222` admit 臂、`agent.go:6583-6585` `traceQueryToolVisibleFromRuntimePreflight`)——病A 修复消费**同一载体**即单值源纪律(不铸第二载体)。
- 附带精确信号:`RuntimeArtifactPreflightProfile.ZeroCurrentSourceRepo()` (:51-53,census Completed 门保精确、部分步行惰性)。

### 修向候选(红线分层)

- **方向A1(推荐,根修)**:两个盲守卫(:1893 合成 + :2939 撤销)改走 ctx-aware 谓词,在既有 bundle 判据之外**增读** `ctx.RuntimeArtifactPreflight.HasTraceArtifact()/HasLogArtifact()` 作为 external-runtime-artifact 载体等价物;`policy.ExcludesCurrentSource()` 合取**保留不放松**(typed enum,精确)。红线:硬门(抑制合成)建立在两个精确信号上(preflight=确定性附件在场布尔;policy=schema 校验 typed enum)✓。需把 ctx 穿入 `synthesizeSourceInventoryProfileForTypedEnumeration`(call site :1623 所在 Execute 已持 ctx)。
- **方向A2(补充,可同批)**:`ZeroCurrentSourceRepo()` census 为真时独立抑制合成——trace-only 仓的 source-inventory 义务结构性不可满足(与 §29.104.12.1 拒冕一般化同族论证)。census Completed 门保精确;census 未完成时惰性回退 A1。
- **方向A3(不推荐做主修)**:perf_triage 尺寸跳过路径改写一个最小 typed「external trace 在场」bundle 存根。修的是同一盲类的全体(HasObservationOnlyRuntimeArtifact 有 20+ 消费点),但 PerfBundle 语义被借作在场标记 = 载体语义污染(bundle 消费者期待 janks/stalls 事实),且 preflight 已经是那个「附件清单」——再铸一份违反单值源。**否**。
- 软引导侧:无需——本病灶纯属精确信号缺席,不是嘈声判断题。

### 后果面/严重度

- 修前(TOOLWIN 前):答案毁灭级(§29.122 环7 六锚全缺)。**修后(现基线):烧预算不毁答案**——白铸 profile → lens 窗成形 → lens probe 轮次 + 病B 4 轮 emit 烧耗;复放直接归因 wall 216s。
- 客户可见面:大 trace 会话延迟显著拉长(分钟级),真实 trace 全命中。P2 恰当;属「调查结束/成文阶段非致命不硬拦」(§29.104.13)精神的上游供给侧修复。

---

## 病B:lens 义务空仓烧轮

### 一句诊断

lens 义务的全部四条「lens 已执行」承认臂都以 `observation.IsActive()`(= 有 Sets 或 SourceClasses)为前置——空仓 lens(执行成功但零行,`PublishSourceInventoryObservationFromLens` 返回零值 struct)结构性不被承认为「已执行」,完成门每轮重发同一 downgrade,恒烧 3 轮拒绝 + 第 4 轮断路器 force-complete。

### 机制链(file:line)

1. **空仓返回零值**:`internal/tool/source_inventory_reconcile.go:373-398` — advisory 不成活 ∧ class-universe 不成活(trace-only 仓零 source class)→ `return types.SourceInventoryObservation{}`(:398,字面「no observation」)。repo_map 仍把它挂上 ToolResult(`repomap/tool.go:425-432` `SourceInventory: &observationCarrier`),但是惰性壳。
2. **IsActive 门**:`types/source_inventory_observation.go:98-100` — `Active && (len(Sets)>0 || len(SourceClasses)>0)`。零值 → false。
3. **四条承认臂全灭**(`internal/tool/source_inventory_universe_coverage.go:142-146`,`SourceInventoryLensExecutionGapForContext`):
   - advisory 佩 `repo_lens:tool_query`(:217-227)— advisory 不成活;
   - observation 佩同 provenance(:229-239)— `IsActive()` 前置;
   - dispatch/ctx ToolResults 有 lens(:262-272)— 要求 `SourceInventoryLensExecuted(*result.SourceInventory)`,而 `types/source_inventory_lens_execution.go:4-19` 第一行就 `if !o.IsActive() return false`。
   → `gap.Blocking = true` 恒真(:148),无论模型执行多少次 lens。
4. **调度侧同盲**:`orchestrator.go:4924` `env.SourceInventoryLensExecuted` 用同一谓词 → `source_inventory_lens_first.go:14` lens 窗每轮重新优先 → 模型每轮被推回去重跑注定空仓的 lens(烧的不只是 emit 轮,还有窗轮)。
5. **烧轮记账**:`emit_investigation_complete.go:2281-2293` lens 臂每次 attempt 命中 `sourceInventoryLensExecutionDowngrade` (:5768-5813,重发同一「lens has not run」文案+AddRepair);blocker key 稳定 → `preCompleteDowngradeConverges` (:2553-2588) 按 `downgradeConvergenceHardThreshold = 3` (:2533) 收敛:**第 1-3 次 attempt 各吐一次 downgrade(Success=true 软信号),第 4 次 force-complete + CompletionCaveat(lane=source_inventory_lens)**。同 else-if 链的 `sourceInventoryResolvedCompletionDowngrade`(:2268,lane=source_inventory_completion)按 resultKind 可为同族第二烧点。
6. **退出条件**:仅断路器(4 轮)或模型碰巧凑出 `SourceInventoryAcceptedClosureCoversExactUniverse` / path-handoff(:5769-5771)。空仓下前者不可达。

### 烧耗量化

每轮 = ≥1 次 explore LLM turn + repo_map lens 重跑 + emit_investigation_complete attempt + 门链模拟;共 4 轮 emit + 若干窗轮。§29.122 复放:**wall 216s 直接来源**;约 4-8 次 LLM 调用当量。答案不毁(断路器带 caveat 诚实落地)——纯预算/延迟烧耗。

### 修向候选(红线分层)

- **方向B1(推荐,根修)**:「lens 已执行」立为独立于结果非空的一等 typed 事实。两点:
  ① `source_inventory_reconcile.go:398` 空仓路径不再返回零值 struct,改返回/持久化一个 typed「executed-empty」observation(Active=true + lens-execution state + `repo_lens:tool_query` provenance + 零 Sets;既有 `sourceInventoryObservationWithLensExecutionState` (:412) 即挂载点);
   ② 承认臂放行:`SourceInventoryLensExecuted` 或至少 `sourceInventoryToolResultsHaveSourceInventoryLens` (:262) 接受「成功执行的 source_inventory lens、结果为空」为执行凭证。
   红线论证:执行凭证 = 确定性工具结果(工具名+view+Success),**精确信号**,非模型散文;拿掉的是一个把「空结果」误读为「未执行」的硬拦——恰合 §29.104.13「非致命不硬拦」与完成门权属模型(feedback_completion_gate_respect_model:致命 = 零见证/工件缺失/schema;诚实空仓 + absence 收束非致命)。gate 语义不降级:文案本来就允许 `result_kind="absence"` 收束(:5811),只是承认臂到不了那里。
   连带:`env.SourceInventoryLensExecuted` (orchestrator.go:4924) 自动痊愈 → lens 窗不再每轮重成形(调度侧烧轮同根同治)。
- **方向B2(次选,收缩不除根)**:lens-execution lane 仿 `DowngradeLaneCompletionForm` 降 exactThreshold 至 1-2(:2563-2569 已有 lane 差异化先例),条件 = ToolResults 里存在成功且结果为空的 source_inventory lens。仍烧 1-2 轮,阈值是半嘈声旋钮——只配作 B1 的保底,不配作主修。
- **方向B3(补充)**:`ZeroCurrentSourceRepo()` 为真时 lens 义务整体不铸(与 A2 同一 census 信号)。只覆盖 trace-only 仓形,不覆盖「正常仓、真空仓库存」的一般空仓形——作 A2 的伴生小臂即可。
- 软引导侧:lens downgrade 文案 (:5797-5811) 可顺带增补「若 lens 已返回空,直接以 result_kind=absence 收束」的显式指路(现文案末句已有但埋深)——纯 prompt 面,随批便宜修。

### 与病A 的耦合

病A 修复消灭 trace-only 大 trace 场景的**主要 profile 供给源**,但病B 独立可达:h6 模型自发 profile 形、以及任何真空仓库存的正常仓。两病同批(账本已并 LENSBURN)、A 上游 B 下游,B1 是独立正确性修复而非 A 的兜底。

---

## 严重度与批规模建议

| | 客户可见面 | 烧耗 | 建议 |
|---|---|---|---|
| 病A | 大 trace(>512KiB=真实 trace 常态)+枚举形请求:会话延迟分钟级 | 白铸 profile→lens 窗+病B全额烧耗的供给源 | P2,修向A1(+A2 便宜臂) |
| 病B | 空仓 lens 场景恒烧 | 4 轮 emit+窗轮,复放 wall 216s,≈4-8 LLM 调用 | P2,修向B1(+文案便宜修) |

**批规模:中批(合并 LENSBURN 单批)。** 病A=小件(两守卫穿 ctx+新谓词+单测+L1 双 pin 复核);病B=小中批(observation 语义面有 3+ 消费点:completion authority/authority snapshot/coverage/env,需突变电池:承认臂放行正臂绿+空仓假执行负臂红+4 轮烧耗回归复放)。两件共享 preflight 载体与 census 信号,同批摊薄复核成本;终验建议带修二进制大 trace 复放对照 wall(216s 应塌缩)。

## 与既有基建衔接

- **单值源**:病A 消费 TOOLWIN 已立咽喉的 `BusContext.RuntimeArtifactPreflight`(`read_loop_next_action.go:222` admit 臂、`agent.go:6579-6585` `traceQueryToolVisible` 单源谓词同载体)——不铸第二载体,不动 traceQueryToolVisible 本体(其 4 消费点 census 已 pin,§29.122)。
- **TOOLWIN iter=0 语义保持**:h3 复放的「lens 窗成形→iter=0 trace_query 成功」通路不受影响;病A 只消灭**假**成窗,病B 让**真**成窗在空仓时 1 轮诚实退出而非 4 轮。
- **L1**:emit_analysis 守卫与完成门改动均在 read 主路径,须过 `TestRunMode_ReadByteIdentical` + read e2e;`DeniedTools` 刻意退出窗(landing-repair emit-only)pin 不动。
- **禁区注意**:病B 修复不得触碰「结论一致性指控臂」禁区,不新增任何以模型散文为判据的门;B2 若采用,阈值差异化须走既有 lane-aware 先例形而非全局旋钮。
