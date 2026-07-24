# PARTDISC-1 / CENSAME-1 方案设计 + CLUSTERTIE F2 活体技术档案

> 日期:2026-07-24
> 审计基线:`main@053e56049ae2062070cb00f125bdd802603cc6fd`(git rev-parse HEAD 自证,工作树净)
> 性质:只读审计 + 方案设计文档,本轮零生产代码/测试改动。交付对象:后续实施同事。
> 审计方法:三路代码深读(PARTDISC/CENSAME/F2)+ 一路账本 verbatim 提取,每路双复核(锚点逐条核对 + 对抗证伪);F2 路对抗复核在 git archive 快照上注入探针测试**实跑产线代码实证假并可达**。全部复核 corrections 已收编入本文(收编处以【复核修正】标记)。
> 权威出处:战役账本 `docs/design/real_trace_campaign_20260705.md` §29.206(:3558-3559)/§29.213(:3577-3578)/§29.195(:3516-3517)/§29.210(:3568-3569)+ 三处备案码注(F2=`internal/tracequery/cluster_freq_share.go:680`、F3=`internal/tracequery/core_capability.go:626`、F4=`internal/tool/answer_document_mutation_runtime_tree.go:4893`)。`trace_analysis_open_gap_ledger_20260710.md` 对三备案零条目(两轮 grep 复核证实)——立案与状态追踪唯一权威=战役账本+码注。

---

## 0. 排期定位与总约束

§29.213 用户裁定(campaign:3577-3578,用户 verbatim「都按照你推荐的最优方案来。」)排期总序七批,§29.218 确认 5/7 收官(campaign:3593「件6 PARTDISC-1/件7 CENSAME-1 待续」)。本文覆盖:

- **件6 PARTDISC-1**(本文 §A):裁定原文逐字=「**PARTDISC-1**(CLUSTERTIE F3 披露扩臂:partition_below_floor/partition_drift/partition_limits_veto 因子并报,零 gate)」
- **件7 CENSAME-1**(本文 §B):裁定原文逐字=「**CENSAME-1**(CLUSTERTIE F4 普查谓词与渲染点同源+零渲染行负臂 pin;CHAINGUARD P4 R8 兜底臂 AST/结构看护)」
- **F2 活体档案**(本文 §C):裁定原文逐字=「③**F2 亚周期游移=维持记档等活体**(备案 fix_direction 在案,活体出现再修)。」——**F2 不随件6/件7 开工**,本文 §C 仅为活体出现时的开工档案。

### 约束清单(实施期逐条对照;出处已复核逐字)

| # | 约束 | 出处 |
|---|------|------|
| C1 | PARTDISC-1 = 披露扩臂,**零 gate**:不得改变任何聚类/判簇决策字节 | campaign:3578 |
| C2 | F3 病理边界:既有披露臂集是 witness-lane only(no_samples/co_witness_floor/transition_conflict);分区侧 fired/snapshots/漂移/limits-副证零披露载体(struct 私有、零 query-face 消费者)。扩臂只补披露,不动 witness-lane 判定 | campaign:3559;core_capability.go:626-631 |
| C3 | 因子同构范式=「transition_conflict(两侧 kHz+偏斜因子)/co_witness_floor(见证数)」;§29.195 修复轮 P2-F1 确立「derive 侧记录 + 审计直发因子」纪律 | campaign:3517 |
| C4 | F4 收窄目标=普查谓词与渲染点同源 + 零渲染行负臂 pin(先红后绿) | campaign:3578;tree.go:4893-4904 |
| C5 | CENSAME-1 第二半=CHAINGUARD P4 R8 兜底臂 AST/结构看护,承接 §29.210 记档候办「census=target_self 无 basis 兜底形 AST 看护」(R8 self 臂曾「网先抓了自己一个漏」后补) | campaign:3578、:3569 |
| C6 | F2 不修,维持记档等活体;客户恒值形结构性不可达(本文 §C.3.3 三层论证) | campaign:3578、:3559;cluster_freq_share.go:680 |
| C7 | 件6/件7 为排期总序之末两批,依序推进,每批旗舰双复核+终局范式 | campaign:3578、:3593 |
| C8 | 三备案统一 P3 定级(「CLUSTERTIE-1 dual review 2026-07-21, P3, 记档不修」) | 三码注 |
| C9 | 既裁边界:§29.212 明文「G15/G16=既裁维持…§29.206 F2-F4 系公告分区条目不覆盖」——不得与 G15/G16 混同重诉;「披露不改判定」原则适用本批 | campaign:3575 |
| C10 | 若设计需新 typed wire signal,须按 R2' 同步全部同步面(近期同构参照:census note 单发射 helper R2' 七处)【复核修正:出处=campaign:3569,非 :3556】。本文两方案均**不新增 wire signal**,规避该义务 | cluster_freq_share.go:675-678;campaign:3569 |
| C11 | open gap ledger 无条目;收账落战役账本新节 | ledger 全文 grep 零命中 |

通用红线(CLAUDE.md/记忆红线,实施期 BLOCKING):精确信号才能进硬门,嘈声只作软引导;用户面零内部术语(R3/R4/wire 拼写/fail-open 等机制词禁出厂);渲染面 pin 先红后绿留证据;唯一行为变更的发布接线必配 e2e 正向 pin(RUNSPLIT-1 M4 教训);推送前向用户确认。

---

# §A 件6 PARTDISC-1:分区拒并因披露扩臂(零 gate)

## A.1 病根与现状机制图

### A.1.1 分区车道结构

- 推导入口:`deriveClusterFreqDomainsLimits`(cluster_freq_share.go:402)在 pairwise 见证扫描前一次性计算 `snap := deriveAnnounceSnapshotPartition(sampled, timelines, limits)`(:466)。
- 载体:`type announceSnapshotPartition struct { fired bool; snapshots int; groupByCPU map[int]int }`(:693-697)——**私有、零披露字段;全仓唯一消费者=`sameGroup`**(全仓 grep 复核证实,类型名连测试文件都不出现)。
- 消费点:pairwise 循环 `snap.sameGroup(i,j)`(:485-504)命中铸 proEdge;con>0 分支先行 `continue`(:475-479,见证矛盾压过分区)。
- 判定链:`deriveAnnounceSnapshotPartition`(:713-830):全局 ts 排序正值事件 → 15µs 链距成 burst → full 快照精确判(`judgeBurst`,:752-790)→ 分区恒定 → 快照地板(≥2)→ limits 副证 sub-veto(:808-827)。固定常数零自适应(:141 15µs、:1330 地板 2)。

### A.1.2 分区车道全部拒绝/降级/fail-open 出口

| # | 出口 | 锚点 | 返回形 | 「跑过被拒」vs「从未跑」用户可见字节 |
|---|------|------|--------|--------------------------------------|
| E1 | `len(sampled)<2` | :714-716 | 零值(fired=false) | 相同(语义上即「未跑」,可接受) |
| E2 | 零正值事件 | :734-736 | 零值 | 相同(同上) |
| E3 | **分区漂移**(值分组跨快照变化) | judgeBurst false :784 → 整体零值返回 :795-797 | 零值,此前累计 snapshots/漂移点全部丢弃 | **逐字节相同——F3 病根主形** |
| E4 | **快照地板**(refSig==nil ∨ snapshots<2) | :800-802 | 零值;snapshots==1「见过一次完整快照」事实丢弃 | **逐字节相同——F3 病根主形**(refSig==nil⟺snapshots==0,与 snapshots==1 今天也无法区分) |
| E5 | **limits 副证 sub-veto**(某值组 ≥2 个不同正值上界) | :822-823 `continue` | fired=true 但该组成员不入 groupByCPU | 被否组「为何被拒」零披露字节。【复核修正】「全局逐字节相同」仅在被否组为唯一值组时严格成立;多值组混合形下其余组分区合并照常生效,全局判簇结构可分辨——但核心主张(该组拒因零披露)不受影响 |
| E6 | partial burst skip(缺行/骑跨/重复 cpu) | :753-755、:760-764 | 该 burst 不计不否 | 相同(设计中性;全 partial 归并到 E4 的 refSig==nil 形) |
| E7 | 消费序压制(con>0 对先 continue) | :475-480 | 分区合并被见证矛盾压制 | 部分已披露(conVeto 车道,CLUSTERSTREAM F1 已修;judged 终判则静默,见 A.4(a) 不收编裁定) |

