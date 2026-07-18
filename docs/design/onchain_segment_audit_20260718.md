# 命题3 分段侧审计 — 同线程行的时间分段与逐段入链判定现状

审计官:分段侧。基准 = 用户命题3 原文:「即便是同线程内的边也是分段的,如果其中某段没有到关注线程的单跳或多跳点唤醒边和被依赖边,也不能作为链上。」
代码基准 = main 8a6e327a9(净树,git archive 副本探针);真 trace = /Users/han/opt/customlogs/xxx_all.systrace(tieba 59566)。
探针 = scratchpad/segaudit_copy/internal/tracequery/zz_segaudit_probe_test.go;全量输出 = scratchpad/segaudit_probe_out.txt。

## 1. 分段机制现状(per lane)

### 1.1 锚窗的铸造(判定基座)

- `chainAnchorWindowsByPID`(rank_chain_anchor_rspa.go:103):锚窗 = 链中该 pid 全部 depth>0 节点窗的排序合并并集。**每个节点窗不是任意时间窗**:expandChain(query.go:22775-22793)只在 waiter 的 S-sleep「最趣区间」上追唤醒,子节点窗 = [waiter 该睡眠段起点, 本 pid 交付的 sched_wakeup.Ts]——窗的右端点**就是本 pid 指向链方向的真边时间戳**。多级链 = 逐层嵌套的边闭窗(depth k 窗 ⊂ depth k-1 的等待段)。
- 因此锚窗∩不是裸时间 proxy:它是「**边闭窗内含**」= R4 成文公理(边=凭证,边前=有效,边后=解除)的机械形。窗级上与「该段处于一条到关注线程的单/多跳边的边前作用域」**等价**;段级(亚窗)上是**继承凭证**,见 §3。

### 1.2 各账行的段结构

| 行族 | 真段清单载体 | 逐段入链判定 | 判定信号 |
|---|---|---|---|
| runnable 窗席(window_stats) | 账本关账点段级累计:query.go:5510 `runnableIntervals` + 5463-5468 每段 `anchoredOverlap`(单一关账点,full=anchored+remainder 精确二分);铸席时 `ledgerAnchorStamped`(14894) | RSPA `reanchorOnChainStateSeats`(rank_chain_anchor_rspa.go:656):case A/A'/B 四臂,⛓ 锚定份 + ◇ 余数席 | 段∩锚窗(边闭窗继承) |
| D/IO 窗席 | `dioIntervals`(query.go:5512-5526,帽 32 全有或全无)+ 段级 anchored;铸席 stamp 15558-15560 | 同上 D/IO 臂(rspa.go:966-1014) | 段∩锚窗 |
| critical_blocking D/IO VIEW 行 | `credentialSegments`(types.go:4573,同 dioIntervals 供体)+ hull(reconStartTs/EndTs=嘈声,只判 ∅) | HULL-CRED 四臂(query.go:20845-20892):hull∩=∅→demote;段清单有效→逐段∩锚窗,≥1 真交→keep⛓+公示 `ChainCredentialSegments`;全不交→demote+披露;清单缺席→keep+「(包络级凭证)」 | 段∩锚窗,**per-segment 公示** |
| wakeup_chain 链席(causal/aggregated_impacts) | 值本身即跳窗内 clip 的逐段累计(summarizeWakeupCausalImpact 22880-22915);聚合席携 `OccurrenceWindows` | 定义即窗内(边闭窗) | 构造性 |
| scheduler_latency / low_frequency 卫星 | `familyMemberIntervals`(须能复算自身标量,`rspaRowIntervalAnchoredMs`:460 否则 fail-open) | 锚定份→◇ 余数形;全锚定→XLANE-1/2 coverage proof(段清单∩链席段清单 Σ≈全额才代表化降道,rspa.go:915-921);清单不可证→全席 ◇(值不动) | 段∩锚窗 + 段∩链席段(精确) |
| priority_inversion_runnable_wait / cpu_affinity_or_cpuset | 无(不可分账,§29.83 件③) | R4 lane 臂:任何未锚定份→**全席** ◇,值不动(rspa.go:792-842) | 整席 verdict,无逐段 |
| blocking_span(锁) | XERR1-FIX(types.go:4392-4453):值收敛到 waiter 的 Σ(sleep+D+iowait) 段 ∈ span∩窗(`BlockingValueBasisWaitSegments`);包络值降词面;WaitBudgetExceeded/WaitCoveragePartial 诚实臂 | 凭证=typed waiter→holder 对(边等价),RSPA 豁免表「already credential-anchored」 | typed pair + 段基值 |
| semantic span(jit/gc/纹理…) | 值 = span∩同线程链窗交(rank_family_fold.go:189,union-of-intersections 防双计) | 交>0→on_chain;R3-IMPL host_wakeup_edge 基(query.go:17461-81):按宿主指向 target 的真边**逐边界二分**,边前 ⛓ / 边后 ◇ clone——全系统最严格的段级边判定 | 交(proxy)/ 边界二分(边) |
| self 席(target 自身) | 自身窗席有 memIvs,但**无逐段门** | 恒链上(XLANE-1):`chainAnchorWindowsByPID` 排除 target(rspa.go:106);decisions 跳过(297);`rspaRowIsSelfExempt`(342);基=self_wall_clock_interval / self_deterministic_span 诚实词 | 全额,定义性 |
| io_latency / page_cache / block_io 等 | 无段清单臂 | `chainContextForCandidate`(query.go:18179)裸 hull∩节点窗 | **纯时间 proxy** |

