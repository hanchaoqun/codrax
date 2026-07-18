# 铸造侧审计:「链上」铸造点全谱 + 边分类学 + 命题1/2 对账

审计基准:用户三命题(1 包含自身;2 严格边+多级链;3 同线程分段凭证——本报告只对 1/2,分段问题仅在触及处标注)。
代码基线:main=8a6e327a9(读活树,只读)。核心文件:internal/tracequery/{query.go, rank_family_fold.go, rank_self_running_fold.go, rank_self_wall_clock_selfall.go, rank_chain_anchor_rspa.go, rank_semantic_edge_anchor_r3.go, types.go}。

## 一、OnChainBasis 闭集全谱(4 成员)

闭集定义:types.go:3201-3244 + query.go:17381-17387。`{"", self_deterministic_span, self_wall_clock_interval, host_wakeup_edge_pre_span}`。

### 1. `""`(链窗重叠基/wakeup-chain 本道,legacy 默认)

铸造点(ChainRelevance="on_chain" 且 basis 空):

| 铸造点 | file:line | 语义 | 铸造条件落在严格边上? |
|---|---|---|---|
| CausalImpact 排行席 | query.go:17025-17027 (rootCauseItemFromCausalImpact) | 链扩展节点的因果影响行 | **是(构造性)**:非目标节点只经 expandChain 递归进入,每一跳都要求 findWakeup 命中真实 sched_wakeup 行(query.go:22775-22793),无边不递归、无边不添 node |
| CausalAggregate 排行席 | query.go:17129-17131 | 同 pid×state 聚合席 | 同上(成员皆边入) |
| RootEvidence 目标自身行 | query.go:14498-14501 | 目标自身 runnable/io/d_state RootEvidence 排行席 | 无边——**自身恒链上**(R8 前身形),Causality="on_wakeup_chain" 系 legacy 词面(注:此处自身行仍佩 on_wakeup_chain 而非 self token,由 normalize/causality minter 不改写既有非空 Causality;词面上是残口,见命题1-c) |
| 语义 span 重叠臂(单span) | query.go:17482-17484 | span ∩ 同线程链节点/impact 窗 > 0(精确区间代数 semanticTraceSpanChainIntersection) | 间接是:链窗本身由边闭合([sleepStart, wakeupTs]);重叠计入值=精确交集,非包络 |
| 语义家族重叠臂 | rank_family_fold.go:771-774 | 家族成员∩链窗并集>0,参赛值=交集并集 | 同上 |
| 通用 enrich 同线程重叠臂 | query.go:18203-18207 (chainContextForCandidate) | 行区间 ∩ 同 pid 链节点窗 > 0 | 间接是(链窗边闭合),但**行区间对多数类型是包络**(HULL-CRED 只修了 D/IO;runnable 行有精确 inventory 臂 17884-17896) |
| **通用 enrich 无区间臂** | **query.go:18208-18211** | 行无 typed 区间(end<=start)且 pid 是链成员 → 无条件 on_chain,且 overlapMs=整个节点窗墙钟 | **否——裸线程身份继承 + 伪造重叠值**。RNB-1 B-4/HULL-CRED 只在 critical_blocking D/IO VIEW 面(20650-20735)和 RSPA 卫星臂修了;rank 泛道该臂仍在 |
| blocking_span Q4-A 对端臂 | query.go:17804-17809 | 已解析锁对(BlockingKind+BlockingPeer)行取 waiter 链上下文较优者 | 半是:凭证=typed waiter→holder 锁对 + 对端窗重叠(非唤醒边) |
| binder_wait 排行席 | 经 RootEvidence(query.go:13877-13886)+通用 enrich | P9 typed binder 事务对铸的等待行 | 记录本身有 typed 事务对;链道判定仍走重叠臂 |
| runnable 精确 inventory 臂 | query.go:17884-17896 | runnable 段清单逐段∩同 pid 节点窗并集 | 是(逐段∩边闭合窗,§29.104 家族的正面形) |