结论:**E3/E4/E5 三个真拒并出口在所有用户面(split_audit 字符串、freq_only reason token、engine caveat、tracediag key_first 行、树面成因词)与「分区从未运行」不可区分**。客户 N1 回放实证词面(cluster_announce_partition_test.go:12-13):`"cpu0↔cpu1 @925.310393 判定臂=co_witness_floor(共见证变迁不足:共见证=0(<2))"`——分区跑没跑、为何拒,零字节可见。

### A.1.3 F3 备案码注【逐字】(core_capability.go:626-637;已与源码逐字复核)

```
// 备案 复核 F3 (CLUSTERTIE-1 dual review 2026-07-21, P3, 记档不修): the
// announce-partition lane's refusal causes never reach this disclosure face —
// the arm set here is witness-lane only (no_samples / co_witness_floor /
// transition_conflict), while the partition's fired/snapshots/漂移/limits-副证
// state has no disclosure carrier at all (struct is private, zero query-face
// consumers). A customer replaying a drift or double-ceiling fleet file that
// still lands freq_only sees the same co_witness_floor wording as the
// pre-partition §29.200 filing and cannot tell the partition lane ran, nor
// why it refused. Same family as CLUSTERSTREAM 修复轮 F1 (真否决静默=须修).
// fix_direction (报告 verbatim): 披露性扩臂(零 gate):split_audit 或 caveat
// 增 partition_below_floor/partition_drift/partition_limits_veto 因子(与
// transition_conflict 因子同构),freq_only 且分区曾运行时并报。
```

## A.2 freq_only 披露链 plumbing(改动定位依据)

引擎侧:
- `coreCapabilityMap.freqOnlySplitAudit`(core_capability.go:257)/`freqOnlyReason`(:263)。
- split_audit **仅两个铸点**(grep 穷尽复核):>4 簇 overflow 臂 :511;fmax_tie 臂 :555(:553 先清 stale tie-break 审计)。格式化单点 `capabilityFreqOnlySplitAudit`(:638-684),词形 `cpu%d↔cpu%d @%.6f 判定臂=%s(%s%s)`(:683)。
- reason 铸点:no_domains :449/:458;no_sampled_cluster/single_cluster/cluster_overflow :487-491;fmax_tie :546;comove_floor(±single_burst) :409-411。

wire 侧:
- `SupplyFoldBasis.CapabilitySplitAudit`(JSON `capability_split_audit`,supply_fold.go:137)与 `CapabilityFreqOnlyReason`(:147),铸点 supply_fold.go:740-743(仅 freq_only 时挂)。
- gated 车道 reason 孪生:query.go:24027 → 聚合 :13542/:13575/:13598/:17436/:17541。

用户面出厂点:
- **engine caveat(中英双语)**:`capabilitySplitAuditCaveat`(query.go:26432-26444,接线 :26273),first-hit 合同,census 走 `scanResultSupplyFoldBases`(:26369-26416)。
- **rich note → 投影 → 树面**:reason token 专属通道(split_audit 不走);词面单点 `runtimeTraceProjFreqOnlyCauseShort`(answer_document_mutation_runtime_supplyfold.go:174-212)。
- **tracediag**:render_key_first.go:756 `capability_split_audit=%s`。

**载重结论:split_audit 是纯 zh+token opaque 字符串,只经 SupplyFoldBasis(wire JSON)→ engine caveat → tracediag 三面出厂,不进树面板词/图例/wrap 原子表;板面词只消费 reason token。在 split_audit 字符串内扩子句 = 零新 wire 字段、零 note key、零投影字段、零图例/wrap 触点。**

## A.3 同构参照:transition_conflict 因子同步面(R2' 实数)

1. token 常量 `freqCoMoveSplitArmConflict = "transition_conflict"`(cluster_freq_share.go:1137);2. zh 词面 mapper `freqCoMoveSplitArmZH`(:1149-1150 「同窗异值变迁」);3. typed 因子载体 `freqCoMoveSplit`(:1111-1121)+ `clusterFreqConVeto`(:154-159)+ `clusterFreqDomains.conVetoes`(:182-185);4. 铸点 `freqWitnessCoMoveDiag`(:1294-1296)+ `recordConVeto`(:547-560)+ 审计注入(core_capability.go:665-671);5. 格式化子句(core_capability.go:675-676);6. wire=opaque 字符串随 CapabilitySplitAudit(无 schema enum/note key/投影字段);7. caveat lift query.go:26442;8. tracediag render_key_first.go:756;9. 文档:账本【复核修正】§29.195(real_trace_campaign_20260705.md:3517)+ customer_revisit_guide_20260720.md:90(教客户 grep `判定臂=transition_conflict`);10. pin:TestClusterStreamConVetoRecordFeedsSplitAudit(cluster_stream_test.go:295-353【复核修正:文件全长 353 行】)、core_capability_cap3_test.go:463 臂词面、:484 basis 披露、:533 端到端 caveat、caveat_lift_breadth_test.go:71/:120-127 first-hit 合同。

对比:若走「新 typed wire 信号」路线(SupplyFoldBasis 新字段/新 note key),同步面膨胀到 10+ 处(含 tracediag skip-list 三处逐字段记账、note key 双表、投影字段+解析、图例完备性 pin、wrap 原子表)。**新因子取 opaque-string-clause 同构,同步面收敛在 tracequery 包内+文档+pin。**

## A.4 同族收编裁定:CLUSTERSTREAM F1「真否决静默」残留三路径

F1 主修已落地(conVetoes 记录 :519-560 + 审计直发 core_capability.go:664-672 + pin cluster_stream_test.go:295)。残留三条:

- (a) **judged 终判(2-4 簇)∧ conVetoes 非空**:否决真实塑形发布结构,但 judged 依合同不铸 split_audit(cap3_test.go:239 负臂 pin)。**裁定:不收编**——否决即结论本身,healthy 判决上逐边披露是噪音(噪音从源头消除红线);与「freq_only 且分区曾运行时并报」射程对称。
- (b) **freq_only 非碎片臂(comove_floor)∧ conVetoes 非空**:split_audit 不铸(仅 :511/:555 两铸点)。**裁定:不收编**——成因是样本地板非否决,强披露会误归因;记档残留。
- (c) **capabilityFreqOnlySplitAudit 空返路径(:653 无采样代表/:666-667 无匹配记录)∧ 分区拒并**:今天整条审计为空。**裁定:收编成立,即本批主体**——分区拒并因与 conVeto 因子共用同一披露载体(freqOnlySplitAudit 字符串 → CapabilitySplitAudit → caveat/tracediag);分区子句独立于见证审计空返(见证审计为空时分区子句仍单独出厂)。

## A.5 方案设计

### A.5.1 披露载体裁定:struct 扩审计字段 + clusterFreqDomains 随行 + split_audit 子句并报(推荐)

三选项:
- (i) derive 返回 `(partition, reason)` 二元:reason 裸 token 丢因子(快照计数/漂移 ts/被否组名册+上界),不完整。否决。
- (ii) audit sink(回调/日志):日志是软引导面非用户面,违披露纪律。否决。
- (iii) **推荐**:`announceSnapshotPartition` 旁挂 typed 审计小 struct;`deriveClusterFreqDomainsLimits` 拷到 `clusterFreqDomains` 新字段 `partitionAudit`(与 `conVetoes` :182-185 完全同构的「Disclosure input only」位);两个 split_audit 铸点并报子句。理由:conVetoes 先例已证明 derive→domains→audit-string→basis→caveat 是本仓「拒并因披露」既定管道;零新 wire 字段=同步面最小;`sameGroup` 继续只读 fired/groupByCPU,判定字节恒等由构造保证(C1)。

### A.5.2 因子枚举、审计载体与铸点