默认道:一切无专门臂的行走 `chainContextForCandidate` = 行窗∩同 pid 节点窗 >0 → on_chain(18203-18207)。这是唯一残存的裸 overlap 硬判面。

### 1.3 proxy 还是等价 — 结构裁定

**锚窗∩ = 「窗级边凭证 + 段级时间继承」**,与命题3 的关系取决于读法:

- 读法 A(边前作用域读法,系统成文 R4):段落在某条真边的边闭窗内 ⇔ 该段处于该边的边前有效期 ⇒ **等价**(每扇锚窗右端点就是该 pid 的真边)。
- 读法 B(严格逐段边读法,命题3 字面):要求「该段」自身可归因到一条到关注线程的边(该段的工作在为链上请求服务)⇒ **proxy**。两个偏差形都真实存在且已在真 trace 量化(§2):
  - 正形(窗内无凭证段):pid 在边闭窗内兼做无关服务——时间内含无法区分「推进链上请求的 ms」与「同窗服务第三方的 ms」。实测 60555 在自己锚窗内发出 24 次对第三方(SaInit0/hmfs_txn/udk-irq)的唤醒 vs 仅 3 次对链成员;其锚定 ms 的 97.8% 落在段内无任何指向链的边的段里(多数是合法边前 D/IO,但混入第三方服务不可分离)。当前判成 ⛓ 全锚定。
  - 反形(段有真边但落锚窗外):真边存在但链没走那扇窗——成因全部是链构造上界:MaxBranches top-N(12750-53,有 caveat)、depth≥1 每节点只递归**单个**最趣区间(22724)、MaxDepth、cycle guard。实测 59843 有 5.436ms(带边段时间的 20.7%)、60595 有 5.173ms(26.0%)真边段未锚定。当前判成 ◇ 余数(宁漏勿猜,方向诚实)。

## 2. 真 trace 实证(tieba 59566,窗 34579.472865..34579.587805 = 114.94ms)

链:24 节点 / 16 边 / 8 分支 / 深≤3;链上非 target pid 三个:59843 CookieMonsterCl(d1,锚窗 8 扇 Σ64.9ms)、60595 NetworkService(d2,5 扇 Σ42.1ms)、60555 ThreadPoolForeg(d3,3 扇 Σ25.0ms)。

四象限(runnable+D/IO 普查段;边 witness = 该 pid 段内[+0.5ms 闭合容差]发出的指向链成员的 sched_wakeup/waking):

