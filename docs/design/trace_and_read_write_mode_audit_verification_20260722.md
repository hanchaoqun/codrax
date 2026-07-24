# 「Trace 分析规则与读写模式系统缺口审计」核验报告(逐条判定与纠正)

> 核验对象:`trace_and_read_write_mode_current_implementation_audit_20260722.md`(基线 `main` @ `ca644b94b`,与核验时 HEAD 一致)
>
> 核验方法:七域并行行级核验(输入/唤醒链、effective/cluster/周期、排序/凭证/守恒、因果投影、五区面板/P3、read mode、write mode),每条可核声明对代码 file:line 亲证,每条批评/建议对裁定账本(`docs/design/real_trace_campaign_20260705.md`、`customer_dead_session_audit_20260703.md` §7.4/§7.5、CLAUDE.md 红线)双向核账。凭据未亲证不判 accurate(宁漏勿假)。
>
> 判定词:**accurate**(准确)/ **partial**(部分准确,附纠正)/ **inaccurate**(失实,附纠正);处置词:**既裁维持**(触既裁设计,重开需显式推翻对应裁定)/ **已候办**(同款限度已立案)/ **确认真 gap**(值得立案)/ **非 gap**(蓄意设计或指控不成立)/ **需裁定**(产品方向,须用户裁定)。

## 0. 总判

1. **描述面质量高**:全文百余条机械描述中约 85% 逐行核实准确——状态全集、唤醒链六条件(除条件3/4)、guaranteed/extra 预算、edge census、周期公式、能力系数表、supply fold 公式、effective 双闭表、凭证普查五值、排序五级、容量表全部数字、投影 fold 十三步、五区面板结构、P3 十条、write controller 状态机与审批机械,均与代码逐点吻合。
2. **判定面系统性问题**:G1-G13 沿用了前次审计(20260721)的编号,但**未对 §29.183(该次审计的逐条定谳)双向核账**——G1/G2/G3/G4/G5/G6/G7/G9/G10/G13 十条在 §29.183 已逐条定谳(多数「不修+禁重诉区」,部分残口已在 §29.185-§29.188 落地交付),本版继续按开放 gap 列报;其中两处**重复了 §29.183 勘误清单已回传的同款描述错误**(§5.2 条件3/4、§6.5 分母)。
3. **真金**:读写模式域是本审计的实质贡献。**RW1(write 续跑无仓库身份门)全案实证,是本审计最有价值的确认 gap**;RW2/RW3(只读 shell 语义洞)在「护栏合同」层面成立;RW7(持久化静默降级)成立;RW8/RW10/G17 三条文档漂移成立。
4. **需用户裁定一项**:P0-1 的「OS sandbox/移除 awk」是从 LLM 失误护栏升级为对抗性安全边界的产品定位变更,且与仓内成文 skill 指引冲突,不应默认 P0。

---

## 1. 描述面纠正表(仅列 partial/inaccurate 条;未列出的小节 = 逐行核实准确)