```go
// cluster_freq_share.go,紧邻 freqCoMoveSplitArm* 家族(:1134-1138)
const (
    announcePartitionRefusalBelowFloor = "partition_below_floor"
    announcePartitionRefusalDrift      = "partition_drift"
    announcePartitionRefusalLimitsVeto = "partition_limits_veto"
)
// zh mapper(freqCoMoveSplitArmZH 同构):
// below_floor→「完整公告快照不足」 drift→「快照间值分组漂移」 limits_veto→「限频上界政策边界矛盾」
```

**注意(F2 交互,§C.6.2)**:因子集按可扩闭集铸,预留第 4 名位 `partition_value_set_veto`(名待批)——F2 活体开工时只做加法。

```go
type announcePartitionAudit struct {
    refusal   string  // "" | below_floor | drift(全信号拒并二选一)
    snapshots int     // 拒并时点已计完整快照数(below_floor 因子;drift 时=漂移前计数)
    driftTs   float64 // drift 因子:漂移 burst 首事件 ts
    limitsVetoGroups []announcePartitionLimitsVetoGroup // fired 形逐组否决(可与 refusal=="" 共存)
}
type announcePartitionLimitsVetoGroup struct { members []int; ceilings []int64 } // 均升序、确定性
```

铸点(全部零 gate):
- 漂移出口 :795-797:返回携带 `refusal=drift, snapshots=<已计数>, driftTs=events[start].ts` 的零 fired 结构(fired 仍 false,sameGroup 行为逐字节不变)。
- 地板出口 :800-802:`snapshots==1` 时携带 `refusal=below_floor, snapshots=1`;**refSig==nil(snapshots==0)保持零审计**——「分区从未跑出证据」的诚实形,披露它会把子句洪泛到一切普通变迁 trace。披露门=精确整数信号(snapshots≥1 ∨ 漂移 ∨ limits 否决)。
- limits 出口 :808-827:`continue` 前收 `{members(升序), 该组 distinct 正值 laneMax(升序)}` 入 `limitsVetoGroups`。**:808 `for gid, members := range groups` 是 map 迭代——审计名册必须按 min(members) 排序后收集保确定性**(现有 continue 无输出序问题,新披露有)。

### A.5.3 plumbing 逐跳触点清单

| 跳 | 位置 | 改什么 |
|----|------|--------|
| T1 | cluster_freq_share.go:693-697 / :713-830 | struct 旁挂审计;三出口铸审计;limits 组确定性排序 |
| T2 | cluster_freq_share.go:167-186 | `clusterFreqDomains` 加 `partitionAudit`(conVetoes 旁,同「Disclosure input only」注) |
| T3 | cluster_freq_share.go:466 后 | `out.partitionAudit = snap.audit` |
| T4 | cluster_rail_evidence.go:573-579 | `refined` 重建时携带 `partitionAudit`。现仅拷 source/sampledAsc/explicitInputIgnored;conVetoes 刻意不携带是因其端点标签细分后失义——**分区审计是全局事实、标签无关,应携带;落注说明与 conVetoes 的不对称理由** |
| T5 | core_capability.go:511 / :555 | 两铸点改 `joinSplitAuditClauses(capabilityFreqOnlySplitAudit(...), capabilityPartitionRefusalClause(domains))`,";" 连接;见证审计为空而分区子句非空时单独出厂(A.4(c)) |
| T6 | core_capability.go:593-637 | 函数头合同更新;F3 备案码注改 EVOLUTION RECORD(备案销账) |
| T7 | supply_fold.go / query.go / tracediag | **零改动**——子句随 opaque CapabilitySplitAudit 自动过 :741 铸点、:26273 caveat lift、render_key_first.go:756 |
| T8 | docs | 账本收账节;customer_revisit_guide_20260720.md:90 grep 指引补 `分区车道=partition_*`;architecture.md 无 split_audit 章节、不触 |
| T9 | 测试 | A.6 NP1-NP7 |

### A.5.4 中英词面草案(用户面白话;`token(zh)` 并置文法=本仓已出厂披露文法,cluster_freq_share.go:1140-1142「greppable AND readable」)

- drift:`分区车道=partition_drift(公告快照分区已运行:此前完整公告快照%d次,@%.6f 快照内值分组发生变化,分区证据整体弃用)`
- below_floor:`分区车道=partition_below_floor(公告快照分区已运行:完整公告快照仅%d次(<2),证据不足,未参与判簇)`
- limits_veto:`分区车道=partition_limits_veto(公告快照分区已运行:值组[cpu%s]带%d档不同限频上界(%s),按政策边界矛盾该组未合并)`
- EN 面:随 engine caveat 包裹句既有通用 EN 释义出厂(query.go:26443);与既有 `判定臂=co_witness_floor(...)` 逐臂无独立 EN 先例一致。可选加固:caveat 包裹句含 `分区车道=` 时追加一句英文短释(精确 substring 信号)。
- 缺席语义:分区从未跑/从未见完整快照/跑过且成功——三形均**零新字节**(absence never narrates)。
- zh 子句禁「fail-open/铸/veto」等机制词,用「弃用/未参与/未合并」白话(红线④)。

## A.6 测试计划

现有 pin(全部维持,零改动预期):cluster_announce_partition_test.go 全文 7 pin(客户 200-sweep 判三簇 :72-111、intact 快路 :115-124、漂移 fail-open :129-150、快照地板 :155-179、单值组诚实合并 :185-199、limits sub-veto :207-245、真文件端到端 :252-303)——**注意:漂移/地板/sub-veto 三 pin 只钉判定不钉披露,正是 F3 测试面缺口**;cluster_stream_test.go:295;cap3_test.go:239/:463/:484/:533;caveat_lift_breadth_test.go:71/:120-127。既有断言全 `strings.Contains`(已核 :127/:463/:510-511/:518/:559、cluster_stream_test.go:339-344),前缀保留+";" 追加子句不红。

新 pin:
- **NP1 渲染面先红后绿(红线⑤)**:`TestRunLiftsPartitionRefusalCaveat`——真 ftrace 漂移形 BuildIndex→Run,断言 caveat 含 `capability_freq_only_split_audit=` 且含 `分区车道=partition_drift`;先落 pin 在基线跑红留证,再实现转绿(同构 :533)。
- **NP2 字节区分 pin**:同为 freq_only(fmax_tie)两 fixture——(i)分区从未见完整快照(纯变迁碎片形)、(ii)跑过一次完整快照后漂移——断言两者 CapabilitySplitAudit 不等,且 (i) 不含 `分区车道=`(负臂)。
- **NP3 三因子逐臂 unit pin**:漂移形→partition_drift+快照计数+漂移 ts;单快照形→partition_below_floor;limits 双上界形→partition_limits_veto 含名册+两档 kHz。每臂同时断言 members/groupCount/freqOnlyReason 与批前逐值相同(**零 gate 恒等主证**)。
- **NP4 分区成功形负臂**:客户 200-sweep judged→split_audit 保持 ""(既有 :239 覆盖);single_cluster 形→无 `分区车道=` 子句。
- **NP5 refSig==nil 静默臂**:普通变迁富集 trace(零完整快照)落 overflow/tie→无分区子句(防披露洪泛)。
- **NP6 rail refinement 携带 pin**:refineDomainsWithRails 后 partitionAudit 仍在(T4)。
- **NP7(可选)**:con>0 ∧ sameGroup 重叠形专项负臂(P3-3 追审记档 :493-502 点名形)。

## A.7 风险与回归面

1. **零 gate 恒等**:sameGroup 只读 fired/groupByCPU(:702-709),审计纯旁挂;NP3 逐值断言+既有 7 pin 复跑为回归网。
2. **披露洪泛**:snapshots≥1 精确门;NP5 钉死。
3. **确定性**:limits 组 map 迭代序必须排序(A.5.2);漂移 ts 取 burst 首事件。
4. **memo 并发**:审计随 `indexDerivedClusterFreqDomains` memo(:291-302)一次铸、只读共享,沿用 READ-ONLY BY CONTRACT 房规。
5. **rail refinement 携带**(T4):不携带则细分后铸点丢分区子句(与 conVetoes 既有丢弃同坑);携带需落注区分语义。
6. **射程残留(记档)**:comove_floor/single_cluster/no_domains 臂不并报分区子句(前两者分区无关或成功,后者分区未跑);未来活体出现混形再扩(零 gate,扩臂成本=一处 join)。
7. **验收门**:真板 dump zh/en 逐字节 diff(非触发板字节恒等);洁净室三门;先红证据留存。

