# ONCHAIN-FIX-3 3b-e 四件边种扩展评估(只读排查,2026-07-19)

评估官产出。仓库=main 67e5e1a45(活树只读,零改动;探针全部在 scratchpad/edge3be_eval/ git-archive 副本内)。
探针:`edge3be_eval/internal/tracequery/zz_edge3be_probe_test.go`(四件旗舰窗量测)、`zz_edge3de_fulltrace_probe_test.go`(3d/3e 全 trace 存在率)、`zz_edge3d_uncapped_probe_test.go`(3d 未截断 span 库存 + 大 trace 分窗扫)。
量测窗:tieba59566 flag(34579.472865..587805)/tieba trace(34579.450627..595184)/donghu2955、donghu17267(13762.791708..13763.024898);另 big_trace.ftrace(67M)分窗扫。
spec 基准:scratchpad/onchain_fix_spec.md ONCHAIN-FIX-3;审计底稿 docs/design/onchain_mint_audit_20260718.md §二(反向缺口五种)+ onchain_segment_audit_20260718.md。
CHAIN-BUDGET(§29.136)已落地确认:`registerExtraCandidates`(query.go:23177)显式注释「D state is the FIX-3b closed-credential scope, not this lane」——3b 的接口位已预留。

---

## 3b D/IO 闭合唤醒者铸链节点

**现状机械**(核实):`expandChain` 只对 `StateSSleep` 递归(query.go:23482-23495);D/IO top-1 段走 `case StateDSleep, StateIOWait` → 终端 RootEvidence(blocked_reason 改写 Type/Summary,query.go:23498-23515),不向 waker 递归;extras 车道 `registerExtraCandidates` 显式滤掉非 S-sleep(23185)。M-IO 闭合凭证今天只做**校验**不做扩展:`resourceCompletionClosureProven`(rank_chain_anchor_rspa.go:1233)方向是「给定 IO 记录,查其完成线程是否唤醒过链线程的锚定 D/IO 段」,消费点唯一=io_latency rank 行 enrich(query.go:14652)。

**闭合凭证覆盖率实测**(链节点窗内 ≥1ms D/IO 段;判据=findWakeup 闭合 ∧ ∃io∈stats.IOLatencies: CompleteThread==waker ∧ wakeTs∈[IssueTs, CompleteTs+0.5ms]):

| case | D/IO 段 n / ms | 唤醒闭合 | M-IO typed 闭合 | 闭合 waker |
|---|---|---|---|---|
| tieba flag+trace | 5 / 17.078 | 5 / 17.078 | **2 / 6.645(38.9% ms)** | udk-irq-2-64, udk-irq-3-65 |
| donghu2955 | 1 / 3.853 | 1 / 3.853 | **0 / 0** | (binder reply 闭合——CompThread_0 的 3.853ms D 段即 binder_wait txn12145937,waker 是 binder server 非 IO 完成者,M-IO 凭证不覆盖,按 spec 正确拒) |
| donghu17267 | 1 / 3.598 | 1 / 3.598 | **0 / 0** | 同上形 |
| 合计 | 7 / 24.529 | 7 / 24.529 | 2 / 6.645(27.1% ms) | |

**would-be 子节点实测**:tieba 2 个新节点(udk-irq),子窗 [段起, wakeTs] 内 waker 状态账合计 6.645ms,其中 running 仅 **0.118ms**——供者身份可见性收益为主,数值席收益极小(供者在子窗内绝大部分时间自己在睡)。donghu 两案 0 新节点。