| 原文位置 | 判定 | 纠正 |
|---|---|---|
| §4.3 dominant「仍相同则较早出现者优先」 | partial | dominant 判定=lane 时长严格大于才换主,平局按固定 lane 序 `io_wait>d_sleep>runnable>s_sleep>running` 一次性定序(每状态恰一 lane,序后不可能再平,无「较早者」臂);「时长与状态分全等时较早段优先」属**目标分支候选排序**(`query.go:24390-24399` stable sort),不属 dominant 判定(`thread_state_universe.go:127-151`) |
| §5.2 条件3/4「铸边优先 sched_wakeup,候选面无 wakeup 才回退 waking」 | **inaccurate** | 铸边车道对 `sched_wakeup` 与 `sched_waking` **同池等权受理**(仅排除 `sched_wakeup_new`),在 `[sleep.start, sleep.end+5µs]` 内取 `(Ts,Line)` 最新一条,无类型优先级(`query.go:24434/24451-24455/24615`);「wakeup 全无才回退 waking」是 **wakeup edge census 的单尺来源规则**(`wakeup_edge_census.go:244-252`),不是铸边条件。⚠ 此为 §29.183 勘误清单中前次审计唯一被判错并已回传的条目,本版原样重复 |
| §6.5「至少 2/3 gap 在约 15% band 内」 | partial | 分母不是全部 gap:整数倍 observation gap 先被 carve 剔除,**carve 后剩余 interval** 至少 2/3 落入 `[p×0.85, p×1.15]`(`query.go:13742-13748`)。⚠ 同为 §29.183 勘误清单已回传项 |
| §7.6「off-chain semantic/background 无普通 ordinal」 | partial | background 通道结构性无序数(§29.36.2 通道3);但 off-chain semantic 行固定 tertiary tier 后仍走 takeOrdinal——typed relevance=adjacent 的 semantic 行(如边锚 ◇ 余段席)**持邻近通道序数**,仅 relevance=background 时无序数(`query.go:19433-19442/19310-19314`) |
| §7.7「达到 5% 标『待追认』」 | partial | ≥5% 即正常互指披露(双侧 roster 句/∩ chip;载体缺失或 roster 满时降 type-token caveat),**不存在「待追认」标记**;<5% 时句面静默、双侧保留 typed undisclosed token,值/席/序数零动(`rank_direction_axiom.go:433-519`)。「待追认」源自已按 §29.183 G13 修掉的陈旧注释(AUDITFIX-A) |
| §8.5 V4「要求同 subject、object、type 和真实 object identity」 | partial | 恒需:同 canonical subject+object、行或时间范围相交、非同 EvidenceID;type token 需相等但有 WO-D3 豁免臂(单侧 token 缺席+sentinel object+精确同值)。**真实(非 sentinel)双侧身份仅是 near 档(≤3%)附加门**;exact 档 sentinel object 也 fold 且值字节不动;改值仅 near 档取 max(`trace_causal_projection_aggregate.go:1034-1092/1110-1118/1148-1166`) |
| §8.6 规则1/4(R2 通则) | partial | ≥3 门与「同窗默认 SUM」是通则,但**漏述本基线已落地的 §29.207 同段镜像口径**(▒ 背景车道例外):同线程同型跨记录、occurrence interval 真重叠的镜像账,成对即并、取并集/镜像值、**禁求和**,×N 计数+「同段镜像·不可相加」话术随行(`aggregate.go:1449-1457/1801-1813`,`MergedSameSegmentMirror`) |
| §8.6 规则5「×N 保留 rank-0 family 证据」 | partial | ×N 保留 count/显示值/极值/窗口 roster/rank 身份/RankFamilyKey/无损 MergedEvidenceIDs;**family 文法字段(FamilyMember*/FamilyFoldCaliber/BackgroundRank)被刻意清空**而非保留(RCM-2 F-1 反嵌合,`aggregate.go:1851-1864`) |
| §8.7 anchor window「三层优先级」 | partial | 实为**两层**:① frame_target_resolution 且 window_source 在白名单(query_window/显式并集变体)的 Span,多帧最后者胜;② 无 frame anchor 时,恰三 anchor family(wakeup_causal_aggregate/wakeup_causal_impact/root_cause_* 前缀)中最后一条的 typed selected_window note——审计的第 2、3 项是同一层(`trace_causal_projection.go:3061-3105/3140-3146`) |
| §9.1「board/window 身份有效」列为入榜基线 | partial | 板/窗身份**不是准入臂**:准入=四精确 typed 臂+三排除臂+微折;板身份只进小计阶梯与 multi-board 尺,窗存在性只进 Σ 资格;面板级门=`RootCauseFamilyObserved`(`answer_document_mutation_runtime_elim.go:109-124/542-560/1884`)。另漏共享臂的 target_self_state/context_only 两排除 |
| §12.1「L1 结构测试要求关键 body 字节稳定」 | partial | L1 pin 的是**行为等价**(`Mode=""` 与 `ModeRead` 的 BusContext 输出等价,`mode_dispatch_test.go:99-140`),CLAUDE.md L1 明文「not a frozen byte copy」,runReadSchedulerLoop 允许为读特性演化;审计复述的是 architecture.md:96/2895 的陈旧 byte-preserved 措辞(该陈旧措辞本身列入文档清理,见 RW10) |
| §13.5「真实验证失败用户可显式 accepted_failed」 | partial | 默认 replan 属实;但 `accepted_failed` 由 controller 路径铸出(finish disposition 撞真实失败/blocked slice/attempts 回填),**无用户直接命令面**;用户参与在 ask_user/审批环(`writeflow/controller.go:307-322`,`write_controller_scheduler.go:4699-4771`) |
| §19 trace 口径段「链上、邻接和背景根因的优化归因提醒」 | partial | ▒ 背景**从不进入 ◎ 可消除种群**(基石 C:跨线程口径无定义内可消除量,`elim.go:51-53`;▒ 区头明文「不计入链上归因」`elim.go:2642-2644`),把背景并入可消除归因量违背该边界;另漏 ◈ 业务线索名维度区。修正稿见 §5 节 |