---

# §B 件7 CENSAME-1:普查-渲染同源 + R8 兜底臂结构看护

## B.1 子件① F4:freq_only 成因上收「普查-渲染错位」

### B.1.1 F4 备案码注【逐字】(internal/tool/answer_document_mutation_runtime_tree.go:4893-4904;已与源码逐字复核)

```
// 备案 复核 F4 (CLUSTERTIE-1 dual review 2026-07-21, P3, 记档不修): the census
// reads the WIRE lane while the render face has its own precondition — a
// gated-lane row counts here on GatedCapabilitySource==freq_only, but its
// suffix actually renders only when runtimeTraceProjInversionComponents
// returns ok=true; on the components fail-open shapes (原始不可知/恒等不平)
// the row is counted yet renders no short mark. Two such rows and the head
// still declares 「本板成因…「按频率比」行同此因,行内不再复读」 over a board
// with ZERO badged rows — the promise sentence dangles. No value loss (the
// detail face keeps every cause), pure promise-face inconsistency, low
// probability. fix_direction (报告 verbatim): 普查谓词与渲染点同源(参与=会
// 实际产出 freq_only 后缀的行:supply 车道以 fold 判据、gated 车道以
// components ok 预判),或渲染期打标同 Marks 模式;补零渲染行负臂 pin。
```

### B.1.2 现状机制图

**普查侧**:`runtimeTraceProjStampFreqOnlyCauseHoist`(tree.go:4905-4942),调用点 :4204,宿主 `buildRuntimeTraceProjTreeModel`(:3005)——渲染前模型构建期。种群=四栅栏组全行(:4908);**参与判据=纯 wire token**(`SupplyFoldCapabilitySource==freq_only` :4913 ∨ `GatedCapabilitySource==freq_only` :4917);触发=`rows≥2 ∧ len(reasons)==1 ∧ reason!=""`(:4926-4934)→ 置 `FreqOnlyCauseHoistReason` 并给四组全体行盖 `FreqOnlyCauseHoisted=true`(无害:非 freq_only 行的后缀单点无视该旗,supplyfold.go:304-306)。

**树头承诺句铸点**:`runtimeTraceProjTreeFence` 内 tree.go:8909-8916(zh :8912「- 本板成因: …——「按频率比」行同此因,板级一次声明,行内不再复读」;en :8916;同点 mark 图例座 :8910/:1721)。**关键顺序事实:承诺句写在树头,先于所有行渲染进同一单 pass buffer;hoist 决策(:4204)又先于承诺句。**

**渲染点谓词**(实际产出「按频率比」短后缀的门,后缀词面单点 `runtimeTraceProjCapabilityCaliberSuffixReasonHoisted` supplyfold.go:305-327):

*supply 车道*:主渲染点=fold clause 单点 `runtimeTraceProjSupplyFoldClauseCore`(supplyfold.go:768-927,capSuffix 织入 :781);前置门=fold 判据 `runtimeTraceProjSupplyFoldVerdictFor != None`(:770;None⟺`!SupplyFoldComputed`,:496-497)。零后缀分支:UnknownBasis ∧ deficit==0 → 裸句「CPU 频率数据不全,无法折算」(:922-925,唯一无 capSuffix 的 clause 分支,tree.go:13888-13891 注释亲证【复核修正:行号 :13888-13891】)。§24② fence 压制:inversion **cause-node** 行的机制句在 fence 面整体压制(真实门=structuredOK(`runtimeTraceProjCauseNodeRow`,rcr.go:615-632)∧ mechanismSentence ∧ inversionRow,tree.go:13896),clause 只活在明细块。

*gated 车道*:唯一 fence 后缀铸点在 `runtimeTraceProjInversionComponents`(rcr.go:994-1098)running 分量内部(`GatedRunningDeficitMS>0` 门 :1023,后缀织入 :1077)。builder ok=false 门:eff≤0 ∨ PeriodicSource(:999)、total≤0(:1003)、零分量(:1087)、**打印精度恒等不平** `round3(GatedRunnableMS)+round3(GatedRunningDeficitMS) != round3(EffectiveImpactMS)`(逐分量先 round3 再求和与 round3(total) 比较,rcr.go:1090-1096;【冷读修正】不是先求和再 round3——亚毫厘边界值上两式判定不同,实施 N4 恒等不平 fixture 时须按代码式取值)。fence 消费点 :1502-1503 另有上下文门 `!handled && eligible && inversionRow`。ok=false 时 FAIL-1 镜像臂(:1528-1539)**只 mark 图例**(:1537 freq_only 图例座),行本体零后缀——图例在场而行缺词,承诺面更显悬空。

*明细/prose 面恒传 hoisted=false*(无损明细,不在承诺句射程):tree.go:15326/:15340/:18912/:19025;rcr.go:991-993 doc 亲注。

### B.1.3 分叉类逐类核实(计入普查但 fence 零后缀)

| 类 | 形 | 证据与可达性 |
|---|---|---|
| S-a | supply source==freq_only ∧ SupplyFoldComputed==false → verdict None → clause 全不渲染 | 【复核修正,载重】**产线可达路径=×N 合并清账**:`trace_causal_projection_aggregate.go:1952-1956` 无条件清 `SupplyFoldComputed`+四账值(SFD 复核 F3 裁定「display layer never mints accounting the engine did not publish」),但**不清 `SupplyFoldCapabilitySource`/`FreqOnlyReason`**(该文件仅 :452 毒臂/:467 继承两处 source 赋值)——freq_only 成员的 ×N 合并行即「census 计入而 clause 恒 None」活体。解码车道不可产此形(projection.go:3711-3712 同门置 computed=true,source⟹computed 耦合;:3716-3717 码注亲言)。**该路径使悬空形比备案「low probability」评级更常见** |
| S-b | verdict==UnknownBasis ∧ deficit==0 → 裸「无法折算」句零后缀 | supplyfold.go:922-925;可达:`Computed ∧ UnknownMS>0 ∧ DeficitMS==0 ∧ source==freq_only` |
| S-c | §24② fence 压制:inversion cause-node 行 supply 后缀不出 fence | tree.go:13885-13905;若该行 gated 车道非 freq_only 或 components 失败,行面零 freq_only 词 |
| G-a | gated components ok=false:恒等不平/PeriodicSource/total≤0/零分量 | rcr.go:999/:1003/:1094-1096。【复核修正】恒等不平真源=EffectiveImpactMS 独立通道(`runtimeTraceProjInversionGatedTotalMS` 在 gatedSum>0∧eff>0∧!PeriodicSource 时以 eff 为基准,supplyfold.go:585-591;components 语境内 rcr.go:999 已先排除 PeriodicSource)与 :17434/:17539 整组拷贝+eff 另行重导出的分叉;token first-non-empty 继承(query.go:13568-13576)本身不制造不平(disjoint 臂 :13583 total 恰由分量和导出) |
| G-b | fence components 臂不达:非 inversion 行/被更早 family 臂 handled | rcr.go:1502 |

**与备案描述的出入(精确)**:备案「原始不可知」形已被 GATED-CAL 件1② 退役(rcr.go:1045-1055,RawMS=0 改渲「原始未发布」子行、后缀照出);现役 fail 集=eff≤0/PeriodicSource/total≤0/零分量/恒等不平,再加备案未列的 G-b 与 supply 侧三形。**gap 本体确认成立且面比备案更宽、可达性比备案更强(S-a ×N 路径)。**

### B.1.4 悬空时用户看到什么 + 构造触发形

用户看到:树头「- 本板成因: 簇最高频并列,核类排序不可判——「按频率比」行同此因,板级一次声明,行内不再复读」+ 图例条目(FAIL-1 镜像臂还点亮 freq_only 图例座),而全板零行携带「按频率比」——承诺句指向不存在的行集。值零损(明细面保全因)。

