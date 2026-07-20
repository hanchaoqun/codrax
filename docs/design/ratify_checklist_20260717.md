# 委托默认处置 · 人工追认清单(2026-07-17 盘点;2026-07-19 滚动增补)

依据:§29.104.19 用户常任委托升级(2026-07-17)——「后续需要人工审核确认的暂时按照你的评估最优方案进行默认处理,我会在合适的时候审核,不要阻塞后续批的启动。」+ §29.130 新裁定⑤委托升级(2026-07-18)——「需要裁定的部分,请先默认按照最优推荐进行实施,我后续抽空再追审核。」
账本:`docs/design/real_trace_campaign_20260705.md`(R-1…R-12 行号基线 main=4b90fd27f;R-13 起行号基线 main=67e5e1a45)。
盘点方法:全账本 grep「待人工追认/委托默认/待追认/委托」+ §29.104–§29.126 逐节精读 + `score_derivation_20260717.md` 交叉;2026-07-19 增补=§29.129–§29.137 九节逐节扫「待人工追认/委托默认/备案/裁定池」+ scratchpad 各批 spec/fixround 文件交叉(cf1_fixround/lm1_fixround/av2_fixround/elimv2_progress/ev2_fixround/ofix1_progress/of1_review/ofix2_progress/of2_fixround/cb_rework/cb_rereview/evalcase_review/lockspan_seat_investigation)。
正式项:R-1…R-12(2026-07-17)+ R-13…R-20(2026-07-19 增补,九节战役窗)+ 销项区(已追认销 1 族);附录 A=边缘备案(知悉级,A-1…A-12),附录 B=§29.104.19 之前的早窗委托裁定指针。

每项字段:① 编号+短名 ② 账本节引用 ③ 当时的问题与可选方向 ④ 已采默认处置+理由 ⑤ 推翻改动成本 ⑥ 建议。

---

## R-1 CASE3-D4 件④:合并行 eff 语义 = Σ member eff(B 根修+A 止血)

- **账本**:§29.85 CASE3-D4 收账(L2700,标记「主会话按委托默认」;2026-07-14,早于 §29.104.19 升级但同标记,未见显式追认记录,故列正式项)。
- **问题/可选**:合并(aggregate)行 eff 继承种子单成员值,「3次·折算 2.500」必被读作合计=唯一零词面披露的误导面。可选:A 词面臂 typed donor 记号 / B 引擎重铸 eff:=Σ member eff / C 专词。
- **默认处置**:B 根修+A 止血;任一成员无 eff→清零回退(宁缺勿假,禁混 cum 口径);◎ 榜序/加冕随动=诚实后果;金样复扫入工单。理由:与 §29.50.4 合计参赛同向,是唯一消除误读的根修。
- **推翻成本**:**大** —— 值语义已入引擎,榜序/加冕/金样已随动,回退需反向重放全部随动面。
- **建议**:**维持**。

## R-2 XERR1-EXT:锁席帽截 P1 = 披露道(非侧道保席)

