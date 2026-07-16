# RANKDIS-SWEEP:LLM-facing 词汇歧义五族普查报告(2026-07-16)

> 挂账:real_trace_campaign_20260705.md §29.104.16.1。witness=customlogs/cust_span_vs_prio.txt + cust_span_vs_prio_info.txt。五族并行只读审计(序数/值口径/scope/关系通道/JSON 字段名)+汇总,46 finding 去重为 27 件。

# RANKDIS-SWEEP 五族汇总报告(去重+排序+立案对照+修向编队)

## 0. 前提与核验声明

- **账本状态警示**:`docs/design/real_trace_campaign_20260705.md` 现止于 §29.110(§29.104 族至 .14.1);grep 全仓 **"RANKDIS"/"29.104.16"/"29.104.15" 零命中**——派单所引 §29.104.15 ELIM-GAP / §29.104.16 RANKDIS 两节尚未成文(在飞未落盘,同 LOCKNS-FIX 形)。下文"已覆盖"判定按派单口径(RANKDIS=rank 键三族:root_cause_rank 双通道/state_drilldown/auto-window);**实施批开工前须先补写两节,否则覆盖判定悬空**。
- **抽查核验**:本汇总只读复核 12+ 关键发射点(types.go:3088/2090、trace_query.go:877/3595/6602/161、stream_window_sweep.go:108、query.go:15620-15632/12144-12150/18957、rcr.go:1346-1356/868-882、tree.go:13008-13020、rank_cross_type_recon.go:270-282),**全部与族报一致,零虚构**。
- 五族原始 finding 共 46 条,去重合并后 **27 件**;跨族重复 8 组(见 §1 合并注)。

## 1. 去重合并总表(按误导等级排序)

### T1 — witness 实锤(客户模型已被误导),13 件