守门/降道(减法面,同属 "" 基):
- rootCauseItemCanBeDirectOnChain / rootCauseTypeCanBeDirectOnChain 闭表(query.go:18143-18177):不在表内的类型 on_chain 一律降 adjacent(page_cache_churn 等结构性排除;blocking_span 只认已解析形)。
- io_latency M-IO 完成闭合凭证(query.go:17977-17981 + resourceCompletionClosureProven rank_chain_anchor_rspa.go:1213-1226):无 per-IO 闭合凭证降 ◇。
- 宿主包含臂(query.go:18008-18015 + stampResourceClosureEvaluation):io_burst/block_io/file_io/workqueue/dma_fence 区间 ⊆ 锚窗并集才保道。
- RSPA 再锚定(reanchorOnChainStateSeats rank_chain_anchor_rspa.go:656-1052):runnable/D-IO 窗席按 census 锚定账二分,⛓ 只保锚定份,余席 ◇;卫星(scheduler_latency/low_frequency/affinity/inversion/fragmented)各有 typed 臂。
- HULL-CRED 四臂裁决(query.go:20845-20892):D/IO VIEW 行 hull∩锚窗>0 时逐段裁决,≥1 真段交才保⛓(佩逐段凭证);全不交降◇+披露;段清单缺席保守留道佩「包络级凭证」诚实词。
- normalizeRootCauseChainRelevance(query.go:16350-16369)+ chainRelevanceFromCausality(18296):Causality→relevance 回退映射,self token 也映 on_chain。

### 2. `self_deterministic_span`(SELF-SEM §29.61.1)

- 铸造点:query.go:17557-17559(E2 fall-through 自身臂)→ 17454-17460;家族 rank_family_fold.go:437-438 + 745-752。
- 谓词 selfDeterministicSemanticSpanLane(rank_family_fold.go:355-373):链宇宙存在 ∧ Target 已解析 ∧ sameThreadRef ∧ 确定性语义类 ∧ 窗内。
- 严格边:**无,设计如此**(命题1 的实现形;Causality=RootCauseCausalitySelfDeterministic,明示不铸唤醒边不宣称跨线程关系,types.go:3254-3259)。

### 3. `self_wall_clock_interval`(SELF-ALL §29.61.2)

- 铸造点(共享谓词 selfWallClockSeatLane,rank_self_wall_clock_selfall.go:145-159,一实现三消费):
  - rank enrich 翻道:query.go:17841-17845(◇→⛓,overlapMs 归零不伪造);
  - io_burst 面:query.go:19540-19544;
  - critical_blocking 面:query.go:21130-21134;
  - 自身 running 供给折算缺口席:rank_self_running_fold.go:125-127(mintSelfRunningSupplyFoldDeficitSeat;eff=缺口,0 缺口不铸席);
  - 症状行词面(无 basis,只 R8 通道):enforceSelfSymptomRowsChainChannelWireFace rank_self_running_fold.go:239-261。
- 准入:typed 自身身份 + 墙钟口径(registry Additivity==wall_clock_per_thread ∧ 非 aggregate_only ∧ V2-P0 caliber-side None)+ 窗内 typed 区间;SYM-2 等待症状族(wakeup_chain/lock_contention lane)与语义类(SELF-SEM 已有)排除。
- 严格边:**无,设计如此**(Causality=self token,零唤醒边宣称)。

### 4. `host_wakeup_edge_pre_span`(R3-IMPL §29.88.1)

- 铸造点:query.go:17560-17581 → 17461-17481(单 span);rank_family_fold.go:440-442 + 545-589 + 753-770(家族)。
- 凭证清单(rank_semantic_edge_anchor_r3.go:80-138,只用既有 typed 边库存,禁自造第二套边判定):
  - direct:WakeupEdgeCensus 原始 census 对 host→target(链扩展未消费的裸边也算);
  - chain_hop:host 在 chain.Edges 里自己的边(凭证沿链传递=多跳形)。
- 边前=有效/边后=解除,跨边按边界二分(◇ remainder clone semanticEdgeAnchorRemainderSeat)。窗退化 fail-closed(:99-108)。
- 严格边:**是**(host 自己的窗内 typed 唤醒边;别人的边不算——donghu 17267 decoy 负 pin)。

## 二、边分类学(哪些边种被认作入链资格)