| pid | 全额 ms | 锚定∧段内有边 | 锚定∧段内无边(proxy-only) | 未锚定∧有边(漏计) | 未锚定∧无边 |
|---|---|---|---|---|---|
| 59843 d1 | 26.738 | 20.823 | 0.419(锚定的 **2.0%**) | 5.436(带边时间的 **20.7%**) | 0.060 |
| 60595 d2 | 20.342 | 14.689 | 0.011(0.1%) | 5.173(**26.0%**) | 0.469 |
| 60555 d3 | 19.659 | 0.424 | 18.641(**97.8%**) | 0.199(31.9%) | 0.395 |

- 60555 的 97.8% 是读法 A/B 分歧的极端体:它的 D/IO 段(4.269/4.265/4.256/2.389ms 等)全部窗内、段内零前向边(前向边在窗末:34579.533952/553152/587602 EDGE 行)。按 R4 边前语义合法;按严格逐段边语义无凭证。同窗 EDGECENSUS to_third_party=24 证明确有第三方服务混入同一锚窗。
- 59843/60595 的 20.7%/26.0% 漏计是链上界实锤:如 59843 cpu=1 的 0.716+1.661+1.411ms 段各自携真边(段内即有对链成员的唤醒)却锚定=0——对应的 target 睡眠分支未被展开(qualifyingBranches>8 的 caveat 在链上)。
- RSPA 二分在飞:三个 ◇ 余数席(59843 rem 5.496 of full 26.738 anch 21.242;60595 rem 5.642 of 20.342 anch 14.700;60555 rem 0.278 of 1.524)。前两席 `divergent=true`(chainLane 19.933 vs census 21.242;14.597 vs 14.700)= case A' 双账披露臂在真数据上触发。60555 runnable 窗席被 clip 成 ⛓ 锚定席 1.246(case B split)。
- HULL-CRED 在 bundle 路径实测触发:60555 io_wait 4.884 行携 credSegs=10(keepSegmentVerified),两条 d_state 行各 credSegs=1。**直连 BuildCriticalBlockingCalls(query.go:20482-85 → 366 getStats())走无锚 stats,四臂整体跳过**(RNB-1 成文 fail-open)——同一行在两个入口 lane 判定机器不同(bundle=逐段判;直连 view=裸 overlap)。
- 小窗复核(34579.472865..475857,3ms):59843 全额 2.377 全锚定,其中 0.716ms(30.1%)段内无边(首段 472865..473581,窗闭边在 475843 落于后段)——单跳单窗形下 proxy-only 份同样存在。
- 偏差率总结:读法 A 下 proxy 偏差 = 0(定义等价);读法 B 下 proxy-only 上界 = 2.0%/0.1%/97.8%(依 pid 角色:纯管道线程≈0,共享 worker 极高);反形漏计 20.7-31.9%(两种读法下都真实,方向=漏不冒)。注意本探针的段内边 witness 是**合法性的下界**(D/IO 段的前向边必然在段后:线程阻塞时发不出唤醒),真「与链完全无关」子集在内核调度数据里不可确定性分离(需 binder txn 级请求追踪)。

## 3. 自身行的分段(命题1×3 交互)

现状 = 自身**不分段全额入链**,三处豁免闭环:锚窗铸造排除 target(rspa.go:106「self-causality: fully anchored by definition」);RSPA decisions 跳过 target pid(297-300);`rspaRowIsSelfExempt`(342-350,含 self-basis 行)。自身席带段清单(memIvs)但无逐段门,基词 = self_wall_clock_interval / self_deterministic_span(诚实 causality 词,非 wakeup-edge 冒称,query.go:18273-18293)。