| # | 合并件 | 词面 | 合并来源(族#finding) | 关键发射点 | 立案对照 | 编队 |
|---|---|---|---|---|---|---|
| M1 | 裸 rank 键多板复用 | `"rank"` 三结构共键+双通道 rank=1 撞号 | ordinal#2+#3+#4 ⊕ wire#1(4→1) | types.go:3088/2090;tool/trace_query.go:877/3595/5013;query.go:17326-17329 | **RANKDIS 已覆盖,需扩案补漏发射点**:engine Summary query.go:7302、JSON types.go:2090、caveat :1082、清单行 :3199、标题 :3215、AbsorbedItems 混排 :3563-3565 | A1 |
| M2 | wakeup_chain:path#N 序数字形 | ClaimKey `path#N` vs 板席 #N 同形 | ordinal#1 | tool/trace_query.go:6602;handoff :204;projection :105 | **RANKDIS 需扩案**(三族未含;双 Rank #1 witness 的另一半根源) | A2 |
| M3 | gated 合成值冒充族(四面一根) | 「全额」假盖/窗口投影列违图例/裸「有效归因X」/◎ 裸 runnable | value#1+#5+#6+#9(4→1,同根 query.go 覆写) | query.go:15628-15632、12144-12153(覆写根);rcr.go:1346-1353(退化臂)、:870-880(fail-open)、:1224;tree.go:11515、11757-11760;elim.go:384-392、405-430;runtime.go:1234 | **新立**(建议案名 GATED-CAL;RANKDIS/ELIM-GAP/HEADLINE-ELIM 均不覆盖;类词臂=INV-SUPPLY §29.61.11 推广) | B1 |
| M4 | 「链上累计」用在自身线程头条 | 同线程 8 段合计戴跨线程累计词→模型虚构跨线程 credential host | value#3 | tree.go:13015-13020(已核验:`CumulativeImpactMS>0` 即戴词) | **新立**(与 §29.110 相邻不重合:彼=披露附录,此=系统头条词面失实) | B2 |
| M5 | 值词库分裂/教学陈旧簇(4 子件) | ①wire↔显示词无映射(gated_runnable/sum_disjoint 进客户正文);②eff↔cum 四名一值+关系缺失(模型自铸「直达」);③Description教学 hidden-cost 旧语义;④projected_total_ms 幻键(Description 教的键 rank 行 JSON 不存在,已核验 :161 原文) | value#2 ⊕ value#4 ⊕ wire#2 ⊕ wire#8 | defaults.go:179;trace_note_keys.go:829-836;runtime.go:1230/1235-1236;tree.go:11372;tool :3527/:161/:7248-7249;observation_ledger.go:3153 | **新立**(值词库桥接族);witness 注:transcript 说 interval_union 而报告 fold=sum_disjoint,wire 族判模型口误、value 族判词面燃料——两读法均由 C1 对照教学修复 | C1+C2+C3 |
| M6 | trace_mark_category 总账 scope | count=14 total=13.247ms 跨线程全窗聚合 + `thread=` 实为 top_thread → 被当板#1 席确证,五轮调和 | scope#1 | query.go:18957(已核验 `thread=%s` 无 scope 词) | **新立**(独立 scope 词件) | B3 |
| M7 | 同席双 token 静默混读 | priority_inversion_candidate / priority_inversion_runnable_wait;absorbed 行 grep '"type":' 看不见 tier(已核验 :273-279 tier 与 type 分离)+行内 summary 尾词跨族 | relation#1 ⊕ relation#8(2→1) | rank_cross_type_recon.go:273-279;query.go:15621/22960/22972 | **新立** | C4 |
| M8 | 反转 token 显示词三家分叉+词位倒挂 | 一 token 三词(调度压力候选/优先级反转·可运行等待/runnable调度候选)+表 cell 词分裂+◎ 总览强席显弱词 | relation#2+#3+#4(3→1,同 token 词位族) | rcr.go:327-329;typelabels.go:41-42;runtime.go:3565-3569/3801/4141-4148;tree.go:9837-9839/9853 | **INV-SUPPLY §29.61.11 扩展**(总览臂)+新立(其余两面) | A5 |
| M9 | 「深度未解析」对 L1 行撒谎 | 下一步 prose vs 树 chip「链上L1(父节点未确认)」同报告互斥 | relation#5 | runtime.go:6587/6518-6533;C6 单源 tree.go:8898-8910 | **新立**(C6 §24.12 词族第四面补齐) | B8 |
| M10 | 置信档词=车道常量折词 | 板#1 戴「置信中」板#2 戴「置信高」,反向支持模型推翻板序 | relation#6 | tree.go:16611-16631;query.go:15623-15626/15719-15722 | **新立**(HEADLINE-ELIM 同误读链但未覆盖此词面) | C5 |
| M11 | state_drilldown total=/impact= 随 source 换 population | 同列名两种账+与树面同线程同态账无桥 | scope#4 | query.go:7110-7156;tool :5013 | **RANKDIS rider**(同发射行 rank= 已立案) | B6 |
| M12 | 供给缺口 1.911ms 跨席误绑 | 值-主体绑定漂移(binder:500_1B→shadowhook) | value#8 | 模型散文;elim.go:604 邻接诱导 | **已覆盖=CR-4 在飞批**(memory: project_cr_smr_20260712)→ 喂验收样本,不新立 | 旁路 |
| M13 | 跨席 Σ 有效归因 24.3ms | 混入供给席+未证不重叠即求和 | value#11 | 模型散文;承诺面 tree.go:1468 | **ELIM-GAP §29.104.15 归族核对**:成文时若范围不含「有效归因跨席求和」则作 rider 增补,不另立 | 旁路 |