**接入 CHAIN-BUDGET 的改法**:接口已备——(a)extras 车道:23185 的状态滤放行 D/IO,候选打 typed lane 记号,drain 时凭证臂分叉(S-sleep=裸 findWakeup;D/IO=findWakeup ∧ M-IO 闭合证明);(b)top-1 车道:D/IO case 在铸终端 RootEvidence 后再试闭合扩展;预算/地板/披露 caveat 全沿用。**但有一个实质架构成本**:M-IO 证明需要 IO issue/complete 配对库存,今天配对在 `ComputeWindowStats`(blockPairing)内,而链构建在 stats **之前**(anchors 喂 stats)——存在环依赖。根修需把 block IO 配对抽成链构建可用的共享 pre-pass(或 chainQueryCache 内联最小配对),这是 3b 的主要实施成本,估 400-700 LOC + 配对抽取重构。

**值/榜涟漪半径(大)**:新节点→chain.Nodes/Edges/CausalImpacts(+2 席,值≈0.1ms)→`chainAnchorWindowsByPID` 新增 udk-irq pid 锚窗→RSPA decisions 对新 pid 运转(其 census 席二分随动)→R3 chain_hop 凭证库存扩(udk-irq 无语义 span,实测零增席)→树显示新节点+边;板身份指纹:D-lane 准入是否入 XLANE-3 参数闭集需裁(展开行为变=异板先例)。pin 波及:链形 tests、QUAD、chain_budget_test 帽词面、树 golden。

**顺路收益(§29.136 备案观察项)**:「无边 ≥地板候选零披露」实测**四案全零**(所有 ≥1ms sleep extras 均有真边)——该观察项在库无真 witness,属合成可构造形;3b 实施时在同一 consequence-ownership 代码位顺手加 typed 披露即可(成本≈0),不值得单独开批。

**建议**:**缓办候选**。真收益=tieba 2 供者节点(6.645ms D 账获得供者归因,数值席 0.118ms),donghu 0;凭证覆盖率 27% ms(binder 闭合 D 段按 spec 正确不入);而成本含 IO 配对抽取重构+大涟漪半径+旗舰双复核。收益/成本比在四件中排第二但绝对值小;若实施,先做配对共享 pre-pass,并与「无边候选披露」同批。

---

## 3c 裸 census 边状态席(R3 射程扩展)

**现状机械**(核实):R3 凭证臂 `hostSemanticSpanEdgeAnchor`(rank_semantic_edge_anchor_r3.go:80)只在**语义 span** mint 位消费(query.go:17560-17581 单 span + rank_family_fold.go 家族);凭证=host 自己窗内 direct census 边(host→target)或 chain_hop 自有链边;边界二分 `semanticEdgeAnchorSplit` + ◇ 余席 clone `semanticEdgeAnchorRemainderSeat`(RSPA typed 三元组复用)。非链 host 的 runnable/D-IO 状态席今天无门可入 ⛓(RSPA 只对有 depth>0 锚窗的 pid 运转,rspa.go:103;enrich 泛道对非链 pid 无节点窗重叠=◇)。

**witness 在库:是,且量可观**:

| case | 非链带边 host 数 | runnable pre-edge ms | D/IO pre-edge ms | 代表 |
|---|---|---|---|---|
| tieba flag | 2 | 14.329 | 3.550 | Binder:43397_19-23088 runnable 13.959;**T7@ZeusThreadPo-61839(SCAN-3 判例)dio 3.550 + runnable 0.370,sem_spans=1(语义面已 R3 入席,状态面无门)** |
| tieba trace | 6 | 36.602 | 3.750 | +Chrome_IOThread-60560 runnable 19.358 |
| donghu2955 | 3 | 10.678 | 0 | OS_FFRT_2_0-2614 runnable 9.618(边前份/全额 19.563 二分显著) |
| donghu17267 | 6 | 0.314 | 0 | udk-irq 族全是 µs 级(天然负臂样本) |

(口径:host=census pair host→target,Count>0 ∧ LastTs 窗内 ∧ host∉链节点;pre-edge=段∩[窗起, 最晚边];census 段清单=runnableIntervals/dioIntervals。)