### 被认的边种
1. **单跳唤醒边**:sched_wakeup 行匹配睡眠段终点(cache.findWakeup,边界容差有 caveat),铸 WakeupEdge typed 记录(query.go:22819-22841,含优先级 provenance)。这是 expandChain 唯一的递归边种。
2. **多跳唤醒链**:expandChain 逐跳递归(query.go:22693-22869),每跳独立要求自己的 sched_wakeup 命中;深度上限 q.MaxDepth,默认/上限=10(wakeupChainDefaultMaxDepth view_capacity.go:53,clamp+caveat query.go:1268-1271);环检测(visited);分支上限 MaxBranches + via-immunity(RN-14a);无衰减,每级节点以自己的窗账参赛。R3 的 chain_hop 凭证形把「host 自己的链边」延伸给 host 的语义 span(凭证沿链传递)。
3. **被依赖边**:
   - 锁 holder:typed 已解析 blocking_span 对(BlockingKind + BlockingPeer),入链形=直接链因资格(18149-18151)+ waiter 链上下文借道(Q4-A 17804-1809)。PeerChainStep 明示只延一跳不再扩(query.go:21101-21118)。
   - binder reply:P9 typed 事务对 → BinderWaits/RootEvidence(13871-13886),binder_wait 在直接链因闭表内;IPC 边入 res.IPCEdges 独立面,**不进 chain.Edges、不参与递归**。
   - blocked_reason iowait 供者:**不是边**——D 段终结时 blocked_reason 只改写 RootEvidence 的 Type/Summary(query.go:22852-22860),不向 caller/供者递归;IO 完成供者只以 M-IO 闭合凭证形出现(anchoredDIOWakeups + resourceCompletionClosureProven),是凭证校验不是扩展边。

### trace 中存在但不被认(潜在缺边种)
1. **D/IO 段的闭合唤醒**:expandChain 只对 StateSSleep 递归;D-state/io_wait 是终端 RootEvidence——完成 IO 的 waker(kworker/irq)持有真实 sched_wakeup 边却铸不了链节点席(仅在其对 target 有直接 census 边时可走 R3 语义 span 道)。
2. **锁链多级**:holder 的 holder 不递归(PeerChainStep one hop only)。
3. **binder 图多跳**:binder server 的下游依赖不经 binder 语义链化(server 恢复 client 的实际动作有 sched_wakeup 兜住,但 server 自己在等的东西不入链)。
4. **未选段的边**:mostInterestingInterval 每节点窗只扩一个睡眠段;MaxBranches/min_duration 掉队的目标段(有 caveat)与中间节点未选段的唤醒边不入链。
5. **裸 census 边的状态席**:host 对 target 有 direct census 边但非链节点时,只有其确定性语义 span 获 R3 席;其 runnable/D-IO 状态席无门可入(RSPA 只对有 depth>0 锚窗的 pid 运转)——SCAN-3 61839 形只收语义面。
6. **抢占位移**:同 CPU 位移(inversion overlap)被裁为 10a 纯时间重叠,RNB-1 R4 明确降◇——这是「有因果嫌疑但无 typed 边故不认」的裁定内形,与命题2 一致而非缺口。

## 三、命题1 对账(包含用户关注线程自身)

实现形(多臂 typed):
- a. RootEvidence 自身行 on_chain(query.go:14498-14501);
- b. SELF-SEM basis(确定性语义 span);
- c. SELF-ALL basis(墙钟席全族:blocked/IO facet/runnable/running);
- d. 自身 running 供给折算缺口席(rank_self_running_fold.go);
- e. 症状行 R8 通道词面(enforceSelfSymptomRowsChainChannelWireFace:自身等待症状行永不佩◇/▒);
- f. RSPA/再锚定全族自身豁免(rspaRowIsSelfExempt :342-350;chainAnchorWindowsByPID :106 排除 target;buildRSPAFamilyDecisions :297-300 自因豁免——自身恒全锚定);
- g. keep 臂:rootCauseChainRelevanceForItem 对 self basis 行 lane-decided-once,enrich 复跑不降道(query.go:17918-17922);
- h. causality minter 诚实词:self 行永不改写成 on_wakeup_chain(rootCauseCausalityForItem 18273-18293,ELIM-SELF-FIX 件1③)。

自身账的席位分工:
- 参赛链榜:running 缺口席(eff=可消除缺口)、runnable/D-IO 墙钟席、确定性语义席(union=已证可消除量)。
- 旁栏(不佩⛓但也不佩◇):计数当量族 self 行→⌗ self_caliber_side(query.go:18832-18836,R8 禁◇ ∧ §29.83 计数不进墙钟链道的交点);composite-score 自身行保 legacy lane(V2-P0 负 pin)。
- 症状面:depth-0 目标自身等待 impact 不进 rank 池(query.go:14395-14404,等待态是被解释的症状)但通道词面仍 on_chain——通道身份与参赛资格分离,符合 R8 的「恒链上」语义。