第 13 件:M14 relation=/priority_relation= 同 token 两把 key(relation#7 ⊕ wire#5,2→1;witness 弱实锤=词面混但语义读对;tool :8591/:3499 vs :3527、query.go:21396)——**新立小件**,编队 A4。

### T2 — 高概率(结构通道存在/历史同形实锤),5 件

| # | 合并件 | 来源 | 关键发射点 | 立案对照 | 编队 |
|---|---|---|---|---|---|
| M15 | TraceNoteKeyRank typed 键借用(状态板序走因果 rank 车道,空 chain_relevance 默认 chain 通道) | ordinal#6 | tool :9155;trace_note_keys.go:113/943;trace_causal_projection.go:2902;rcr.go:622-623 | **新立**(RANKDIS 词面案的 typed 车道姊妹;现防线=prompt 去重巧合遮蔽,非设计) | A3 |
| M16 | 树头覆盖句分母双账(31.968 已发布行切片 vs 44.298 全窗等待族,差 28%,witness 面内无桥接词) | scope#2 | tree.go:13529/13856-13914;legend :1511/:1520 | **新立** | B5件 |
| M17 | episode-scope 缺词族 raw Summary(wakeup totals + peer_state 窗覆写无窗基词;F5-1 tieba 历史实锤同形,只修了显示面) | scope#3 ⊕ scope#7(2→1) | query.go:12575/21393/19411 | **新立**(F5-1 源头推广) | B4 |
| M18 | 复合分数进 _ms 键族(io_pressure Score/block_io 复合/rank_impact_ms;§7.30 S1 历史实锤只关文本面;代码注释自认「留裁」) | value#7 ⊕ wire#3 ⊕ wire#4(3→1) | query.go:13374/13405-13410;types.go:2095-2102(已核验注释);explorer.go:3538-3539 教学 | **留裁项补裁→裁定议程**(wire 键改名需用户裁定) | A7 |
| M19 | 「满格」两面两词(本区TOP1 vs 全区最大值) | value#10 | elim.go:721/699-700 vs tree.go:1468 | **新立**(纯词面) | B9 |

### T3 — 卫生,9 件

| # | 件 | 来源 | 对照/编队 |
|---|---|---|---|
| M20 | hotspot `json:"rank"` 第四族(文本有 hotspot 前缀,JSON 裸键) | ordinal#5 ⊕ wire#9(2→1) | **RANKDIS census 扩案** → A1 |
| M21 | Description「top-ranked state」教学裸词(已核验 :161 原文在场) | ordinal#7 | RANKDIS rider → C3 |
| M22 | 「见榜位#N」ZH 指针词未入 tracefence 单源 | ordinal#8 | UXG-1 M1 漏网 → A6 |
| M23 | observation ID 尾数 #root_cause_rank:N=position 非 rank,两 lane 铸法不一 | wire#6 | 新立小件 → C8 |
| M24 | window= 四义(行段/查询窗/首末/子窗) | wire#7 | XLANE-3 §29.109 部分覆盖,残余 scope 词 → B11 |
| M25 | family/families 缺 same-thread 限定 | scope#5 | 一词修 → B7 |
| M26 | 「种子成员账」括号指代 garden-path | scope#6 | 新立小件 → B10 |
| M27 | 三通道词汇三套无对照(链上↔on_wakeup_chain↔channel=chain;self_deterministic 无 zh 桥) | relation#9 | 教学件 → C6 |
| M28 | 三口径词无图例定义+发生段回显同值无注记 | relation#10 | 教学件 → C7 |

**不立案**:ordinal#9(frame item #N,前缀已 scope、无 witness,触碰顺手改)。

## 2. 立案对照汇总

- **已覆盖(3)**:M1 主体(RANKDIS 三族)、M12(CR-4 在飞,喂样)、M24 部分(XLANE-3)。
- **需扩案(5)**:M1 补漏发射点、M2 path#N、M20 hotspot 第四族、M11/M21 rider、M8 总览臂(INV-SUPPLY §29.61.11 推广)。
- **归族核对(1)**:M13 → ELIM-GAP §29.104.15 成文时核对范围。
- **新立(其余 18)**:核心新立案建议两个——**GATED-CAL**(M3+M4+M5 值口径冒充/词库分裂,witness 最重)与 **RANKDIS-EXT**(M1 扩案+M2+M15+M20 全并一案);小件挂卫生批(可并入队列中 DISPLAY-HYG)。

## 3. 修向编队(可直接派实施批)

> 影响面轴释义:**golden**=SEAL-1 金样 h1..h9 端到端渲染金样;**B5**=real_trace_b5 等渲染 golden fixture 家族(显示 bytes pin);**R2'**=typed signal 六/七处同步(新增/改名 note 键与 wire 字段必走)。

### 编队 A — 源头改名族(词汇分叉/裸键,「噪音从源头消除」)

| 件 | 发射点 | 改法 | 影响面 |
|---|---|---|---|
| A1(M1+M20,RANKDIS-EXT 主件) | types.go:2090;tool :5013/:877/:1082/:3199/:3595;query.go:7302/17326-17329;stream_window_sweep.go:108+tool :3354 | drilldown `rank`→`drill_rank`(JSON+文本+engine Summary 三面同改);auto-window 键+caveat+清单行 scope 化(:3215 标题低风险可留);hotspot→`density_rank`;root_cause_rank 双通道行内加 `rank_channel=chain\|adjacent`(或链/邻分键);window_discovery.go:149 零-LLM 面可豁免但建议跟改 | golden+B5 必重滚(blob 键与行形变);**parser 同步**:runtimeTraceMetricSnapshotCoveredByAnswer 精确 key=value 解析;note 键改名走 R2' 七处 |
| A2(M2) | tool :6602 ClaimKey;Description :161 | `wakeup_chain:path#N`→`wakeup_chain:branch=N`(与 RichNotes branch= 同词);Description 补负向教学「branch 编号非排名」 | ClaimKey=observation 去重键,查 fixture pin;R2' 不涉(branch= 已注册);prompt 红线 checklist |
| A3(M15) | tool :9155;trace_note_keys.go:113/943;trace_causal_projection.go:2902;rcr.go:622-623 | state_drilldown observation 换专用 `state_rank` 键+注册表登记;投影编译按 Predicate 门控 rank 键解析 | **R2' 七处全走**;golden 低(prompt 面现被去重遮蔽) |
| A4(M14) | tool :8591/:3499;query.go:21396 | 边文本面 `relation=`→`priority_relation=` 全族同 key | 文本 fixture;低风险 |
| A5(M8) | rcr.go:327-329;runtime.go:3565-3569/3801/4141-4148;tree.go:9837-9839/9853 | ImpactFormTokenFamily 增 priority_inversion_runnable_wait typed 臂;删「runnable调度候选」第三词改引 typelabels;表 cell node-aware(flag 行同词);总览词位=行2 族词(§29.61.11 推广) | golden+B5 大面积重滚(显示 bytes);registry 单源自查 |
| A6(M22) | tree.go:3674 | ZH 指针并 tracefence 单源(「见根因排序#N」或增别名常量+图例点名) | golden(树 bytes) |
| A7(M18,**先裁后施**) | query.go:13374/13405-13410;types.go:2095-2102 | 复合分数出 _ms 键(rank_score/rank_weight_composite)或行内 caliber_side 兄弟字段(registry :424-441 已有权威);explorer.go:3538/evaluator :7336 教学句同批改 | wire 键改名=blob 消费面+golden;走裁定池 |

### 编队 B — scope/口径词补齐族

| 件 | 发射点 | 改法 | 影响面 |
|---|---|---|---|
| B1(M3,GATED-CAL 主件) | rcr.go:1346-1353/870-880/1224;query.go:15628-15632/12144-12153;tree.go:11515/11757-11760;elim.go:384-392/405-430;runtime.go:1234 | ①退化臂加精确 typed 门 `GatedRunningDeficitMS>0` 禁盖「全额」;②行3 构成式放宽(只需分量计入值);③席行窗口投影分通道发布或单元格口径注记+图例反转席限定;④裸 tag 最小口径词+C5 守卫前提改 ConsumedEffective 实际+unbalanced 臂保底;⑤◎ 注记臂换精确门+类词臂推广 | golden+B5 多面重滚;全部精确 typed 门(precise-signals 红线合规);新 note 字段走 R2' |
| B2(M4) | tree.go:13015-13020 | self/semantic 席头条改发有效归因+族口径词「合计(共N段,同线程)」;链上累计只留真下钻链;最小修=追加「(同线程,非跨线程累计)」 | headline 进 Known Facts(跨会话面);golden |
| B3(M6) | query.go:18957 | summary 补 `threads=N` + window-wide-across-all-threads scope 词;`thread=`→`top_thread=`(与 JSON key 名实对齐) | Summary 进 blob+observation;查 summary 消费/解析面 |
| B4(M17) | query.go:12575/21393/19411 | totals 前插 episode-scoped 限定(或 scope=chain_episode 字段);peer_state 内联窗基 | **必须同步** runtimeTraceMetricSnapshotCoveredByAnswer parser |
| B5件(M16) | tree.go:13529(姊妹臂 :13493/:13546/:13555);legend :1511/:1520 | 四态账在场且差超容差时覆盖句内联双基;legend 补分母 population 定义 | golden(树头 bytes);容差常量禁跨语义借用(既有教训) |
| B6(M11) | query.go:7110-7156;tool :5013 | per-source scope 后缀(state_total_in_window / churn_observed_span_total)或 source→口径映射教学句 | RANKDIS rider 同批 |
| B7(M25) | query.go:8401 | 「across %d same-thread (thread,semantic-class) famil(ies)」与 :15205 对齐 | 一词修 |
| B8(M9) | runtime.go:6518-6533/6587 | Depth>0 fork 引 C6 单源词「链上L#(父节点未确认)」;深度未解析只留 Depth<=0 臂 | golden(下一步 section) |
| B9(M19) | elim.go:721 | 改与 tree.go:1468 同词「满格=全区最大值」 | 零行为,bytes 变→golden |
| B10(M26) | tree.go:5423/5425 | 主语点名消歧(本行值=族合计;种子成员账不参与本行合计),zh/EN 双面 | golden |
| B11(M24) | tool :3595-3596/:3613-3614 | 行段义 `row_window=`、首末义 `first_last=` scope 词 | 随 A1 批顺手;parser 同步 |

### 编队 C — 自描述/教学族

| 件 | 发射点 | 改法 | 影响面 |
|---|---|---|---|
| C1(M5①) | defaults.go:179 | 教学改教显示词模板(gated_runnable_ms=行3 runnable(全额)分量;fold=sum_disjoint=合计(共N段,同线程));zh 规范译词「有效归因」单点化 | **prompt 红线 checklist(ATOMIC 7 条)BLOCKING**;eval 回归 |
| C2(M5②) | runtime.go:1235-1236;tree 图例 | 图例补 eff≤cum 关系子句+L1 无链「累计」语义句+causal_impact `impact=` 与显示词挂钩 | golden |
| C3(M5③④+M21) | tool :161 | Description 重写三处:effective_impact_ms 三口径现实(删 hidden-cost 旧语义);projected_total_ms 幻键修正(停发别名或补真字段,择一);「top-ranked state」→state_drilldown scope 形 | **Description 每轮进上下文=最大教学面**,单独批+金样 12 趟+prompt 红线 checklist |
| C4(M7) | rank_cross_type_recon.go:273-279;query.go:22972;skill | absorbed 行 type 旁共置 seat_status=absorbed(或 type_family 共族词);summary 尾词自洽「priority-inversion (runnable-overlap) candidate」;skill 补两 token=一族两通道教学句 | wire 加字段→R2';golden |
| C5(M10) | tree 图例(或 query.go:15623-15626/15719-15722) | 至少图例补「置信档=数值阈值折词,非跨行可比证据强度」;车道常量收敛属行为变更→裁定池 | 图例=golden;常量收敛须裁定 |
| C6(M27) | defaults.go:728 或 tree 图例 | 三词对照一行(链上=on_wakeup_chain=channel:chain;…;自身·确定性优化=self_deterministic) | prompt checklist |
| C7(M28) | tree.go:1663;smr1 面 | 三口径词图例定义一句+发生段回显值加「(席位账,见[E#])」 | golden |
| C8(M23) | tool :6496-6501/:9139;observation_ledger.go:2533 | 段名改 #root_cause_row:N 或两 lane 统一铸法;至少教学声明 position 非 rank | ID 形变查 fixture |

### 建议批次切分(可对接现有队列)

1. **RANKDIS-EXT 实施批** = A1+A2+A3+B6+B11+C8(rank 族一次收口;先补写 §29.104.16 成文)
2. **GATED-CAL 批** = B1+B2(witness 最重的值面冒充;A7 分离走裁定池)
3. **值词库教学批** = C1+C2+C3(Description/skill 大改,单独金样 12 趟)
4. **反转词位单源批** = A5+C4+C5(INV-SUPPLY 扩展收尾)
5. **scope/卫生批** = B3+B4+B5件+B7+B8+B9+B10+A4+A6+C6+C7(候选并入队列中 DISPLAY-HYG)
6. **旁路两件**:M12 witness 喂 CR-4 验收(1.911↔binder:500_1B);M13 待 ELIM-GAP §29.104.15 成文核对归族。

## 4. 五族 clean_faces 合并清单(去重后,防覆盖缺席)

**序数面(ordinal)**:top_running/top_sleep 行(:3684/:3691 零序数)、top_io_inodes(:4826/:4844)、interaction_stats(:3613)、scheduler_latency(:3652)、state_churn 行、wakeup path 文本行(:3483/:3486,序数只在 ClaimKey=M2)、causal_impact depth=(:3527)、aggregated_impact occurrences(:3539)、rank_perf_context/rank_blocking_detail 同板回指(:4393/:4438)、**background_rank 独立键=修向正面样板**(types.go:3102)、席位 chip tracefence 单源(display_tables.go:194-197)、E# vs #N 教学(tree.go:1724)、evidence pack 席位词(query.go:22579-22588)、EN「see root-cause rank #N」单源(tree.go:3676)、明细审计图例(:3192)、list/recall_memory #N(无共现)、微锚折叠 #N~#M(:15732)、tracediag 零-LLM 面。

**值口径面(value-caliber)**:「实际状态」列注记族(超出发生段/窗内/单次成员)、指标快照 state_churn 双块三重披露、确定性优化点表(占窗%+E26 同源)、fence 头「满格=窗口151.382ms」+%列、跨线程聚合「N线程取最大」单元格标注、供给折算方向词(E5/E19/E20 逐行)、周期性信号源图例(runtime.go:1353)、头四行散文基准+防双计、E8/E9・E21/E24・E4/E7 双账关系注(覆盖集不同/物理重叠不可相加,双向互指)。

**scope 面**:rank_family_fold 三形 scope 词(:638/:643/:646/:649+:653)、semanticTraceSpanProjectionScope 四臂(query.go:16236-16254)、:15205 same-thread caveat、全窗态 vs 链上覆盖词梯(runtime_tree.go:16998-17013)、state_churn 显示面(runtime.go:5704-5772,F5-1 修后)、四态账行「全窗」+自检等式(:13217)、running 拆解恒等式(:13245-13313)、关键指标表列图例、同源二分三臂等式(:5364/:5398/:5409)、elim 脚注(:1153/:1227)、cap 下界 caveat(:15225)、邻近席双向指针(E31-E37 全带)。

**relation 面**:causal_token_registry 单源本身无分叉(:156-157/370——分叉全在显示消费端=M8)、C6 词族三面自洽(tree.go:8895-8912+图例,M9 是第四面漏网)、树边词全图例(:1165-1191/1539/9799-9810)、关系明细 cell 词表(:16466-16526 无 raw token 泄漏)、D4 叙事复合形(typelabels.go:510-531)、供给缺口主导复合词单源(supplyfold.go:286-303)、账目关系伞形图例(:1663+5593)、置信档函数单实现三面共用(M10 是数值↔档词桥缺席非面间漂移)、board 摘要通道教学(trace_board_summary.go:108-151)、skill/evaluator 反转权威教学(defaults.go:179/932)、InversionCandidateWordZH 字节单源(display_tables.go:217)、合并口径词族三面一致。

**wire 面**:客户 witness 三 token 全真非幻觉(gated_runnable/priority_inversion_candidate=true/relation=lower_priority_waker,模型 gated 组成读义正确)、blob 头自描述(view/source_path,tool :355/:461-465)、fold 枚举 wire 键教学成对(member_fold_caliber 族)、WakeupEdgeCensusPair 计数键精确、background_rank/absorbed_by_rank_family/absorbed_into 自描述族(M7 残余=type 行共置)、XLANE-3 板身份三元组已上 wire+note(trace_causal_projection.go:719-737;tool :6318-6331)、复合分数行 observation Unit typed 口径(§29.96.2,tool :5879-5885)、target_mask 专门教学、有效归因键-词图例成对、window_discovery rank 面确认零-LLM(tracediag/render_v2.go:77)。

**覆盖边界声明(合并)**:ordinal 族逐点读过全部序数发射点,未读面=非序数语义族;value 族只审值口径词(rank 词面归 RANKDIS);scope 族确认 witness 中 rank 混淆归属 RANKDIS 未重复立案;wire 族确认 tracediag 面免除;五族边界拼合后无已知未审面残留于本题范围(rank/值口径/scope/关系通道/wire 字段五轴)。