**语义正当性**:边前/边后二分在状态席上的读法与 R4 成文公理同构——host 边前的 runnable=「waker 自己被 CPU 饥饿,推迟了对目标的唤醒」(与链上 scheduler_latency 卫星同一供给车道语义);边前 D/IO=「waker 在等 IO 才迟唤醒」。61839 判例正是 SCAN-3 立的形(裸边唤醒已 runnable 的目标,链窗交面盲)。二分纪律、凭证库存(禁自造第二套边判定)、fail-closed 窗退化全可照搬 R3;且这是**既有 ◇ 行的换道+二分**,零新 ms 铸造(守恒天然)。

**牵动面(中)**:enrich 新臂(非链 pid runnable/D-IO census 席,凭证=hostSemanticSpanEdgeAnchor 原函数零改)+ 段级二分(段清单载体已在)+ ◇ 余席 clone(semanticEdgeAnchorRemainderSeat 模板);basis 词:host_wakeup_edge_pre_span 可沿用或铸 sibling token(状态席措辞变体,R2' 六处同步);榜面:tieba flag 一条 13.959ms 新 ⛓ 行=**榜序显著重排**(旗舰双复核必配);与既有 R3 span 席的语义重叠披露(span 包络可含 runnable 段,XLANE-2 overlap 披露形复用)是唯一新增披露义务。pin 波及:R3 tests、rank 榜 golden、冷读 typed 面。估 200-400 LOC。

**建议**:**立即实施(四件中唯一)**。收益最大(旗舰窗 +17.9ms、trace 窗 +40.4ms、donghu2955 +10.7ms 可凭证化)、witness 在库含正负臂、机械全为既有模板复用、无架构环依赖。需随批双复核榜序涟漪 + XLANE-2 式 span/状态重叠披露。

---

## 3d 锁二跳(PeerChainStep 1→2)

**现状机械**(核实):`buildCriticalBlockingPeerChain`(query.go:21439)一跳硬界:peer sleep-dominant 时经 peer 自己的 wakeup edge 命名 DirectBlocker,但 hop-2 只给 dominant-state 裸词(`peerDirectBlockerDominantState`,注释明言「naming the blocker's second-hop blocker would be depth 2」);深度=1 是**pinned 形状属性**(TestPeerChainStepFieldSetGolden,peer_chain_a1_test.go:135-157,q1 L31-33 深链爆炸教训)。

**真 trace 二跳形存在率:全库为零**。未截断 span 库存全量过产线 `collectBlockingSpanRows` 准入+解析机(修正了 stats.TraceSpans top-8 generic 帽的量测盲区):
- donghu 窗:5524 spans,349 blocking-like 行(16 lock_contention+4 monitor_contention),解析出 holder 仅 6 行,**payload-direct=0**(全部 wakeup_edge 推断——owner tid 全是容器 ns id 37xxx/38xxx,LOCKNS ns 梯域),两跳=0;锁等时长全部 µs 级(0.011-0.191ms)。
- tieba 窗:798 spans,84 lock_contention,解析 9 行,payload-direct=0,两跳=0(0.004-0.066ms)。
- big_trace(67M,460 条 Lock contention):分窗扫同样零二跳零 payload-direct。
即:3d 要求的「每跳独立 typed blocking_span 对」在库里连**一跳**都凑不出一条(全部解析经推断车道,依 F2 纪律推断不得当直接凭证),二跳=纯合成 fixture 工程。

**顺带实测发现(评估副产物,非本批修)**:blocking_span 车道消费 `stats.TraceSpans`,而该清单是 duration-desc **top-8 generic + 语义帽**视图(boundTraceMarkSpansWithInfo,query.go:9616)——旗舰窗 20/84 条锁 span 全部低于时长帽而对锁车道不可见(今天它们 µs 级无实害;若未来客户 trace 出现帽下大锁等,3d 之前先撞这个可见性口)。

**牵动面**:若实施=推翻 pinned 设计判定(depth-1 golden+q1 教训),PeerChainStep 结构扩展(wire 面)、显示 summary 词面、cap=2 导出常量;LOCKNS 依赖:payload 对凭证在容器 ns trace 上先决于 ns 梯解析成熟(LOCKNS-FIX 在队列)。估 150-300 LOC+裁定。

**建议**:**需新裁定 + 缓办**(四件中最后)。零 witness、零收益 ms、要推翻既有 pin 与教训、且前置依赖 LOCKNS-FIX。除非客户回访交付含真实多级锁链(payload-direct 对逐跳可证)的 trace,不应开批。

---

## 3e binder 多跳(P9 事务对逐跳)

**现状机械**(核实):`findBinderWaitsForChain`(query.go:13974)对链上 sleep/D 节点匹配 P9 sync-request 事务对铸 binder_wait RootEvidence(reply/waker/pacing 三写销臂);IPCEdges 独立面**不进 chain.Edges 不递归**(server 在等什么不入链);server 恢复 client 的物理动作已有 sched_wakeup 链兜底(审计 §二-3 原文确认)。

**多跳事务链在库实测:零**。四旗舰窗 nested sync pair=0(donghu2955:40 edges/18 sync;donghu17267:20/5;tieba:2-3/1);全 trace 扫(receiver 已解析的 sync 对 16/17 条)nested=0;big_trace(14200 条 binder_transaction)分窗扫 nested=0——且暴露环境约束:**密 trace 分窗解析下 binder 配对整体 fail-closed**(`binder_pairing_fail_closed=true; windowed_pairing_topology_incomplete`),多跳凭证在大 trace 上连量测通道都没有,除非全量索引可建。

**与 IPC 图独立面的关系**:3e 的正确落点是「binder_wait 的 server 侧再挂一层 P9 对」而非把 IPC 边灌进 chain.Edges(后者违反边分类学:binder 对是被依赖边不是唤醒边);cap 沿链深度预算可行但今天无消费对象。

**建议**:**缓办候选**(排 3d 之前)。机械路径清晰(findBinderWaitsForChain server 侧迭代,200-400 LOC)、不推翻任何 pin,但库内零 witness、物理路径已被 wakeup 链覆盖(增量=语义标注非新 ms);等客户 trace 出现真实嵌套 sync 形(如 system_server 转发链)再开。

---

## 总排序建议

| 件 | witness 在库 | 实测收益 | 实施规模 | 涟漪 | 建议 |
|---|---|---|---|---|---|
| **3c 裸 census 边状态席** | ✅ 四案 2+6+3+6 hosts | **+17.9/40.4/10.7/0.3 ms 可凭证化;SCAN-3 61839 判例正收** | 200-400 LOC,全模板复用 | 中(榜序重排需双复核) | **立即实施** |
| **3b D/IO 闭合铸节点** | ✅ tieba 2 段(27.1% ms 覆盖) | 2 供者节点,数值席 0.118ms | 400-700 LOC+IO 配对 pre-pass 重构 | 大(锚窗/RSPA/指纹) | **缓办候选**(实施时并「无边候选披露」,先解配对环依赖) |
| **3e binder 多跳** | ❌ 全库 nested=0 | 0 ms(物理路径已兜) | 200-400 LOC | 中 | **缓办候选**(待真嵌套 witness) |
| **3d 锁二跳** | ❌ 两跳 0,payload-direct 一跳也是 0 | 0 ms(库内锁等全 µs 级) | 150-300 LOC+推翻 depth-1 pin | 中(wire+pin 反转) | **需新裁定+最后**(前置 LOCKNS-FIX;待客户多级锁 trace) |

裁定点(标委托默认待追认):①3c 状态席 basis token 沿用 host_wakeup_edge_pre_span 还是铸 sibling;②3b D-lane 准入是否入 XLANE-3 板身份参数闭集;③3d 若启用需显式推翻 q1 L31-33 depth-1 裁定。
副产物备案:blocking_span 车道 top-8 span 帽可见性口(µs 级今天无害,大锁等形会先撞);「无边 ≥地板候选零披露」库内零实例,归 3b 顺路。