- 构造形 A(supply,最简;改造 `clustertieNode` fixture,answer_document_projection_clustertie_test.go:21-34):两 on-chain running 席,`SupplyFoldComputed=true, DeficitMS=0, KnownMS=10, UnknownMS=10, source=freq_only, reason=fmax_tie(两行同)`→ UnknownBasis∧deficit==0 → 双行裸「无法折算」句;census rows=2 → 头句照发。
- 构造形 B(gated 恒等不平):两 inversion 席,`EffectiveImpactMS=5.0, GatedRunnableMS=2.0, GatedRunningDeficitMS=1.0(Σ=3.0≠5.0)、source=freq_only 同 reason`→ :1094 拒 → fence 零后缀,FAIL-1 只点图例;头句照发。

(两形均经复核官静态推演对照代码成立。)

### B.1.5 方案:A=普查谓词与渲染点同源(推荐);B=渲染期打标(否决)

**方案 B 否决,三重结构错配**:①顺序倒置——承诺句在树头先于行渲染写同一单 pass buffer,Marks「先发射后消费」前提(图例在尾部 :2496/:2633)对头部承诺句不成立;②决策自反——hoist 结果本身改变行字节(hoisted 行丢因短语 supplyfold.go:307-311),「先渲后判」需 dry-render 两遍,而 marks 是 emission 计数语义(tree.go:10155-10157 亲注),两遍渲染需快照/回滚——为 P3 显示件引入全 buffer 重组,爆炸半径不成比例;③把参与判据搬进渲染副作用流,普查从纯函数变成渲染顺序耦合。

**方案 A(推荐),取「调用真实渲染门函数」的强同源形**——新谓词 `runtimeTraceProjFreqOnlyRowWouldRenderSuffix(node, windowMS) (bool, reason)`:

- supply 臂:`source==freq_only ∧ verdict:=runtimeTraceProjSupplyFoldVerdictFor(node,windowMS) != None ∧ !(verdict==UnknownBasis ∧ SupplyFoldDeficitMS<=0) ∧ !(§24② 压制形)`。【复核修正,谓词规格】§24② 压制形的排除臂必须镜像真实 fence 门=`inversionRow ∧ runtimeTraceProjCauseNodeRow(node) ∧ verdict==Triple`(tree.go:13896 三合取;缺 cause-node-row 合取会把 ◇/▒ 非 cause-node inversion 席的 Triple 机制句行漏计——该形后缀照渲,else 臂 :13936-13937;另 `inversionRow ∧ WithDemand` 按 verdict switch 不可共存,supplyfold.go:504-508 Triple 优先,故排除臂只需 Triple)。reason=SupplyFoldCapabilityFreqOnlyReason。**fold 判据=备案 fix_direction 原词。**
- gated 臂:`source==freq_only ∧ GatedRunningDeficitMS>0 ∧ inversionRow ∧ componentsOK(node)`,componentsOK 薄封 `runtimeTraceProjInversionComponentsOK(node) bool` 固定 zh 调用真实 builder 取 ok(builder 是 node 纯函数、**无 marks 副作用**——rcr.go:994-1098 全程只构造 component 结构体,mark 由调用方打 :1520-1524,复核官亲证;语言参数不影响 ok 判定)。**禁第二判定拷贝**(同 F2 车道「no second judgment copy may grow」纪律与 capabilityFreqOnlySplitAudit 复用 freqWitnessCoMoveDiag 先例,core_capability.go:594-597)。
- 普查体只换参与判据;≥2/单因/非空触发逻辑、全行 hoisted 盖章、头句字节全不动。
- **残余近似与误差方向**:rcr.go:1502 的 `!handled && eligible` 渲染上下文门不复制(谓词多计被别臂占位的行);修正后谓词已镜像 §24② 三合取门,已知少计形关闭;任何残余错配两个方向都无害——多计→N2 输出级 pin 兜住;少计→hoist 不发、行保全因内联=fail-open 退回诚实旧形。
- **注意(复核官附带发现)**:聚合毒臂 :452 清 source 但**不清** FreqOnlyReason——若实施时把 reason 纳入谓词输入,须以 source 在场为前置,防孤儿 reason 入普查。
- **合规**:零 gate(仅措辞参与判据,值/序数/wire 零动);零新 typed signal → R2' 零义务(同构信号盘点见 B.3);渲染面改动配先红后绿 pin;判据全为精确 typed 信号。

### B.1.6 「零渲染行负臂 pin」设计

新文件建议 `internal/tool/answer_document_projection_censame_test.go`:
- **N1 触发形负臂(先红后绿)**:构造形 A、B 各一,buildRuntimeTraceProjTreeModel→runtimeTraceProjTreeFence,断言 fence **不含**「本板成因:」/「board cause:」。基线必红(hoist 现按 wire token 发)→修后绿。zh/en 双面。
- **N2 输出级承诺不变式(残余近似第二网)**:族级 helper `assertFreqOnlyPromiseConsistent(t, fence)`:fence 含「本板成因」头 ⟹ 含「按频率比」的行数 ≥2;对本测试族每 fixture 断言,构造形 A/B 修后复跑。把「census>0∧渲染行=0 不可能」直接表达在承诺面字节上,不依赖谓词内部。
- **N3 正臂字节回归**:既有五 pin 全绿零改判据(clustertie_test.go:62/:94/:114/:133/:150)+ 图例词表 pin(answer_document_mutation_runtime_revisit76_test.go:822);真 deficit fixture 在新谓词下照常参与,字节恒等。
- **N4 谓词单元臂(可选)**:恒等不平 node→false;S-b 形→false;真 fold 形→true;◇/▒ 非 cause-node Triple 形→true(【复核修正】新增,钉住少计修正)。
- **N5(建议新增)**:×N 合并形 e2e——freq_only 成员合并出 ×N 行(S-a 活体形),修后该行不计入普查、头句不发或按真渲染行数发。

### B.1.7 子件① 触点清单

| 触点 | 位置 | 动作 |
|---|---|---|
| 普查参与判据 | tree.go:4913-4920 | wire token 判据 → 共享谓词调用(model.WindowMS 在座,:15340 同源用法) |
| 新谓词+componentsOK 薄封 | supplyfold.go:305 邻域(词面单点同文件)或 tree.go:4905 邻域 | 新增 |
| 备案码注改写 | tree.go:4893-4904 | → EVOLUTION RECORD(引 §29.206 F4+本批) |
| builder doc 注 | rcr.go:991-993 | 补「census 同源消费 ok 判定」一句 |
| 新 pin 文件 | answer_document_projection_censame_test.go | N1/N2/N4/N5 |
| 旧 pin | clustertie_test.go 五枚、revisit76_test.go:822 | 全绿零改 |
| 账本 | real_trace_campaign_20260705.md | CENSAME-1 收账节 |

零 wire/schema/编解码触点。**行为变更面=仅「原本承诺句悬空/半空的板」hoist 改为不发、行保全因内联(从谎句退回诚实旧形);真渲染板字节恒等(N3)。**

## B.2 子件② CHAINGUARD P4 R8 兜底臂结构看护(最低优先记档形)

### B.2.1 现状与漂档风险

`internal/tracequery/chain_credential_census.go`:单铸点 `censusChainSeatCredential`(:97-132)七档接戳表——①OnChainBasis 双 switch(:98-103)②Causality self 双 token(:104-107)③`SubjectIsAnalysisTarget`(:108-115,R8「自身恒链上」兜底臂;头注 :22-28 亲刻该臂系实施中期被真迹测试抓获后补,§29.210:3569「网先抓了自己一个漏」)④ChainIdentityInheritance(:116-118)⑤ChainCredentialEnvelopeLevel(:119-121)⑥ChainAnchoredMs>0(:122-124)⑦Source "wakeup_chain" 前缀(:125-127)+OverlapMs∧rankFoldStartUsable(:128-130);全不中→none(:131)。种群门=`chain ∧ Rank>0 ∧ eff>0`(:162-167),铸序即普查(:134-146)。