- **账本**:§29.116 修补轮(L3015,标记「P1 处置=委托选定披露道,遵『排除≠消失』教义+§29.104.13」)。
- **问题/可选**:裁定⑤值收敛后锁席 0.141→0.066 跌出默认参数 pool 帽(#14/帽12)静默消失,同板可现「锁等待主导」注=矛盾头(既有 D1「席死于帽」病形被批准改值暴露)。可选:fail-loud 披露 caveat vs 帽外侧道保护席。
- **默认处置**:lockSeatRankFailLoudCaveat(仿 semantic 先例;精确 typed 门)发「largest X ms at pool position N of M; see critical_blocking_calls」,值/席位/帽零动。理由:遵「排除≠消失」+非致命不硬改席位分配;保席需独立裁定。侧道保席已留候选=客户复放若要席再议。
- **推翻成本**:**中** —— 保席要新设帽外席位机制,波及板面/序数/pin 族;caveat 机器可保留。
- **建议**:**维持**;客户回访若明确要席位再启侧道方案。

## R-3 RANKDIS-M18:io_pressure ⌗ caliber-side 降道不动

- **账本**:§29.117 步2+备案①(L3019/L3022:「io_pressure 刻意不入 caliber-side class 排序臂=越权防,⌗ 降道留裁定议程」)。
- **问题/可选**:裁定②只批复合分数迁出 _ms 键槽;io_pressure 是否同步做 tier/序数/fold 车道的 ⌗ 降道无裁定。可选:随批降道 vs 留裁定议程。
- **默认处置**:不动 —— 注册表 class==None 结构 pin+注释留口,突变 fatal 文案点名「需独立裁定」。理由:超出裁定②射程,降道=席位语义变更须自带裁定与突变电池。
- **推翻成本**:**中** —— 需独立小批(排序臂+fold+pin 电池),但预留口已备。
- **建议**:**可改** —— 本就等用户裁定;裁了即按预留口实施。

## R-4 TOOLWIN:h3 FAIL 归因批前病+委托默认立独立任务

- **账本**:§29.118 金样电池(L3031,标记「立案 TOOLWIN(委托默认已立独立任务)」);实施=§29.122 TOOLWIN-FIX;收编经 §29.104.20 用户批准。
- **问题/可选**:值词库批修补轮 h3 一趟 FAIL——算本批回归(阻塞收账)还是批前既有调度窗病(立独立任务放行)。
- **默认处置**:三证零交集归因批前病(分类/调度面零触+失败窗 Description 不在场+基线同形先例 h6-20260715)+复 roll PASS;立 TOOLWIN 独立任务不阻塞。理由:双 witness transcript 在案,归因确定性。
- **推翻成本**:**小** —— 归因有档可查;§29.122 带修二进制 h3 复放端到端 PASS 已反向坐实归因正确。
- **建议**:**维持**;§29.104.20 批准收编已实质追认,此项仅归因判定供知悉。

## R-5 反转词位批 C4:absorbed 行自描述 = 不加 seat_status 新字段

- **账本**:§29.119(L3035,标记原文「选型定谳(委托默认处置 §29.104.19,待人工追认)=不加 seat_status 新字段」——已知项)。
- **问题/可选**:absorbed(被吸收不占席)行需自描述其席位状态。可选:新增 seat_status wire 字段 vs 复用 Tier 枚举+邻位承诺化。
- **默认处置**:不加新字段 —— Tier 即 typed seat-status 枚举,第二字段=同一 typed 事实第二词家、违单一值源;邻位从偶然变承诺=文本面同行序+JSON 面 tier 紧邻 type 双 pin(字段夹塞即红);另加 summary 尾词「priority-inversion (runnable-overlap) candidate」+skill 一句两 token 教学。
- **推翻成本**:**中** —— 加字段=R2' 七处同步+golden+读者面,且重新引入双词家风险(主会话论证为反模式)。
- **建议**:**维持**。

## R-6 反转词位批 C5:车道常量收敛零动留裁定池

- **账本**:§29.119(L3035:「车道常量收敛按委托默认零动留裁定池」)。
- **问题/可选**:C5 置信档车道常量是否随批收敛。可选:随批动代码 vs 零动留裁定。
- **默认处置**:零代码留痕 —— 裁定③图例句已落 §29.118,常量收敛无裁定支撑不越权。
- **推翻成本**:**小** —— 纯内部收敛,裁定后随时单独小批。
- **建议**:**可改** —— 与 R-3 同性质,等裁定即做。

## R-7 SUPPREF-TOL:REJECT 后返工处置选型

- **账本**:§29.120(L3044-3049;战役窗首个 REJECT+返工闭环,复审 PASS;立案源=§29.104.13)。
- **问题/可选**:首轮合并复核 REJECT 两 P0(P0-A 新拦形=tolerant 双向 early-return 掐 ref-loop 兜底;P0-B witness 建立在分析器拒铸的伪造状态)。返工方案需选型:如何在不破权属红线前提下保宽容解析。
- **默认处置**:①positional 车道双轨(精确解析保基线逐字节;strip-retry 臂 success-only,失败一律 fall-through)+A/B 探针转正 pin;②诚实 witness 重定界=忠实产线终态 pin(今仍 DOWNGRADE)+宣称降级三分解((c) 完整拯救移交 CSP63-FIX);③逐层剥 parse-stop+chip 重铸全路径。理由:权属红线+「fixture 取引擎实铸形」的状态面升级;复审 PASS 四证。
- **推翻成本**:**大** —— 已带双不变量 pin 落地并经复审;推翻=重开 REJECT 闭环、重设计宽容解析架构。
- **建议**:**维持**(教训已入红线记忆级;REJECT-返工-复审制度首跑成功本身即价值)。

## R-8 DISPLAY-HYG 队列提序(提到 XLANE-2 之前)

- **账本**:§29.104.18.2(L3069,标记「队列调整(委托默认,用户三次直接指认显示面)」)。
- **问题/可选**:DISPLAY-HYG 原排 XLANE-2 之后;用户三次直接指认显示面。可选:维持原队列 vs 提前。
- **默认处置**:提前 —— TOOLWIN-FIX→DISPLAY-HYG→XLANE-2→HULL-CRED。理由:用户指认密度=优先级信号。
- **推翻成本**:**无** —— §29.123/§29.124/§29.125/§29.126 均已交付,时序既成事实零残留。
- **建议**:**追认即可**(仅程序性)。

## R-9 SCORE-DERIV:block_io 词条按发布值三项成文(对裁定引文的字面偏离)

- **账本**:§29.123 件3(L3078,标记「委托处置(待人工追认)」)+`docs/design/score_derivation_20260717.md:66`(Report-entry deviation 留痕)——已知项。
- **问题/可选**:§29.104.22.1 用户裁定引文的 block_io 公式带第四项「页缓存事件(加权)」,但该项只存在于内部排序分 sort_score(只影响截断成员资格,永不发布为值);发布值恰三项。可选:照抄裁定四项 vs 按发布值三项成文。
- **默认处置**:报告「阅读参考」词条按发布值三项成文;偏离在设计文档 §2 显式留痕;复核独立读码维持。理由:照抄四项=承诺面 over-claim,客户对账必不合(图例=承诺面红线)。
- **推翻成本**:**小**(改一条图例词条+pin)但方向有害 —— 改回四项即引擎面撒谎。
- **建议**:**维持(强烈)**;因属对用户裁定原文的字面偏离,单列请求显式追认。

## R-10 DISPLAY-HYG 第二轮:双单位形 wire 键暂不改,4 面挂注

- **账本**:§29.124 rider 三注释+移交清单(L3083/L3085:「双单位形 4 面挂注=wire 键裁定 M18 先例暂不改」「双单位形收敛(wire 裁定)」列移交)——已知项。
- **问题/可选**:双单位形值的 wire 键是否按 M18 先例改名收敛。可选:随批改 wire 键 vs 挂注留裁定。
- **默认处置**:4 面注释挂注、wire 键零动、收敛列移交清单待 wire 裁定。理由:wire 键变更=独立裁定射程(M18 本身即是用户单独批文的先例)。
- **推翻成本**:**中** —— 改名=R2' 七处+golden+读者 union 同步;M18 提供现成模板。
- **建议**:**可改** —— 待用户 wire 裁定,模板已备。

## R-11 XLANE-2:裁定④「互指」= 子集席单向指针形

- **账本**:§29.125(L3092,标记「委托默认(待人工追认)」)——已知项。
- **问题/可选**:裁定④词面「互指」——实现为子集席单向指针「为[E#]成员子集(整席降道)」+◎ 脚注,超集席无回指。可选:双向句对(超集席补「成员含子集席[E#…]」)。
- **默认处置**:单向指针。理由:XLANE-1「锚定份由链席[E#]代表」句族先例同形;E# 经明细块双向可解析,信息无损。
- **推翻成本**:**小** —— 账本自评「补超集席回指句=增量小改」。
- **建议**:**维持**;若坚持字面「互指」双向,低成本可改。

## R-12 HULL-CRED:裁定③落地五处实现偏离

- **账本**:§29.126 冷读官裁(L3097「偏离五条全裁」)。
- **问题/可选**:裁定③=hull keep-⛓ 逐段凭证化;落地与裁定字面五处偏离:①pid census 保守档 keep+「包络级凭证」诚实注 ②旧工件清单缺席零新词(absence never judges)③真交 keep 零新词(清单本体即凭证)④复用既有 ◇ 降道机器不造新形 ⑤逐段∩用 RSPA 原生锚窗而非 CMP-A selected_window(∅-hull soundness 只对同一窗集成立)。
- **默认处置**:五条经冷读官逐条裁为「中性/更正确」并留痕;值通道零修改,双复核零 P0/P1。
- **推翻成本**:**小-中**(逐条独立;任一翻案=局部词面或窗集改动,值面不动)。
- **建议**:**维持**。

---

# 2026-07-19 增补:§29.129–§29.137 九节战役窗(R-13…R-20 + 销项)

带子项的条目可逐子项回「R-N-x 追认/翻案」;整项回「R-N 追认」视同全部子项追认。

## R-13 CLUSTER-FIX-1 汇流勘定:毒化回执(失败判决)按代际准入缓存

- **账本**:§29.129 修补轮(L3116「失败判决按代际缓存(typed generationError,FIFO 32,open 失败蓄意不缓存;ctime 不可回拨=已亡代际判决永久正确)」);素材=scratchpad/cf1_fixround.md 件7。
- **问题/可选**:侧扫失败判决(「毒化回执」——含中途瞬态 I/O 错产生的 scan_failed)是否准入 sideScanCache 并在代际内粘滞。可选:不缓存(每查询重试)vs 按代际缓存粘滞。
- **默认处置**:typed freqSideScanGenerationError 包裹 POST-OPEN 失败(SameVersion 代际不符/frozen section/中途读错/EOF 后验)按代际 key 入 errItems(FIFO,共用条目上限 32,命中计 hits);中途瞬态 I/O 错在本进程内粘滞为诚实 scan_failed 降级,换代际自然失效;OPEN 失败(ENOENT/EACCES/EMFILE 等瞬态环境类)蓄意不缓存=单 syscall 重试便宜。理由:强身份账含 ctime(内核所有、不可回拨)→已亡代际判决永久正确;仅进程内不落盘=多实例安全;MUT-8 剥 store 即红。
- **推翻成本**:**小** —— 删 errItems lane+对应 pin(TestClusterFix1ScanFailureVerdictCachedPerGeneration),代价=瞬态错后每查询重扫。
- **建议**:**维持**(粘滞面=诚实降级非错误答案;真误伤场景=文件未变但 I/O 环境恢复,重开进程或触文件即愈)。

## R-13b CLUSTER-FIX-1 汇流勘定:毒化回执字段准入侧扫缓存(census pin 勘定)

- **账本**:§29.129(多写者语义合并节:「裁定边界 pin 勘定=毒化回执属原始扫描完整性事实准入缓存」)。
- **问题/可选**:与 CPU scalar authority 批汇流后,fullFreqCurves struct 新增毒化回执字段(freqUnsafe/limitUnsafe/freqAll/limitAll+durationOrderViolation witnesses)——「缓存只存原始扫描内容」裁定边界 census pin 因此变红,需勘定该族字段属「原始扫描内容」还是「派生结论」。
- **默认处置**:勘定=毒化回执是**采集期对原始流的完整性审计事实**(哪些 lane 有物理时间戳回退),与 droppedFreqCPUs 同性质,非派生簇结论(无 domains/fmax/class)——准入缓存;census 期望表更新+勘定注释(任何 domains/fmax/class 字段出现仍红)。
- **推翻成本**:**小**——改 census 表+缓存剥字段(但剥了会使跨 Index 复用丢完整性事实=重复审计)。
- **建议**:**维持**(完整性事实随扫描铸造,复用不改判定,剥离反而有害)。

## R-14 LEVELMERGE-1 修补轮三选型

- **账本**:§29.130 修补轮八件(L3129);素材=scratchpad/lm1_fixround.md 件1/件2⑤/件3。
- **子项与默认处置**:
  - **a PeriodicSource 聚合席出被拆种群=专用披露臂(非字节恒等)**:发布权威=VS-1 折减复合值(runnable+lateness),split 拆账会抹 lateness(19→15 探针实锤)→出种群;但重叠测度真实在手,禁静默纪律要求披露而非隐匿——选披露臂(periodic 专句,主值/eff/runnable/lateness 全保,零 A/B 行),overlap≤tol 时仍字节恒等。弃案=纯字节恒等(丢弃在手证据)。
  - **b 行2「分账构成份」臂门 FullMS>0→ConstituentSeat bool**:精确 typed bool 作门,×N 清账后浮点归零不噬句(合并 Σ 行零值自解句 zh/en);弃案=「清账时连 bool 一起清」——bool 载 ◎ census+anchorForm fork+side 路由,清掉连坐三面。
  - **c B 行重验失败 side-lane=复用 R4 ChainCredentialLaneDemoted typed 旗**:R4 凭证降道族语义精确同族(「席账目无法出示 typed 凭证→整席 ◇、值零动」),note 布线/图例词/demotedSide 路由/anchorForm fork 全现成,零 R2' 成本;弃案=remainderSeat 借旗(display 词说 RSPA 锚定二分=词面撒谎)。
- **推翻成本**:a **小**(改回恒等=删披露句);b/c **中**(改门形或新铸旗=R2'+pin 族重排)。
- **建议**:**维持**(三项均有被否弃案的书面论证,且 16 突变电池在岗)。

## R-15 AXIOM-V2 族五件

- **账本**:§29.132 偏离七条+追认项(L3149);素材=scratchpad/av2_fixround.md 件9①②③④+axiomv2_progress.md。
- **子项与默认处置**:
  - **a 53-token 归向委托默认三件**:running 族→频率与热治理(§20.2 引擎不变量=竞争 running 席 eff 恒为折算缺口,代码+四板活体双证)/gc_pause→内存(与 memory_gc 同向,防假跨向对)/missing_wakeup→未定(无边不认向);另 blocked_reason=Count 口径永不入种群仅词面(其「IO与依赖」归向词面张力→附录 A-9,ONCHAIN-ENV 点名复查)。
  - **b Self\* 豁免=Rank0 非竞争行读法**:豁免仅限 Rank0 自身非竞争行;竞争 Self 席照常入种群(E26×E5 唯一自洽读法)。
  - **c OnChainBasis 空串入闭集(第七条未申报微偏离)**:legacy 链上席(HULL-CRED 三铸造车道之前的存量席)basis 恒空串,空串出圈→守恒检查器/互指句对最常见 runnable 聚合席全瞎;表外新值仍 default 出圈(fail-open 保持)。冷读官独立冷读同段代码同结论=唯一可行读法,列偏离备案非 finding。
  - **d ∩ 值回源=partial**:互指句 OverlapMs 为派生值,两臂区间清单不整体落盘,须经 basis 侧钻取重建(读两席 member_line_ranges/self running fold 再求交);与 XLANE-2 SelfGapSemanticOverlaps 同形先例同水位(口径词+两臂行号在场=可重构),不降级;若裁定要求全重构→载体=∩ 区间清单 note(工程量另立)。
  - **e 0.018ms 相对地板裁定候选(裁定池,未实施)**:donghu 17267 E1×E8 overlap=0.018ms 经对抗官核验=真 typed 区间交集(非 float 噪声非包络假象);当前不设 de-minimis 地板(禁造任意常数红线)。若用户裁 µs 级互指句为噪声:建议**相对形**地板(overlap < min(两席 union) 或席 eff 的 x% 时降入 undisclosed type-token 道),**禁绝对 ms 常数**(tieba/donghu 窗长差一个量级,绝对常数随尺度失真)。
- **推翻成本**:a **小**(registry 单行改归向+golden 随改,但需自带语义论证);b/c **中**(改读法=种群谓词+活体 witness 重排);d **中**(全重构=新 note 载体工程);e **小**(实施地板=独立小件)。
- **建议**:a-d **维持**;e **待裁**(默认不设地板即现状,裁定池)。

## R-16 ELIM-V2 族五件

- **账本**:§29.133 偏离八条+委托默认(L3157);素材=scratchpad/elim_v2_spec.md 条5+elimv2_progress.md「委托默认(待人工追认)」区+ev2_fixround.md 件8。
- **子项与默认处置**:
  - **a 守恒尾行恒发**:「各方向支撑区间并集皆 ≤ 窗X(检查器)」pass 句恒发(pass 门=方向世代 proxy,legacy 双态不发);修补轮件7 补 pass 残余缺口=扫描扩到排除前种群(排除载体带违例→违例行发+pass 让位)。
  - **b ◇ 行 ·方向=X 转录词首批即带**(fail-open,词表外零佩戴)。
  - **c 单席节头不合并进席行**(节头「▸ 方向 · 最大可消X」保持独立行,不与唯一席行合并)。
  - **d ◇-only 板不发块头(修补轮新增委托点)**:◇ 块头「邻近(条件可消上界 · 不入方向守恒)」仅在方向节+◇ 并存时发(角色=分隔),◇-only 板不发(无可分隔之物)。
  - **e 119ch 落盘勘正**:原收账句「全行 ≤113ch」不实,勘正=member 行最宽 **119**(∩ chip +11ch 所致;member 行不受 100 结构帽约束,行为无回归)、结构行 ≤63、>120 零行;几何量测=preview ch-grid 静态计算代测,**DOM 真浏览器量测缺口保持开放**(复核官同样无真浏览器,偏离备案)。
- **推翻成本**:a-d **小**(各为独立显示面开关/词面,pin 随改);e **无**(纯记录勘正;补真浏览器量测=待环境)。
- **建议**:a-d **维持**;e **知悉**(UXR 先例「显示批必配 DOM 几何量测」的缺口在案,待有浏览器环境窗补测)。

## R-17 ONCHAIN-FIX-1 两件

- **账本**:§29.134(L3161-3163);素材=scratchpad/ofix1_progress.md 偏离备案1/3+of1_review.md P3-2。ONCHAIN 六问族本体已获用户追认(见销-1),此两件为族内工程子决策。
- **子项与默认处置**:
  - **a identityInheritance=准入时记录语义(非实时位)**:降道后不改史,发射端+显示端 gate 当前链上道;复核突变①证与「降道后逐点清位」替代四板逐字节等价=行为免费;选准入史=比逐降道点清位(≈10 处)更小爆破面,tracediag 原始面保留准入史可审计;词面永不在降道行出现(链上硬纪律满足)。
  - **b 件2 补偿臂 pass 顺序耦合=checklist 义务(不加 typed 记号)**:rankCausalThreadSet self-token 补偿臂限 Source=wakeup_chain 前缀,其宽度依赖「症状面翻转(enforceSelfSymptomRows)晚于 fallback 集消费(scheduler enrich)」的 pass 顺序;现无违反;处置=复核 P3-2 观察入 checklist(未来动 pass 顺序时必查此条),暂不加 typed 铸点来源记号收窄。
- **推翻成本**:a **小-中**(换实时清位=改 ≈10 降道点+pin,行为等价故纯语义选择);b **小**(加 typed 铸点记号=独立小件)。
- **建议**:**维持**(a 有等价性突变实证;b 若后续批触碰 pass 顺序则升级为 typed 记号)。

## R-18 ONCHAIN-FIX-2 三件

- **账本**:§29.135(L3168 件3);素材=scratchpad/ofix2_progress.md 件3+of2_fixround.md 件3/偏离2/偏离7。Q6「帽32 改已证下界」方向本体已获用户追认(见销-1),此三件为兑现中的词面/边界/量测子决策。
- **子项与默认处置**:
  - **a 「已证下界」撞词改「不小于所证」**:设计拟词「已证下界」已被 XERR1 覆盖披露行「收敛值为已证下界」占用第三语义→显示面改「(凭证清单不完整,实际锚定不小于所证,见图例)」,内部注释八处统一实词;XERR1 语义全族原句不动。
  - **b partial 前缀全不交→keepEnvelope 禁判 disjoint**:缺证≠证无——帽截前缀与锚窗全不交时不得判 disjoint 降道,退 keepEnvelope 不发布段不佩证;配 len==cap 等式+短闩非法形拒(便宜修)。
  - **c cap=32 保持+67M 未量测诚实备案**:实测 tieba max=12/donghu max=5(headroom 2.6×,零 >32),悬崖已除(超帽→前缀已证下界)使上调收益机制性归零;67M 大件独立量测受限未获,诚实备案不虚报。
- **推翻成本**:a **小**但方向有害(改回=词面家族混淆);b **小**(改判 disjoint=一臂,但违「缺证≠证无」教义);c **无**(常量;67M 量测到手后可复核)。
- **建议**:a/b **维持(强烈)**;c **维持**,67M 量测机会出现时补测复核。

## R-19 CHAIN-BUDGET 五件

- **账本**:§29.136(L3177-3179,「追认清单新增」三件为账本显式指令+扩容/归 3b 两件补录);素材=scratchpad/cb_rework.md/cb_rereview.md(P3③/新P3/P3-A/P3-D)+chainbudget_progress.md。
- **子项与默认处置**:
  - **a 帽亡真值行处置=维持披露道(三选项待裁)**:返工修根后 tieba OS_FFRT 19.984+donghu2955 四行(76.800/27.507/21.923/10.044)+keva 1.354 为**有值 on_chain 新行合法帽亡**(逐席死因表:挤出者全为真边新席 eff>0=漏计回收本身;candidate 车道从无零值行);按 R-2 既裁「席死于帽=披露道非保席」不越权保席,压缩 caveat(58→12/45→12/54→12)在岗;复审官独立背书。**三选项入池**:(i) R-2 式值披露 caveat 扩展 (ii) 背景值行帽亡侧道(需独立裁定+pin 电池,改全套板 pin 血统) (iii) MaxLimit/limit 尺度重裁。
  - **b 17267 自身 runnable 三席「两把尺」跨行恒等句=缺席备案**:base 单席 5.604 → fixed 三席 3.956[self_wall_clock]+1.193[self_wall_clock]+1.648[edge 锚]=Σ6.797;原始值三问 (a)(b) 已答,(c) 恒等 pin 缺——补「三席分账两把尺」跨行披露句=新承诺面句族(wording+幂等+pin 族,且不得宣称跨尺合计=M3 禁混尺)≠便宜修,备案待裁(复审官背书备案成立)。
  - **c 无边候选披露归 FIX-3b**:extras 递归无 sched_wakeup 真边的候选不铸 missing_wakeup(逐跳真边硬门零松动),其「存在但未披露」观察归 ONCHAIN-FIX-3b 射程(D 态同原则补全),本批不加披露词。
  - **d 扩容 MaxBranches 8→16+MaxChainNodes=96**:用户裁定③「预算过紧可适当扩大」授权下的具体尺度选择——实测需求 12/13/max 73(O10 换代注释),96 保证层最坏 16×10=160 不受此门(门只压 extras);成本真 A/B=+27.3%/+63.5%/+44.1%(独立复测吻合,预算硬界+96 过冲界实证);指纹 += max_chain_nodes(additive 不合板)。
  - **e cb_rework P3-A 注释措辞**:chain_budget_test.go P1-1 pin 注释第三对手序主张「registration-order drain」对本 fixture 不成立(节点内注册前已按 duration desc 排序,seq 序==值序);电池判别力不受损(M-D/M-E 两幻胜者真可观测);处置=留待下批顺手改注释或补跨节点注册序反例。
- **推翻成本**:a **中**(选项 ii=独立批;i/iii=小批);b **中**(新句族+pin);c **小**(3b 批内顺做);d **小**(常量+容量表 pin);e **无**(注释)。
- **建议**:a **待裁**(与 R-2 同性质,客户回访若要席再启);b **待裁**(要即立小批);c/d/e **维持**。

## R-20 EVALCASE-DH 两件

- **账本**:§29.137(L3184 LOCKSPAN-SEAT 调查结论+L3185 F4/L3186 裁定池);素材=scratchpad/lockspan_seat_investigation.md+evalcase_review.md(F4/修向裁格)。
- **子项与默认处置**:
  - **a LOCKSPAN-SEAT 修向 A/B=裁定池(现状 pin 钉死待裁)**:真机制=display top-8 时长界坐在 blocking carve 上游(µs~亚 ms ART 锁 span 被 ≥3.4ms envelope span 挤出;semantic-work-class 保留臂不含锁形)=「帽基当全量」族结构形;**证据价值倒挂**复核独立背书(PresentFence envelope conf0.72 铸席、ART 锁证词无席)+两佐证(generic 截断零披露=更彻底静默/INODE §28.6「never the top-8 slices」先例与修向 B 同向)。修向 A=最小:锁形精确信号保留臂(parseLockContentionPayload ok)+family 帽,合「精确信号才可作硬门」红线;修向 B=更根:collectBlockingSpanRows 换无界候选源(display 帽只管显示),两消费面回归半径已列。默认=现状三段 pin 钉死(tripwire:任一修向落地按设计红),不动待裁。
  - **b DH-B1/XA-R1 跳过理由牵强→小回访项立案**:DH-B1(binder ns 命名陷阱 binder:37722_6)——binder 有 4 个合成测试文件但真 fixture 容器 tgid 入名陷阱无覆盖;XA-R1(锁窗 rank top=NetworkService)——同名合成形在 process_domain_census_test.go 但真 fixture rank 席位形无 pin。冷读官裁两条跳过理由最弱,处置=维持跳过+立小回访项(补真 fixture 形 case)。
- **推翻成本**:a **中**(A=一行级臂+帽;B=候选源换轨+两消费面回归);b **小**(小回访=两 case 增补)。
- **建议**:a **待裁**(裁定池;素材/tripwire 完备,裁了即施);b **维持**(回访随后续 eval 窗)。

---

## 销项区(已追认,销)

### 销-1 ONCHAIN 六问处置族(Q1–Q6)= 用户已追认

- **账本**:§29.131 六问追认(L3139:用户 verbatim「认可 当前判定的最优方案。」)+§29.134 收账句「用户已追认族」。
- **内容**:Q1 结案零改/Q2 候选/Q3 归 CHAIN-BUDGET/Q4 归 FIX-1+2/Q5 双入口收敛/Q6 帽32 改已证下界并入 FIX-2——六开放问题处置由委托默认**升格为已追认**,ONCHAIN 族相关待追认项就此销账;Q5/Q6 的兑现批(§29.135 件2/件3)遵已追认方向落地,其兑现中的残余子决策(词面/边界/量测)单列 R-18 供追认,不影响族方向已决。
- **处置**:**销**(无需再追认;列此仅存指针)。

---

## 附录 A · 边缘备案(委托语境下的批内工程处置,未标「待追认」,知悉级)

| # | 事项 | 账本 | 处置 |
|---|------|------|------|
| A-1 | CSP63-FIX orchestrator.go:7540 共享谓词不 carve(收紧方向违权属;残口备案,未来接线须配 pin) | §29.121 修补轮 A② | 备案维持 |
| A-2 | XERR1-EXT「值切」形(锁 span 根本未入 top-8 pool)不在披露射程 | §29.116 备案④ | →DISPLAY-HYG 候选 |
| A-3 | HULL-CRED keepEnvelope 臂吸收「在场但无效」清单而图例只述缺席两成因(产线不可达论证,词不超宣称) | §29.126 备案 | 记风险不动作 |
| A-4 | 值词库批金样规格「12 趟」按 9+敏感对复趟 口径备案 | §29.118 | 备案 |
| A-5 | DISPLAY-HYG 第二轮移交清单 17 件——其中**裁定面两件(C8 标点两制/C4 折叠边词)需用户裁定** | §29.124 移交清单 | 待裁定/下轮 |
| A-6 | CLUSTER-FIX-1 件4 composite 指令原形「两 causally-compatible systrace 子件 bundle」结构不可构造(双 provenance 红线封死)→三臂替代形(可达 e2e+伪造花名册双防御臂);顺带发现并修真缺陷=超帽 union 被吞成 clock_unmappable 误标(MUT-7 红) | §29.129 修补轮 | 备案(真缺陷已修) |
| A-7 | CLUSTER-FIX-1 件1 第四枚 mid-scan swap pin 备案不做(后验窗口无生产 seam,定时竞态违确定性纪律;MUT-2 零红诚实记录;helper 由 11 调用点覆盖)+件2「导出常量」按包内具名常量落地(家风一致,要真导出待指示) | §29.129 修补轮 | 备案 |
| A-8 | CLUSTER-FIX-1 原批移交:S4 limits lane 剔核披露(后续批或永久接受待裁)/anchor-seek 未盖章形专属 pin(需大 fixture,推断覆盖已核)/**15µs skew 注释量测基修订(裁定池:相邻真实变频 14µs,值相等判据独撑)**/CLUSTER-FIX-2+CLUSTER-SIG 候选批待排 | §29.129 备案/移交 | 待裁/候选批 |
| A-9 | AXIOM-V2 词面观察:blocked_reason registry 归「IO与依赖」向与其内核语义(任意 uninterruptible 原因,含锁/内存)词面张力→ONCHAIN-ENV 批点名复查;registry 槽位=family-fold 先例(偏离②);XLANE-2 并存句词面折叠为一族句式=候选(偏离③,现状=各句独立为真蓄意并存) | §29.132 偏离②③+av2_fixround 件9④ | 留意/候选 |
| A-10 | ELIM-V2 其余偏离(R-16 五件之外):spec-wire 形不一致适配(L1 判据=显示侧 typed 包络互斥,AXIOM-V2 未发货 wire 互斥 bool)/direction_conservation_excess 升 hard_consumer(NKR 四同步)/件1b FixDirection 穿 fold/absorb 丢失=非列项根因修(三点回填)/63ch 超 60 软标/tieba 代理改形 | §29.133 偏离八条 | 备案 |
| A-11 | ONCHAIN-FIX-1 复核 P3-1 措辞义务:偏离②全称句改「进入判定的 interval-less 有凭证 pid keep 走 envelope 词;未进入判定者按准入史佩身份词」(tieba 60555 行级实证)+P3-3 IOBurstEpisodeSummary 无准入记号=构造不可达留观察(无区间 episode 现不可达;若现=诚实零 overlap 仅缺披露词) | §29.134 复核 P3 | 措辞随批勘正/留观察 |
| A-12 | EVALCASE-DH:F3 显示耦合热点备案(J3 e2e 整行 pin+五报文 token=显示批 first-touch 名单)+跳过 10 case 全备案(金样三件 spec 自标缓/两件被吸收/三件机制既有覆盖;两条牵强已升 R-20-b 小回访)+DH-IO1 退 census 锚(caller+count,delay 值 pin 违 spec 踩点④已撤,手算记录降观察注) | §29.137 F1/F3/F4 | 备案 |

## 附录 B · §29.104.19 之前的早窗委托裁定(多数已呈报或随批验收;如需一并追认按节回查)

- §29.25②+§29.26③(2026-07-10):审计处置委托「其它的按合理的方向进行发展」+ ③追认清单五项(交集口径/排版回裁/tier 退役+SemanticClass/gc_pause 第六类/B4·B6·B9·A1·A2 远程关闭)——当时已「明列供用户复核,任一项否决按 pin 反向回滚」。
- §29.38(L2178/L2182,2026-07-11):周期源 sleep 聚合维持不豁免(主会话按用户委托)+五项通道裁定处置。
- §29.39/§29.40(L2196/L2201):信息契约豁免裁决规则③+14 条待确认豁免主会话裁决(准绳=五问框架)。
- §29.43(L2235):rank-v2 开放项 O-1..O-5 按既定委托取默认(可推翻)。
- §29.43.1(L2305):折叠行词面必携两 typed 要素(按既定委托)。
- §29.67(L2565/L2567):新裁定 C 收窄形(▒ 恒 tertiary)主会话追认;完整 context 词形留 typed token 批候选。
- §29.104.13(2026-07-16):SUPPREF-TOL 立案本体+完成门权属扩展至成文阶段(用户原则重申,非委托项,列此仅作 R-7 上下文)。

---

## 追认操作建议

逐项回「R-N 追认」或「R-N 翻案:<方向>」即可(R-13 起带子项者可细到「R-N-x」);翻案项按本清单⑤的成本档排期(小=随行批,中=独立小批,大=需重开设计)。标「待裁」的子项(R-15-e/R-19-a/R-19-b/R-20-a 及 A-8 的 15µs 裁定池件)本就等裁定,裁了即施。全部维持则一句「清单整批追认」落账 §29 尾节。

## 2026-07-19 用户整批追认落账(账本 §29.150)
用户裁定 verbatim:「其它的也都按推荐的来。」= R-1..R-20 全部子决策按各条「建议」栏处置**追认生效**(R-9、R-18-a/b 强烈维持确认;R-8 程序性追认;R-4/R-16-e 知悉)。原 4 待裁子项就地裁定:R-15-e=相对形地板落地(§29.150③,INTERFLOOR-1);R-19-a=CAPFIX-1(§29.150①,链上TOP恒先+值披露);R-19-b=RULER2-1 补句(§29.150②);R-20-a=RESOLVED 修向 C(§29.147)。附录 A-5 两件:C8=分制成文消同句混用、C4=统一带树连接符形(§29.150 整批段);A-8 15µs=修订注释量测基常数不动;S4 limits=随 CLUSTER-FIX-2 补 caveat。R-21+ 未编号候选(3c basis sibling/3b 指纹闭集留 3b 开批先裁/§29.140 编队/DATAGATE-2 两点/FREQDIR-1、UPSTREAM-3 委托点)一并追认。本清单进入维护态:后续新批委托点继续滚动登记。

## R-21 UPSTREAM-3(§29.151,2026-07-19,维护态新增)
P2 待追认:件3 倒装 legacy pin——带锚显式排除(引用性边界形)在混合请求且无 explanation profile 时改交 trace-only 诚实降级答案(原强制源码分析);方向=词面不得硬门红线一致,行为翻转登记。建议维持(analyzer 误标属 prompt 歧义修辞域,另行观察)。实施偏离 1-8(function_or_purpose 扩射程/quote 臂保留/件2 转接范围/无锚否定合取/pin 翻转三件/ratchet/log 形/explorer fixture 升 typed 载体)=委托默认,建议维持。候选(不待裁):P3-a 锚否定合取、P3-b 跨 kind 洞、P3-d 同族幸存臂、tier1_floor 裸维地板、死 helper 清理。

## R-19-b 销案(2026-07-20)
RULER2-1(§29.158)落地跨行两把尺披露句,合并复核 SHIP;R-19-b 待裁态销,禁混尺红线全程保持(结构体无跨尺字段+反向禁令 pin)。

## 2026-07-20 第二轮整批追认(账本 §29.160)
用户:「其余的保持按默认推荐的来。」= §29.151-§29.159 各批委托默认点(INTERFLOOR RATIO=5%/UPSTREAM-3 P2 倒装 pin+偏离 1-8/E2PROP 批内委托/FREQDIR 词面与 rider/CAPFIX headline 设计+selfSide carve/CALSIDE 偏离 7 件/PARTSPLIT 恒等基+side-channel/RULER2 词面与族界/PROFREBASE 分型注)整批追认生效。裁定池六件=§29.160 逐裁(①链上限定撤三门/②维持/③地板+值序/④top-2/⑤EN 双面/⑥图例正交句),POOL2-1 落地。

## R-22 UPTAIL-1(§29.166,2026-07-20,维护态新增)
行为翻转独立行:ExternalOnlyCurrentVersionCheckKeepsCurrentStatus pin 分叉——bool+裸符号版本查询自此诚实降道(allowed_optional,current-status 旗清),typed anchored profile 与 file:line 目标保全;方向=已追认 P2 族同向(词面不硬门),授权链=R-21 候选 P3-d+批 spec 件3;建议维持。件1 收紧臂(preflight carve !anchor 合取)+runtime 车道 delta=委托默认,建议维持(债面不可铸证在案)。候选:CurrentSourceObligationSignal 同族精确锚 sweep。