**开放问题(不擅断)**:目标自身的零跳「边」是否使其每一段自动有效?注意结构事实:target 的 depth-0 节点窗只是 top-8 被展开的睡眠分支;自身 runnable/D 段落在**未展开分支间隙**里的部分,同样以 self 基全额入链——若用户模型要求自身也逐段(如只认自身处于被展开等待结构内的段),现状不满足;若 XLANE-1「自身恒链上」即终裁(每一 ms 自身耗时都直接延迟自身),现状满足。需用户裁定。

## 4. 命题3 verdict

**已满足「逐段+边」**(边=窗级凭证段级继承,读法 A):
1. RSPA runnable/D-IO 窗席二分(⛓锚定+◇余数,单关账点精确);
2. HULL-CRED critical_blocking D/IO VIEW 行(bundle 路径,逐段∩+per-segment 凭证公示);
3. R3-IMPL host_wakeup_edge 语义 span(逐边界二分——唯一严格读法 B 级的臂);
4. XLANE-1/2 卫星 coverage proof(段清单∩链席段清单,精确信号);
5. blocking_span/binder_wait(typed pair 凭证 + XERR1 段收敛值基)。

**时间 proxy(部分满足)**:
6. `chainContextForCandidate` 默认道(hull∩节点窗)——io_latency 等无臂行的唯一判定;
7. 语义 span 无宿主边时的 span∩链窗交(值段级 clip,凭证仍窗含);
8. 包络级 keep 臂(段清单缺席/超帽 32,诚实词但无逐段);
9. 直连 critical_blocking view(anchor-less fail-open,成文保守)。

**整席无分段**:
10. priority_inversion_runnable_wait / cpu_affinity_or_cpuset(已裁不可分,全席 lane verdict——非缺口);
11. self 席(§3 开放);
12. 聚合链席行本身(值窗内构造,发布为聚合;OccurrenceWindows 携段)。

**与命题3 的两处实质差距**(读法 B 下):
- 正形:窗内第三方服务 ms 无任何门可排除(60555 形)——内核数据下确定性不可分,只能靠披露(候选:把 EDGECENSUS 型「同窗第三方唤醒计数」做成软披露词面,嘈声信号只做软引导,符合 §1 红线);
- 反形:真边落未展开窗外 → 20-26% 漏计到 ◇。机械收敛候选:`attachWakeupEdgeCensus`(query.go:12771)已全量扫 wakeup 库存,可对 (waker=pid, wakee∈链闭包) 的每条真边铸最小边闭微窗 [该边前 wakee 睡眠段起, edgeTs] 并入锚窗并集——载体零新面(锚窗即 TimeWindow 并集,anchoredMs 已段级),成本 O(边数×log 窗数);语义上等于把「多跳边」的枚举从「链遍历到的」放宽到「库存里可证的」,方向仍是精确信号进硬门。
- 边 witness 下推到段级的载体成本:段清单载体已全部存在(runnableIntervals/dioIntervals/familyMemberIntervals/OccurrenceWindows/credentialSegments + ChainCredentialSegments wire 前例);主要成本在 (a) dioIntervals 帽 32 全有或全无——超帽即整席退包络级,下推需分块或提帽;(b) runnable 族 rank 席尚无 per-segment 凭证公示面(HULL-CRED 只覆盖 D/IO VIEW 行),推广是 wire 面复制(member_line_ranges 前例)而非新算法。

## 5. 开放问题清单

1. 自身账逐段与否(§3)——命题1×3 交互,需用户裁定。
2. 命题3「该段有边」采读法 A(边前作用域,R4 现状)还是读法 B(逐段可归因)?60555 形(共享 worker 同窗兼服)按哪种裁。
3. 反形漏计(真边未展开,20-26%)是否值得边库存微窗补录(机械候选见 §4)?现状方向诚实(漏不冒)。
4. io_latency / 无臂行的裸 overlap 链道是否需要段/边臂。
5. 直连 critical_blocking view 与 bundle 路径的判定机器不一致(anchor-less fail-open vs 逐段四臂)——同行两面 lane word 可能不同,是否收敛。
6. dioIntervals 帽 32 的全有或全无在长账行上系统性退包络级——分块公示是否立案。