**漂档风险历史实证**:R8 臂后补一事证明,新增一类「无 basis 席」(合法准入但不携带既有戳位)时,接戳表与 `RootCauseRankItem` 准入 typed 字段族只靠人手同步;漂档时 census 把合法席误判 none 降 ▒(过杀),或新准入车道换新字段而七档全盲(漏杀假绿)。

### B.2.2 看护方案:AST + 反射清册双 pin(零产码改动)

新文件 `internal/tracequery/chain_credential_census_structure_test.go`,复用仓内既有基建(AST 先例:perf_identity_structure_test.go:1-13 harness 与 `tracequeryProductionFunctionCalls` :19-25;反射先例:peer_chain_a1_test.go:150)。

- **Test A(读集闭包 pin)**:go/parser 解析本文件,walk `censusChainSeatCredential` 函数体,收集 `item.<Field>` 选择子集合,断言与声明清单**集合相等**:`{OnChainBasis, Causality, SubjectIsAnalysisTarget, ChainIdentityInheritance, ChainCredentialEnvelopeLevel, ChainAnchoredMs, Source, OverlapMs, StartTs, EndTs}`(现状恰十字段,复核官核实)。双向红:偷读新字段未登册红;登册字段被删读也红。
- **Test B(结构体侧准入字段清册 pin,漂档主网)**:`reflect.TypeOf(RootCauseRankItem{})`(types.go:3478)遍历字段,凡命中准入命名族——前缀 `{Chain, OnChain}` ∪ 精确名 `{Causality, SubjectIsAnalysisTarget, OverlapMs, Source}`——必须出现在测试内手册 disposition 表:`census_probe`(七档输入)/`census_output`(ChainCredentialCensus 自身,types.go:3669)/`exempt`(逐字段一行理由:ChainAnchorFullMs=值记账非准入、ChainCredentialLaneDemoted=census 后降道记录、ChainRelevance=通道词由 census 处置写、ChainAnchorRemainderSeat 等锚账簿族——字段名均实存,复核官核实)。未来新增 `ChainMembershipXYZ` 类字段未分类→红,报错文案直接命令三选一:接入接戳表(新档须同步扩闭集 enum 与 ◎ chip 词映射)/exempt 落理由/改名出族。**这正是「防新增无 basis 席绕过七档接戳表」的机械化。**
- **Test C(判决值域)**:返回值∈五常量闭集——已被 `TestChainguardCensusVerdictTiers`(chain_credential_census_test.go:31)按臂覆盖,不重复建。

既有相关 pin 不动:chain_credential_census_test.go 十二枚(:31-:478,含 PostNormalizeMintCannotEscape :354)、answer_document_projection_chainguard_test.go 六枚(:42-:342,含第二席门 :96)。

**定位声明**:本子件=§29.210 记档候办「census=target_self 无 basis 兜底形 AST 看护」的落地形(:3569 候办两项中另一项「caveat 席名集合并入形」已由 §29.218 DISPFIX-1 收账);纯测试、零 gate、零行为、零新 signal;失败面只在未来编辑期出现。落地即绿,各配一次本地突变演练(临时在 census 函数加读一字段/在 struct 加一 Chain 前缀字段)验证真咬后回滚,演练记录入收账。

## B.3 R2' 同构信号盘点(证明本批零同步义务)

本批两子件均不新增 typed signal。近期同构信号 **ChainCredentialCensus**(CHAINGUARD-1)实数同步面计 6 处:①引擎类型定义 tracequery/types.go:3669(+头注 :3657);②投影镜像字段 types/trace_causal_projection.go:626+rich-note 解码 :3519;③note-key schema/编解码 types/trace_note_keys.go:673+载体表 :1579;④渲染面(第二席门 tree.go:5043、◎ chip 词映射、tracediag);⑤文档(账本 §29.210);⑥pin(六个测试文件)。本批不动其中任何一面。

## B.4 测试计划汇总与风险

1. 先红:基线落 N1(构造形 A/B)确认红→实施谓词同源→N1 绿;先红证据留存。
2. N2 族级承诺不变式全 fixture 断言;N3 旧五 pin+图例表 pin 全绿零改判据;N5 ×N 合并形 e2e。
3. 子件② Test A/B 落地即绿+突变演练记录。
4. 回归:`go test ./internal/tool/ ./internal/tracequery/ ./internal/types/`;真板 dump zh/en 逐字节 diff(非触发板恒等)。

风险:谓词残余近似(G-b 上下文门不复制)→多计方向由 N2 兜底;componentsOK 复用 builder 已核无 marks 副作用;子件② disposition 表维护成本由红报错自解释,表过期只在编辑 RootCauseRankItem 准入族时显形,恰是设计意图。

---

# §C CLUSTERTIE F2 活体技术档案(不开工;活体出现即可开工)

## C.1 F2 备案码注【逐字】(cluster_freq_share.go:680-692;已与源码逐字复核)

```
// 备案 复核 F2 (CLUSTERTIE-1 dual review 2026-07-21, P3, 记档不修): the merge
// is blind to 快照间亚周期异值漂移 — two true clusters constant-equal in every
// FULL snapshot but each briefly jumping to DIFFERENT values BETWEEN snapshots
// (mutually >15µs apart ⇒ con=0, pro<floor) still merge, and the §29.200
// lossless argument (capRatio 恒 1) fails there: the merged fmax takes the
// strong side's excursion value and overstates the weak side. judgeBurst only
// reads full-burst values; the members' whole-trace value-set divergence is
// not a veto input. Reachability is harsh (EVERY full sweep must miss EVERY
// excursion) and the customer constant-value fleet shape (zero transitions)
// is untouched, but the direction is 假并 (违宁漏勿假) — hence filed.
// fix_direction (报告 verbatim): 分区组内 merge 前加值集同质性副证:同组成员
// 全 trace 正值集合(或 max)不等→该组不铸 merge(fail-open 与 limits 副证
// 同构);或在账本落『接受残洞』裁定注明该形。
```

## C.2 判定链机制图(见 §A.1.1/§A.1.2,共用;F2 专属补充)

见证车道语义(触发形推理所需):`freqStateChangePoints`(:1091-1100)同值重公告去重→状态变点;`freqWitnessScanPair`(:1198-1281)入场公告不铸见证(:1201-1206 切 [1:]),pro=15µs 窗内同值变迁贪心配对(:1210-1223),con=配对后双侧未匹配变迁同窗共存(:1238-1265,值必不等)。三车道优先序=代码顺序:sameEmission 快路(:470)→ con continue(:475-479)→ pro≥地板(:481-483)→ sameGroup(:485-503)。

## C.3 假并触发形刻画(**含对抗复核活体探针实证**)

### C.3.1 触发形(充分侧刻画)

设两真簇 A、B(硬件真值独立频域),以下四条件**同时成立时假并必然发生**:

1. 每个 full 快照中 A∪B 全体成员公告同一共享值 V(§28.5 毒形停放底座);
2. 快照之间各簇真变迁游移:A→Va→V、B→Vb→V 且 Va≠Vb;游移 4 条变迁时间戳两两跨簇 >15µs(否则异值同窗→con 边拦截,或回程同值同窗≥2→witness 车道自行 pro 合并);
3. 每个 full sweep 都错过每次游移(任一 sweep 逮到→组签名漂移→:784 全信号死;骑跨 sweep→该 burst partial skip);
4. 合并组 limits 正 max 去重 ≤1(否则 :822 副证扣留)。

【复核修正,载重】**上述四条件是充分侧刻画,非必要条件——真实触发域更宽**:对抗复核以可执行探针证伪了「才假并」的必要性主张——回程(→共享值 V)跨侧仅相距 10µs 时,同值跨侧回程对自成 size-2 burst 且每 CPU 恰一样本,被 judgeBurst 判为**额外 full 快照**(sig 与 refSig 同为 [0,0],实测 snapshots=3)而非杀信号;witness 车道逃逸只需同值跨窗配对 <2(单次游移结构上至多凑 1 对,pro=1<地板)。**因此条件 2 的「两两 >15µs」不是排除判据**(见 C.5 checklist 第 5 条修订),且该形还会虚增 snapshots 计数。