其余小节(§4.1/§4.2/§4.3 状态表/§5.1/§5.2 条件1·2·5·6/§5.3/§5.4/§5.5/§5.6/§6.1-§6.4/§6.6/§6.7/§7.1-§7.5/§7.6 其余/§7.7 资格与守恒/§8.1-§8.4/§8.6 规则2·3/§9.1 双席折叠/§9.2-§9.6/§10 全表/§12 其余/§13.1-§13.4/§13.6/§18 索引)逐行核实 **accurate**。

---

## 2. G1-G17 逐条判定

前提:G1-G13 是前次审计(20260721)编号的延续,**§29.183 已对其逐条定谳**。本版继续按开放问题列报的条目,凡触既裁者判定如下(重开任何一条需显式推翻括注的裁定,不能以重复列报代替):

| ID | 本版判定 | 处置 | 依据 |
|---|---|---|---|
| G1 周期启发式进硬排名 | partial:effective 确是排名输入,但「注释 vs 消费矛盾」不成立——注释(`query.go:66-69`)明文把 rank ordering 列为驱动面;CLAUDE.md「硬门」定义=emit 时硬拒/contract.Check(结构正常问题的用户可见失败),排名不在其列;检测器是常数固定的确定性口径算法,非相似度启发式 | **既裁维持** | §29.183 定谳「不修,禁重诉区」+§29.185① 用户维持+DRIFTGUARD 注释(`query.go:71-83` 亲刻「do not re-file」);底层裁定 VS-1 §7.8/§29.27①/§29.19④/§29.171 |
| G2 「proven eliminable」过强、凭证不分层 | partial:census+none demote 描述准确;但「凭证不分层」不成立——**凭证四字族已按强度分层交付**(每 ⛓ 席恰佩 唤醒锚定/目标自身/交集证明/成员继承 之一,图例强→弱梯度句「词越靠后成色越保守」,`elim.go:1138-1269`/`tree.go:1740`);「准入收紧/降道」方向是 §29.183 点名禁区(推翻 §29.104 终判③/§29.134/§29.61.2) | **既裁维持** | §29.183 G2(基石 B 句保留+chip 加法修)+§29.187① 四字族+§29.188 件9+§29.209 成员继承回退裁定+§29.210 chip 引擎同源 |
| G3 P3 display-only 覆盖窄 | partial:行为描述属实,但这是**用户裁定的分阶段设计**——阶段二挂数据门(中间带活体可观才议,届时新裁定),已列外部等待项,非被遗忘的高危 gap | **既裁维持** | §29.183 G3(不修,两增量入阶段二议程)+§29.169/§29.171(数据门就位)+§29.211(阶段二=纯外部等待) |
| G4 3% near fold=嘈声硬门 | partial:删席+取 max 属实,但「近似相似度」定性失实——fold 键是**精确 typed identity**(canonical subject/object/token+真重叠+near 档双真身份门),3% 只是同一事实边界重采样抖动的值容差;「包络重叠是嘈声不可作硬门」恰是原裁定(PTV6 批②#4,138% 幻影反例)排除出门的那一半 | **既裁维持** | §29.183(不修+3.0% 恰界 pin 已落 §29.186)+§29.185① DRIFTGUARD(`aggregate.go:1101-1109` 明书禁重诉区) |
| G5 via OnChain 语义 | accurate(描述):PathComplete 已加、OnChain 仍 membership;但残留形是**裁定选定终形**——翻转 OnChain=推翻 RN-14 判语,修法已定为加法 typed path_complete | **既裁维持** | §29.183 G5+§29.186 AUDITFIX-B(「OnChain 成员语义零动」) |
| G6 D/IO 上游因果 lane 缺失 | accurate(描述);但「缺 lane」已有裁定:edge3be_eval 3b 实测缓办(0.118ms 收益 vs 400-700 LOC),用户维持+防飘逸注释 | **既裁维持** | §29.183 G6 缓办在案+§29.185① 用户维持 |
| G7 中间非 S 多分支 | accurate(描述);但 extra 候选域是**用户钉死域**,代码 DRIFTGUARD 亲刻「Widening the domain re-opens that ruling(禁重诉区)」(`query.go:23808-23812`) | **既裁维持** | 2026-07-18 候选域钉死裁定+§29.183 G7 不修 |
| G8 timestamp zero 已修复 | **accurate** | 确认 | WINFLAG-1(§29.190④/§29.199)+G8 共享谓词(§29.189);六项对抗测试实跑 PASS |
| G9 守恒措辞 | partial:「只查 eligible 子集」属实且为 §29.132 AXIOM-V2 硬纪律构造;但「外部词面需要显示」**已交付**——守恒图例种群句已落地(`tree.go:1984`「种群=严格链上全额持值席,◇ 邻近、交集证明(包络级)、计数当量与自身症状席不入」,§29.188 件10),「仍存在」判定过时 | **既裁维持** | §29.132+§29.183 G9+§29.188 RULE3-1 件10 |
| G10 tier/ordinal 双空间 | partial:描述属实且审计自认内部自洽;§29.183 已判「既裁构造+拆词已实装(rank_channel 每序数自述通道)→不修」 | **既裁维持** | §29.36.2/3+§29.104.16 RANKDIS-EXT+§29.183 G10 |
| G11 effective 注释漂移 | partial:「闭表代码正确」已亲证;「周边注释仍有历史描述」**无具名 witness**——既裁的 G11 漂移已由 AUDITFIX-A 重写(§29.186),定向 sweep(TotalMs fallback 词族)零命中;若指 supply_fold.go 头注应归 G17 | 非 gap | §29.183+§29.186 已收账 |
| G12 gated reason 注释漂移 | partial:既裁的 G12 修已落地(AUDITFIX-A);现行 gated 字段注释经查一致(`types.go:3865-3896` 含 freq_only reason 完整契约注释);「需持续清理」缺具体位置 | 非 gap | §29.183+§29.186 已收账;新主张请附 file:line |
| G13 5% overlap 只控披露 | **accurate**;「应保持这一边界」与维持裁定同向(DRIFTGUARD 注释自引 audit G13) | **既裁维持**(同向) | §29.150③+§29.160②+§29.183+§29.185① |
| G14 credential caveat 前缀去重吞名 | **accurate**(F-4 known limit 注释原文自认;单车道 >4 席「and N more」计数;sticky wire 不受影响) | **已候办** | §29.210 记档候办「caveat 席名集合并入形」+§29.211 候办族;升级路径=席名集合并入既有句,与审计建议同向;舰队 none 人口=0 |
| G15 频率 lane 丢弃后继续分类 | partial:行为属实,但机制是 **lane 物理序完整性审计(fail-close)** 而非「完整性预算」;「披露不改判定」为既裁形;升级为 completeness gate 需新裁定 | **既裁维持** | §29.129 件3(S4 收披露判定零改)+§29.150⑨+§29.163 件⑨;注:§29.206 CLUSTERTIE F2-F4 是公告快照分区条目,不覆盖此条 |
| G16 limits anchor mismatch 只披露 | accurate(描述亲证:`core_capability.go:278-292`「DISCLOSURE ONLY」);但降级/subdivision 臂已立档为「裁定候选非急件」,代码注亲刻拒绝理由(per-policy-leader emission convention 是舰队级形状假设,硬臂消费违精确信号红线) | **既裁维持** | §29.163 CLUSTER-FIX-2 C2/S9+CLAUDE.md 精确信号红线;启用降级需用户显式裁定 |
| G17 supply fold 旧注释 | **accurate,实锤**:`supply_fold.go:32-38` 头注仍描述已退役的 resolveCoreTopology/频率 tier 推断供能力类,与现行单源(`coreCapability`/`core_capability.go:20-23`「thirds-based inference…never feeds capability classes」)矛盾 | **确认真 gap** | 与 §29.183 注释漂移处置方向一致,零行为单文件头注重写,值得小批立案 |

---

## 3. RW1-RW10 逐条判定

| ID | 判定 | 处置 | 关键证据/纠正 |
|---|---|---|---|
| RW1 write 续跑无仓库身份门 | **accurate,全案实证** | **确认真 gap(本审计最有价值条目)** | `write_workflow_run.go:11-24`(envelope 确无 repo/base/branch/request-hash 字段);`write_workflow_run_store.go:317-338`(List 按 ModTime,FindActiveRun 零身份比较);`write_controller_scheduler.go:636-666`(无比较即续跑);`cmd/root.go:2114-2126`+CLAUDE.md path anchors(store 锚定 `<CWD>/.codrax` 而 `--repo` 不锚定)——同 CWD 换 repo 共享 store,跨 repo 续跑场景成立。修法与 architecture.md:1398 auto-resume 既定设计兼容(身份匹配仍自动续,不匹配 fail closed/显式选择) |
| RW2 只读 shell 子语言洞 | **accurate**(awk/sed/git branch 在白名单、`validateReadOnlyCommandArgs` 无 awk 臂故 `system()` 静态可达、真实 `sh -c` 执行仅资源 cap 无写沙箱) | **确认真 gap(护栏合同层)** | 定位分层:本工具是本地 CLI 以用户自有权限运行,validator 是 **LLM 失误护栏**而非对抗性安全沙箱(仓内从未宣称 sandbox);但护栏自身承诺「must be read-only」(`exec_command_readonly.go:193`),awk 子语言/git branch 参数/sed e 三洞违背自己的合同;且洞击穿 L6 既裁「worktree contains blast radius」前提。收窄修(awk 程序体拒 system()/getline|cmd/重定向、git branch 限只读形、sed 程序体拒 e/w)无争议;**「立即移除 awk」不可行**——与仓内成文指引冲突(`skill/defaults.go:126` trace-query 回退车道、`builtin.go:505` 工具描述推荐 awk 保行号) |
| RW3 路径无 repo-relative 校验 | **accurate**(路径校验仅 cd/GIT_DIR/git 三处;`cat /etc/passwd`、`~/.ssh` 可达;拒绝文案确承诺「stay inside the active repository」) | **确认真 gap(护栏合同层)** | 补充审计未提的两个定向缓解(不改变结论):SEC#26 sensitive-config 命名门(凭证配置文件名单拒,`builtin.go:536-542`)、多仓 active-set 门;二者非通用路径校验。OS 级只读 bind/sandbox 属产品定位,见「需裁定」 |
| RW4 baseline 缺席即 CritNoRegression satisfied | accurate(描述) | 非 gap(蓄意设计) | 代码内成文裁量:「Absence of a baseline is NOT a regression」+默认关闭因双倍 wall time(`criterion/eval.go:347-365`,`runtime.go:801-809`);criterion Detail 已带「regression check skipped (trivially satisfied)」披露。把披露提升到 completion 面(tests_passed/baseline_compared 正交字段)是合理 advisory 增强,可自由立案非必修 |
| RW5 单 sub-repo+workflow 无 repo identity | accurate | 确认真 gap(依附 RW1) | `multirepo_write_scope.go:16-17` 注释指向的 durable workflow 层协调能力与 envelope 现状(无身份字段)不一致;先修 RW1,fanout 属后续 |
| RW6 auto-init 与「HEAD 永不自动变化」矛盾 | partial:「自相矛盾」言重 | 非 gap(docs 打磨) | auto-init 需三档显式授权(REPL y/N /yaml/CLI flag),严格读「automatically」则显式授权不在否定范围;§8.12 已带「除显式 ff 路径」例外措辞、§8.13 整节成文 auto-init——例外已记载,残余仅顶层总述句可加指向例外节的脚注 |
| RW7 Save 失败仅 warning | **accurate**(`write_controller_scheduler.go:3738-3762`:store nil 直接继续;Save err 仅 Warning 无 typed 状态) | **确认真 gap(中优先,非高)** | 「durable dynamic DAG」是宣称能力,静默降级违背合同;`persistence_degraded` typed 状态或 mutation 前 checkpoint 值得立案。缓解注记:applied ref(`refs/codrax/applied`)落主仓 git,是独立于 JSON store 的 durable 通道,脱节面是 controller 状态非 applied 字节——严重度中 |
| RW8 read 降级 vs fail-loud 文档 | **accurate**(stream→硬失败;非 stream→buildDegradedSemanticIR/降级 FallbackIR 继续跑,`orchestrator.go:2226-2280/8700/8764`;architecture.md:56/:3093「重试耗尽终止 Run」与现实相悖,`orchestrator.go:2212` 段首注释同陈旧) | **确认真 gap(文档修)** | 双账本 grep 无既裁触及;降级不硬杀方向与「非致命不硬拦」(§29.104.13)同哲学。修=architecture.md 两处+陈旧注释,列出三态状态机(missing-emit retry/stream hard fail/non-stream degrade) |
| RW9 Status=complete 与 verdict 正交 | accurate(描述) | 非 gap(成文契约) | 注释明令「Consumers must read this field」+architecture.md:1398 同句+result 文案已带 caveat;`successful` 派生字段是可选 API 便利性建议 |
| RW10 历史表述残留 | partial | **确认真 gap(半条)** | 「线性 plan/apply/verify 图」残留**确凿**(architecture.md:139 节标题/:160 概览图节点/:109-121 与 143-151 两张三阶段表——正文已改述 controller DAG,标题与图表自相矛盾);「read 0 字节副作用」**不存在**——唯一「0 字节副作用」(architecture.md:96)语义是写机械对读路径的零字节影响(L1),非 read 副作用声明。应改列真实存在的另一处漂移:architecture.md:96/1396/2895 的 byte-preserved/完全不变措辞与 CLAUDE.md L1 现行定义(行为等价,非冻结字节拷贝)不一致 |

---

## 4. §16 修复优先级重排(核验后)

原审计 P0-P3 混入了既裁禁区与需裁定项,按核验结果重排:

**建议立案(真 gap,按序)**
1. **RW1**:write workflow envelope 增 canonical repo root/repo fingerprint/base SHA+branch/goal hash 四元组,续跑前逐项精确匹配,不匹配 fail closed 或显式选择;A→B 对抗测试(原审计验收测试 17.2-1/2 采纳)。
2. **RW2/RW3 护栏收窄修**(不移除命令、不改产品定位):awk 程序体语义验证(拒 system()/getline|cmd/输出重定向)、git branch 限只读形、sed 程序体拒 e/w/W、常用读命令 path operand canonicalize+仓内校验(原审计验收 17.2-3/4 采纳,「移除 awk」半句不采)。
3. **RW7**:mutation 前 durable checkpoint 或 `persistence_degraded` typed 终态+save-failure 注入测试(原审计验收 17.2-6 采纳;定级中)。
4. **文档清理批**(零行为):RW8(architecture.md §1.3/:3093 三态状态机+orchestrator.go:2212 注释)、RW10 线性图残留四处、L1 byte-preserved 措辞三处对齐 CLAUDE.md 现行定义、G17(supply_fold.go:32-38 头注重写)、RW6 顶层句加例外脚注、L6 措辞随 RW2 修复同步(「worktree 含 blast radius」前提在洞修复后重新成立)。

**需用户裁定(不应默认 P0)**
- P0-1 后半:exec observation 是否从「LLM 失误护栏」升级为 OS 级 capability/path sandbox(安全边界定位)。涉及:威胁模型定位、awk/sed 取舍与 skill 指引冲突、L6 表述。

**触既裁,不立案(重开需显式推翻对应裁定)**
- P1 全部(「proven eliminable」改词=推翻 §29.183 G2 基石 B 句裁定;凭证分层已由四字族交付;member_inherited/target_self 出「proved」人口=§29.183 点名禁区)。
- P2 全部(周期 advisory 化=推翻 VS-1 §7.8 等四层裁定;V4 只标 suspected=推翻 PTV6 批②#4+§29.183;cluster 完整性退 freq_only=推翻 §29.129 件3/§29.163 既裁「披露不改判定」——G16 的降级臂以「裁定候选」身份在档,可走裁定议程,非直接实施)。
- P3-1/P3-2(D/IO resource lane、非 wakeup branch)=§29.183 缓办+候选域钉死,禁重诉区。
- P3-3/P3-4 中的 completion 拆解与文档统一部分并入上面「立案 3/4」。

**验收测试(§17)对应处置**:17.1-1/2/3(G1/G4/G5 形)不立(既裁);17.1-4 凭证词面分别展示——已交付(四字族);17.1-5(none 名单完整)=已候办(G14 席名集合并);17.1-6(G8)已存在且 PASS;17.1-7(G15/G16)不立(既裁);17.1-8(eligible/total)守恒种群句已交付,数值比例披露属新增强需另裁;17.1-9(P3 第一批消费契约)=阶段二数据门,外部等待。17.2-1/2/3/4/6 采纳(随 RW1/RW2/RW3/RW7);17.2-5(read 前后 source tree+HEAD 不变)合理可随护栏批;17.2-7(RW4)advisory;17.2-8(RW9)非 gap。

---

## 5. §19 对外口径修正稿

trace 段(修正「背景入可消除归因」+补 ◈ 名维度;其余保留原文):

> Trace 根因榜先按真实链路相关性分区,再按类型闭表折算后的窗内有效归因排序;普通 sleep、gap 和无供给缺口的 running 不靠原始时长抢名次。真实 scheduler 唤醒边只由满足条件的 S-sleep 与受支持的 sched_wakeup/sched_waking 事件建立(两类型同池取最新,仅排除 sched_wakeup_new)。答案页「窗内可消除量」= **链上(每席佩凭证四字族 chip:唤醒锚定/目标自身/交集证明/成员继承,词越靠后成色越保守)与邻接(条件上界)两个同尺通道的优化归因提醒**,另设 ◈ 业务线索(名维度自查项)与 ▒ 背景(跨线程口径语境,不计入链上归因、不属可消除量);除非某项同时具有可消费的精确反事实凭证,否则不能承诺该毫秒数一比一转化为修复后的目标延迟下降。

read/write 段:第一句(read 边界)准确保留;第二句改为:

> Write mode 通过确定性风险分级、fingerprint 绑定审批和隔离 worktree 约束 apply,主仓 HEAD 除两个显式授权例外(显式 ff merge、显式 auto-init)外不变。当前确认的两项边界缺口:自动续跑须绑定仓库/基线身份(RW1),只读 shell 观察面须补齐其自身承诺的只读/仓内合同(RW2/RW3 护栏收窄);在此之前,不应以模式名称宣称隔离已闭环——但也不应将该护栏表述为对抗性安全沙箱,该定位升级属未裁定的产品方向。

---

## 6. 给审计作者的方法论提示

1. **§29.183 勘误清单已于前次回传,本版重复了其中两处**(§5.2 条件3/4 铸边优先级错置、§6.5 2/3 分母)。后续版本请以勘误清单为增量基线。
2. **G-编号既裁定谳未消化**:G1-G13 在 §29.183 逐条定谳(裁定账本 `real_trace_campaign_20260705.md`),多数「不修+禁重诉区」并有 DRIFTGUARD 代码注释亲刻(`query.go:71-83`、`aggregate.go:1101-1109`、`query.go:23808-23812`、`rank_direction_axiom.go:100-106`);部分残口已在 §29.185-§29.188 以加法形交付(PathComplete、凭证四字族、守恒种群句、rank_channel 词)。列报开放 gap 前请先对该节双向核账;重开某条时请显式写明「建议推翻裁定 X,理由 Y」。
3. **本版实质贡献被真 gap 淹没风险**:RW1 是全案实证的高价值发现,建议单独成案推进,勿与既裁重诉项混在同一优先级序列。