例外形排查(自身行不被认链上的形):
- 无链宇宙(chain nodes/edges/impacts 全空)或 Target 未解析时,self 谓词全 false——无链即无链榜,不构成违背;
- fail-closed 面(incarnation 冲突/时间线完整性失败,query.go:12737-12749)整链不建——同上;
- 残词面:RootEvidence 自身行(a 臂)Causality 仍是 "on_wakeup_chain" 而非 self token(14500),normalize 不改写已有非空 Causality 的 on_chain 行为 self 词需 basis,而该臂无 basis——ELIM-SELF-FIX 件1③ 只保护已铸 self token 不被覆写,未把 legacy self 行的 on_wakeup_chain 换成 self 词。词面级(不涉及席位/数值),候选卫生项。

**verdict 命题1:一致**(机制多臂、typed、恒链上;旁栏形皆有用户裁定:R8/§29.104.16 R10/V2-P0;残口=a 臂 legacy 词面 on_wakeup_chain 自称,微小、词面级)。

## 四、命题2 对账(严格的边 + 多级链)

多级链机制:
- 深度:每跳独立 sched_wakeup 凭证,depth+1 递归,MaxDepth 默认=上限=10,超限响亮 caveat;无逐级衰减(每级以自己窗账参赛,ChainDepth typed 携带到节点/impact/rank 行/家族);
- 分支:MaxBranches top-N + via-immunity 全量回滚制(RN-14a);
- 逐跳时间一致性:viaMonotonicHops F4/F5(非降 wakeup_ts + BFS 最短),跨分支拼接被禁;
- R3 多跳凭证=host 自己的链边(凭证沿链传递,60595 depth-2 判例)。
**verdict(多级):一致。**

严格边对账:
- 正面:wakeup-chain 本道逐跳强制真实 sched_wakeup;R3 基强制 host 自己的窗内 typed 边;SELF 基无边但系命题1 裁定形;RSPA/HULL-CRED/M-IO/宿主包含臂把「时间邻近/整席/包络」系统性从⛓剥离。
- 「无严格边也佩⛓」残余(HULL-CRED 修完 D/IO 后):
  1. **query.go:18208-18211 无区间同 pid 臂**:无 typed 区间的行(end<=start)只凭线程身份继承 on_chain,且 overlapMs 伪造为整节点窗墙钟。资源行有 17957 降道臂兜住、D/IO VIEW 行有 RNB-1 B-4/HULL-CRED 兜住、RSPA 卫星在 decisions 铸出时兜住;**但 decisions fail-open 时(无 census/无锚窗/时钟回退 offCPUProducerDisjoint=false,buildRSPAFamilyDecisions :288)非资源无区间行仍走此臂**。fail-open 属文档化保守边界(无凭证形禁猜→保道),但该臂连 overlap 值都是伪造的(非「保道」而是「造值」)。→ 不一致候选/依赖裁定。
  2. **包络重叠**(非 D/IO 族):18203 重叠臂对多数行读 StartTs..EndTs 包络;精确段清单臂只覆盖 runnable(17884)、D/IO VIEW(HULL-CRED)、RSPA 卫星(rspaRowIntervalAnchoredMs)。聚合/多段行的包络可在真段全不交时「重叠」链窗(XLANE-1 P1-1 教训的同构形,该教训只修了 rspaChainRunnableSeatWindowsByPID 供给侧)。残余暴露面主要是 binder_wait 与闭表内区间承载但无段清单的行。→ 缺口(小,多为自身/已有他门兜住)。
  3. **节点整席**(链窗内全状态入账,summarizeWakeupCausalImpact OnChain=true 无条件):边凭证在跳级,段级凭证是命题3 战场(XERR1/isIntermediateSleepImpact/RSPA 已部分处置)——本报告仅标注。
- 「有边但不认」(反向缺口):见 §二缺边种 1/2/3/4/5(D/IO 闭合唤醒者、锁二跳、binder 多跳、未选段、裸 census 边状态席)。其中 5 与 R3 裁定的适用面(仅语义 span)直接相关,扩展到状态席需新裁定。

**verdict 命题2:边即凭证在 wakeup-chain 本道与 R3/RSPA/HULL-CRED 覆盖面上=一致;多级链=一致;残余不一致点=18208 无区间伪造重叠臂(fail-open 交叉时)+ 非 D/IO 族包络重叠;反向缺口(有真依赖边不入链)5 种,均需裁定而非直改。**