### C.3.2 最小合成 witness(已实跑产线代码实证)

```
sampled={0,1},cpu0=真簇A,cpu1=真簇B
V=1600000, Va=2000000(A 游移值), Vb=1800000(B 游移值)
sweep1: cpu0 @10.000000=V, cpu1 @10.000005=V   (gap 5µs 链住 → full burst, sig=[0,0])
游移A:  cpu0 @10.000300=Va, cpu0 @10.000400=V   (与前后事件 gap≥100µs → size-1 partial burst, skip)
游移B:  cpu1 @10.000600=Vb, cpu1 @10.000700=V
sweep2: cpu0 @10.001000=V, cpu1 @10.001005=V   (full burst → snapshots=2=地板)
limits: 空(或两侧同顶棚)
```

**对抗复核在 git archive 快照上实跑结果**:sameEmission=false、witness pro=0 con=0、分区 fired snapshots=2、byCPU 双双同簇(假并成立)、`coreCapabilityClusterFmax`=2000000(强侧游移值,B 侧真 fmax 1800000 被高估——capRatio 恒 1 论证失效,码注 verbatim;危害面锚 core_capability.go:790-812 成员全 trace 正 max)。**危害放大形(4 CPU)**:加恒值真簇 C={2,3}@2500000 → 实跑报 2 簇(真值 3 簇),{2,3} 恒值簇不受污染,类映射 small/big 而非 small/middle/big。

### C.3.3 客户「恒值周期全量公告形」结构性不可达三层论证

1. 条件 2 前提缺失:客户形零变迁(200+ sweep,cpu0-3=1600000/cpu4-9=2151000/cpu10-11=2500000),连一条游移样本不存在;
2. 条件 1 前提缺失:客户三组值互异,组间分离每 burst 被值互异直接证明;
3. 反事实下条件 3 极难:~1ms 公告周期 × 200+ sweep,驻留 d 的游移被单 sweep 逮住概率 ≈d/1ms,任一次逮住=组签名漂移=全信号 fail-open(死向诚实 floor split,非假并)。码注「Reachability is harsh」核实成立。

## C.4 既有防线边界(本档案最重要一问)

**边界一句话:漂移检测器的唯一输入是 full-burst 内部的值(judgeBurst 只读 events[lo:hi],:759-778);任何只存在于非 full-burst 样本中的值分歧,结构性不可见。**

已覆盖漂移类:full 快照内组签名任何变化(:782-785→整信号零值,「全信号」措辞核实准确)/丢行骑跨 burst 污染(:753/:762 skip 不证不否)/快照数不足(:800-802)/组内相异 limits 顶棚(:808-824 组级扣留)/跨簇异值变迁同 15µs 窗(con 一票否决,优先于分区臂)/组间值互异(分区只在组内铸边)。

**重要澄清**:「值可动、分组不可动」是设计特性非缺口(:655-658)——全体快照同步换值(A 组 X_k、B 组 Y_k 逐快照变动但 X_k≠Y_k)分组恒定,合并正确。

残洞输入类精确刻画:值分歧样本 (a) 从不以「破坏组签名的方式」出现在 full burst 内【复核修正:同值跨侧回程对可自成被误计的 full burst,见 C.3.1】;(b) 异值跨侧变迁不同窗(con=0);(c) 同值跨侧配对 <2(pro<地板);(d) 组 limits 顶棚去重 ≤1。机制学落点=码注「the members' whole-trace value-set divergence is not a veto input」:**分区车道全部否决输入(签名漂移/partial/地板/limits)没有一个读全 trace 值集。**

## C.5 活体识别 checklist(回访比对用)

前提:F3 披露臂上线前分区状态零披露载体,识别须回到原始 trace 与报告面症状(分区车道无专属日志行,永远不会有含「partition/分区」的内部词面工件——PARTDISC-1 落地后可直接 grep `分区车道=`)。

报告面症状:
1. 判簇数 < 硬件真值(如报 2 簇实 3 簇),capability source=default(judged 词族:核类词/按频率比激活),**不是** freq_only floor split 词;全并成 1 簇时反而出 single_cluster freq_only 词(core_capability.go:134)。
2. 弱侧簇核被按强侧游移 fmax 定价:折算「按频率比」因子对不上该核自身 cpu_frequency 最大值。
3. 无任何拆分审计行提及被并对(split_audit 只在 freq_only 判决出面,假并是 merge 非 split)。

原始 trace 计数特征(判决性,即 fix_direction 的手工版):
4. **对报告称同簇的两 CPU:按 cpu_id 分列 cpu_frequency 正值去重集——值集不等(一侧含 Va 另一侧含 Vb)= F2 决定性活体证据;值集相等则非 F2。**
5. 游移几何(支持性特征,【复核修正】**非排除判据**):异值行成对出现(去/回),驻留短于公告周期,时间戳落在 full sweep 之间。跨簇游移时间戳两两 >15µs 是典型形;但回程同值跨侧 <15µs 的单配对形**同属 F2**(且会虚增 snapshots 计数)——不得以「时间戳同窗」排除 F2。
6. full sweep 普查:若找到任何一个分组漂移的 full sweep,信号早已 fail-open 死掉,当前病理**不是** F2,另寻机制。
7. limits 车道:被并组成员 limits 正 max 去重 ≥2 则副证应已扣留,不是 F2。
8. 报告合并簇 fmax == 某一侧游移峰值 ≠ 另一侧全 trace max。

## C.6 预置修复方向(活体出现即开工)

### C.6.1 触点清单

| 触点 | 位置 | 改什么 |
|---|---|---|
| 主修 | cluster_freq_share.go:808-824 组循环 | limits 副证旁并列加**值集同质性副证**:组内每成员从 `timelines[cpu]`(:713 已是入参,零新管线)算全 trace 正值集合(或正 max);组内不全等→`continue`(组级 fail-open,与 :822-823 同构) |
| 判据文档 | cluster_freq_share.go:641-666 | 增第六条副证条目;:680-692 F2 码注改 EVOLUTION RECORD |
| 拒并因枚举 | PARTDISC-1 铸的因子面 | 增第 4 枚举值 `partition_value_set_veto`(名待批;PARTDISC-1 预留名位,见 A.5.2) |
| 账本 | real_trace_campaign_20260705.md | 活体 witness+收账节 |
| pin 族 | cluster_announce_partition_test.go | 见 C.6.3 |

**性质定性(关键)**:新副证是**精确信号**(typed int64 集合/max 相等精确判,零常数零自适应,方向 fail-open 宁漏勿假),有资格进硬门,与 limits 副证同档。**但这是判簇决策变更——CLUSTERTIE「零 gate」约束对 F2 修复不成立,开工须按值通道批纪律走(旗舰双复核+逐席 diff 追审),不得搭 PARTDISC-1/CENSAME-1 披露批便车。这正是 §29.213 把 F2 单列等活体的原因。**

集合 vs max 取舍(开工时裁定):max 相等精确瞄准危害面(fmax 高估);全值集更严,额外拦「中间游移值不同但峰值巧合」形(fmax 无损、成员身份仍假)。倾向全值集(判决性反证完整形);代价=一侧丢光全部游移行的真同簇对丢分区合并(fail-open 可接受:该对 witness 车道同样无救,退化为既有 floor split,只损失 reuse 不捏造状态)。

### C.6.2 与 PARTDISC-1 的因子集交互

PARTDISC-1 实施时因子集按可扩 typed 闭集铸、预留 value-set 名位(A.5.2),使 F2 开工只做加法不做 wire 迁移。F2 因子同步面按 A.3 同构 6 面(const/zh mapper/渲染点/因子结构体/先红后绿 pin/文档)。

### C.6.3 测试形(合成 witness fixture)

复用 `announceSweepTimelines`(cluster_announce_partition_test.go:39-52)+游移行注入 helper:
1. F2 正臂(修前红/修后绿):C.3.2 最小形→断言 `d.byCPU[0] != d.byCPU[1]`(基线上红=活体形复现 pin);**建议加第二正臂:回程同值跨侧 <15µs 单配对形**(复核探针形,snapshots 虚增路径);
2. 危害面 pin:4-CPU 放大形→修后 3 簇+各簇 fmax 各归各(2000000/1800000/2500000);
3. 客户恒值形字节恒等回归::72/:252 保持绿(值集全恒等,副证 vacuous);
4. 丢行负臂:单丢行不改值集→分区合并保留;
5. 整组同步游移负臂:A 组全员共同游移到同值再回(组内值集全等)→合并保留(真同簇游移不误伤);
6. 组级隔离臂:一组不同质、另一组同质→仅前者扣留(与 :207 同构);
7. (PARTDISC 已落地则)披露因子先红后绿 pin。

### C.6.4 替代出口

fix_direction 自带第二臂:「或在账本落『接受残洞』裁定注明该形」——若活体核验后判定客户域结构性无此形(如全舰队采集器均为恒值公告器),可裁定接受残洞收案,零码变。本档案 §C.3/§C.4 即该裁定所需全部机制学证据。

---

# §D 实施顺序、验收门与交付纪律

## D.1 顺序

1. **件6 PARTDISC-1**(§A):独立批,全部触点在 `internal/tracequery/` + 文档;因子集预留 F2 名位。
2. **件7 CENSAME-1**(§B):独立批,子件① 触点在 `internal/tool/`、子件② 纯新增测试文件——与件6 零文件重叠,可并行开发,但按 §29.213 依序推进、各自独立收账。
3. **F2 不开工**(§C):活体出现(C.5 checklist 命中,尤其第 4 条值集判据)才启动,且按值通道批纪律独立立批。

## D.2 每批验收门(checklist)

PARTDISC-1:
- [ ] 零 gate 恒等:NP3 逐值断言 + 既有 cluster_announce_partition_test.go 7 pin/cap3/caveat_lift/cluster_stream 全绿零改
- [ ] NP1 先红后绿证据留存(基线红输出存档)
- [ ] NP2「跑过被拒 vs 未跑」字节区分 + NP5 洪泛负臂
- [ ] limits 名册确定性(map 迭代序排序)
- [ ] T4 rail refinement 携带 + 注释说明与 conVetoes 不对称理由
- [ ] 词面零内部术语;`token(zh)` 文法;缺席零新字节
- [ ] 真板 dump zh/en 逐字节 diff(非触发板恒等);洁净室三门绿

CENSAME-1:
- [ ] N1 先红后绿证据留存;N2 族级承诺不变式;N3 旧 pin 零改判据;N5 ×N 形 e2e
- [ ] 谓词=真实渲染门函数复用(verdictFor/componentsOK/cause-node-row),零第二判定拷贝
- [ ] componentsOK 无 marks 副作用前提复核(rcr.go:994-1098)
- [ ] 子件② Test A/B 落地即绿 + 突变演练记录(加字段红/回滚)
- [ ] 真板字节恒等 diff;洁净室三门绿

通用:每批旗舰双复核+终局范式(git archive 不可变快照先行);账本收账节 + 备案码注改 EVOLUTION RECORD;push 前 fetch 对账、rebase 后重跑洁净室、向用户确认后推送。

## D.3 审计与复核记录(供追溯)

- 审计 workflow:3 路代码深读 + 1 路账本提取,各配锚点核对 + 对抗证伪双复核(账本路=逐字核对),共 11 agent,全部 confirmed。
- 复核实质修正已全部收编:PARTDISC E5 表述收窄、§29.195 节号勘误、cluster_stream_test.go 行程勘误;CENSAME S-a ×N 可达路径(升级 gap 定级,本文作者亲验 aggregate.go:1952-1956)、谓词 cause-node-row 合取修正、恒等不平根源归因修正、账本 :3612→:3577-3578 勘误;F2 四条件必要性证伪(触发域更宽)+ checklist 第 5 条改写、freqCoMoveSplitArmConflict 路径勘误;账本 C10 出处 :3556→:3569 勘误。
- F2 对抗复核探针:git archive 快照注入 3 探针测试实跑产线代码(仓库零触碰),实证最小 witness 假并 + fmax 高估 + 4-CPU 形报 2 簇实 3 簇。fixture 配方已固化于 C.3.2/C.6.3,可直接复刻。
- 文档冷读复核(成文后独立席):三段码注逐字零出入、60+ 锚点无一 WRONG、B.1.5 fence 门镜像与 C.3.1 回程 burst 误计两项逻辑手推 CONFIRMED、账本六处行号全对;三条局部修正(7 pin 计数、恒等不平取值形=逐分量先 round3、!PeriodicSource 合取)已收编入文,总判=可按现状交付。
---

# §E 实施进度(2026-07-24)

| 批次 | 状态 | 落地范围 | 验收 |
|---|---|---|---|
| 件6 PARTDISC-1 | **完成** | 分区拒绝审计旁路、三因子并报、rail 携带、回访说明与战役账本收账 | NP1 先红后绿；NP2/NP3/NP4/NP5/NP6 全覆盖；`internal/tracequery` 全包通过 |
| 件7 CENSAME-1 | **完成** | hoist 普查-渲染同源；零渲染承诺负臂；CHAINGUARD AST/反射看护 | N1 先红后绿；N2/N3/N4/N5 覆盖；两类结构突变均能咬住；`tool + tracequery` 全包通过 |
| CLUSTERTIE F2 | **不开工** | 保留活体档案与预留 token 名位，生产判簇零变更 | 仅当 §C.5 值集活体判据命中时独立立批 |

## E.1 件6 实施偏差核对

- 采用 §A.5 推荐方案，没有新增 wire/note/projection 字段；opaque
  `CapabilitySplitAudit` 原通路自动把新子句送达 caveat/tracediag。
- `partition_value_set_veto` 只作为 F2 后续可扩闭集名位存在，本批没有
  任何铸点，也没有值集硬门。
- limits 审计在现有 map 分组之后显式按组内最小 CPU 排序，上界按 kHz
  升序，消除新增披露字节的 map 迭代不确定性。
- rail refinement 只携带标签无关的 partition audit；conVetoes 仍按原设计
  不携带，避免细分后端点标签失义。
- 基线先红证据：`TestRunLiftsPartitionDriftRefusalCaveat` 在修复前只看到
  `判定臂=co_witness_floor`，缺少 `分区车道=partition_drift`；实现后经
  同一 Run 路径转绿。

## E.2 件7 实施偏差核对

- 普查没有另抄 fold 或 gated 公式：supply 臂直接调用
  `runtimeTraceProjSupplyFoldVerdictFor`，并复用 cause-node/inversion
  真实门；gated 臂通过无副作用薄封
  `runtimeTraceProjInversionComponentsOK` 调原 components builder。
- 为保留“一行同时携带 supply/gated 两个不同原因时必须 fail-open”的
  既有语义，实施函数返回实际会渲染的原因列表，而不是把两车道压成
  单个 reason；行数仍按行计一次，原因集合按两车道分别计入。
- 先红证据覆盖 supply UnknownBasis 零缺口与 gated 打印恒等不平两形：
  修复前 zh/en 均错误出现板头承诺，且行内零 `按频率比` 后缀；修复后
  承诺消失。真 fold 正臂与闭集词面保持通过。
- N5 直接调用生产 ×N occurrence merger，确认其清
  `SupplyFoldComputed` 而保留 freq_only source/reason 的活体形不再进入
  普查。N2 另在最终 fence 字节上钉住“有板头承诺 ⇒ 至少两行实际后缀”。
- CHAINGUARD Test A 用 AST 锁定接戳函数十字段读集；Test B 用反射要求
  `Chain* / OnChain*` 等字段逐一归类为 `census_probe`、
  `census_output` 或带理由 `exempt`。临时新增未分类 `Chain*` 字段和
  临时偷读 `ProcessComm` 两次突变均按预期红，撤回后恢复绿。
- 本批没有新增 wire/schema/note，也没有改变席值、排序、链凭证判决或
  F2 判簇。`go test ./internal/tool ./internal/tracequery -count=1`
  全部通过。
