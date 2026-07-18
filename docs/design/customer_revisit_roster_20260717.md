# 客户回访汇总包(customer_revisit_roster)

盘点时间:2026-07-17。基线:main=4b90fd27f(HULL-CRED §29.126 已收账)。
盘点范围:docs/design/real_trace_campaign_20260705.md(战役账本全文,重点 §29.104 立案族+§29.105-§29.126 收账节)、customer_dead_session_audit_20260703.md、trace_analysis_open_gap_ledger_20260710.md、causal_tree_v5_design_20260711.md、MEMORY 候选、/Users/han/opt/customlogs 工件在库核对、tdkit_20260715 采集包。

**总数:13 项**(A 组修后复放 7 + B 组工件补交 3 + C 组容量/低优 2 + D 组观察束 1)。
排序原则:客户高频使用场景优先(多步查询=客户近期高频高优,§29.104.11 用户调序原文)。

**通用前提**:全部复放项要求客户使用**含 §29.125/§29.126 的新构建**(main ≥ 4b90fd27f;此前客户在用 build=0.1.20260710 不含本窗二十节修复)。LLM 复放=用原问题+同 trace 附件按客户正常用法重跑;确定性采集=零 LLM `--tracediag`(不读 providers.yaml、离线可跑),命令形见各条。

---

## A 组:修后复放件(客户侧操作)

### A1. runnable2 多步场景复放(XLANE 全族修后)——最高优
- **① 案名+账本节**:XLANE 族。立案 §29.104.1/.2(witness=customlogs/runnable2.txt);修复 §29.106(XLANE-1)+§29.109(XLANE-3)+§29.125(XLANE-2,commit 65d7b227a);§29.104.9 XREPRO 本地全量复现;§29.125 明文「客户 runnable2 复放=外部回访项」。
- **② 修了什么**:XLANE-1=runnable 族跨车道双算修根(卫星席全覆盖→整席 ◇ 降道+「锚定份由链席[E#]代表(整席降道)」互指;三级精确区间库存拒 hull 包络;「自身·」佩词校验 Subject==树 target)。XLANE-3=跨步熔合门根修(板身份=(窗,target,参数指纹) typed 三元组;chip 分域佩「·板锚 <comm-pid>」;跨板互指+跨板 Σ 按板分域)。XLANE-2=语义族真子集降道互指(E34=E35∪E49 形→「为[E#]成员子集(整席降道)」)+裁定④ self-gap×语义席重叠披露(「其中 X ms 与语义席[E#]重叠」,主值零动)+E11 三面矛盾 rider。
- **③ 客户侧做什么**:新构建下,对 runnable2 同一 trace、同一多步查询序列(同窗异 target 多步)原样重跑,回传完整报告(md)。若可,附 session log 供步序列对账(§29.104.2 待验证项:E15 承自行是否被 LLM 叙事误当第四份账)。
- **④ 预期验收句**(对照修前病形):
  - 链上 runnable 族 Σ ≤ 该线程全窗 runnable(修前 E11 23.471+E26 17.635=41.106 > 全窗 26.725 的 1.5×;修后恰一全额席,其余席互指降道)。
  - 榜位撞号=0:两板 #1 各佩各「·板锚」chip,不再出现裸 chip「根因排序#1」×2。
  - 跨板 Σ 病句消失:不再出现「各根因席位有效归因合计 355.562ms 超过窗长 233.190ms」类两板混加;各板 Σ 各<窗。
  - 「锚定0.018(⛓链上席)」与「40.071 ⛓ 全额」两说同树并存形消失(同线程跨板席双向互指句在场)。
  - E34/E35/E49 语义族:子集席佩「为[E#]成员子集(整席降道)」,值 9.586/6.376/3.210 逐字保持。
  - E29/E32(shadowhook)不再错佩「自身·墙钟席」(非 target 永不佩「自身·」)。
- **⑤ 缺什么工件**:runnable2 原始 ftrace 未入库(→B2 v2_slim 补采,已裁降级为修后回放确认级,不阻塞);复放报告本身即验收工件。

### A2. cust_total_del 同帧复放(ELIM-GAP+GATED-CAL 修后)——客户高优件三号
- **① 案名+账本节**:ELIM-GAP。立案 §29.104.15(witness=customlogs/cust_total_del.txt,系 cust_err1 同帧 58558 重跑);修复 §29.112(ELIM-GAP-FIX 四件)+§29.115(GATED-CAL 三件+裁定①);§29.112 明文「客户侧同帧复放=外部回访项」。
- **② 修了什么**:◎ 可消除量总览种群臂补第四臂 Node.Rank>0(修前 15 榜席 6 席静默消失);结构闭合恒等式(渲染成员数+计数披露==全种群,静默消失=0 由算术保证);值切计数披露「⛓/◇ 持席行另有 N 行未入榜(TOP5 值切)」;折叠词面读 typed 真相;「(发生段账目)」口径词。GATED-CAL:gated 复合席「(全额)」假盖修(构成恒等门,三臂:构成词/保全额真话/宁缺勿造)+头条「链上累计」跨线程词卸下+裁定①降道卫星出 ◎(专用脚注「已由链上席代表(降道):N 行」)。
- **③ 客户侧做什么**:新构建下同帧重跑(与 cust_total_del 报告同一 trace=record_trace_20260526170707@880、同窗同 target 同问题),回传报告。
- **④ 预期验收句**:
  - 根因排序#2=[GT]ColdPool#9-48667 runnable 8.211ms 出现在 ◎ 板(应列第二 bar;修前完全缺席零披露)。
  - 尾注不再只数语义行:非语义持榜席值切有「另有 N 行未入榜(TOP5 值切),见明细」逐通道披露。
  - 降道卫星不再以全值 ◇ 条占 ◎:改为脚注「已由链上席代表(降道):N 行 [E#…]」。
  - gated 复合席(如 3.429=2.181+1.248 形)不佩裸「(全额)」:佩「构成,见明细」且明细拆解在场;纯全额席保「全额」真话。
  - XERR1 修复面维持:⊗ 假阻塞席不回归(A4 参照)。
- **⑤ 缺什么工件**:同 A4(record_trace_20260526170707@880 原始 ftrace 未入库);复放报告即验收工件。

### A3. cust_span_runnable 同帧复放(HEADLINE-ELIM+XERR1-EXT 修后)
- **① 案名+账本节**:HEADLINE-ELIM。立案 §29.104.14/.14.1(witness=customlogs/cust_span_runnable.txt,donghu 970481 帧);修复 §29.110(三件+修补轮六件);伴随 §29.113(RANKDIS-EXT)/§29.116(XERR1-EXT,榜序变更用户已准)/§29.118(值词库教学)。
- **② 修了什么**:①skill 教学硬度(推翻榜首必须显式「与根因排序分歧」声明+并排引用两已发布值的数值对比;禁范畴论证降级 #1;请求内叙事=调查线索永非排序证据);②校验附注确定性并置披露臂(prose_headline_elim_check.go:标题因实体与根因排序#1 失配→并置句,纯披露永不硬拦);③rider=算力/频点宣称 × typed 供给折算缺口席并置(含否定/虚拟前视/「之一」/归属前缀护栏,修补轮六件)。XERR1-EXT:payload-typed 真锁行值收敛 Σ(sleep+D+iowait)(改值通道且改榜序=用户已准)。
- **③ 客户侧做什么**:新构建下对同帧(record_trace_20260606021843@17686,窗 17729.471126..17729.622508,target 63993,即 tdkit case1 主分析面)重跑原问题(含客户前置分析叙事「IdleHandler 被 shadowhook 阻塞」原样保留——修复应对叙事锚定免疫),回传报告。
- **④ 预期验收句**:
  - 标题句不再无声推翻榜首:或正文首因=类校验(#1 9.586ms primary);或如仍写 shadowhook,必须带显式分歧声明+9.586 vs 8.608 数值对比;两者皆无时校验附注出现并置句「正文核心/首要原因=X(…);本报告根因排序#1=Y(有效归因 N ms)。两者不一致…」。
  - 「排除算力供给不足」类宣称如与 E5 折算缺口席(4.843ms)并存→并置 finding 在场。
  - ART 真锁行值=等待段收敛值(非 span 包络);榜序变化按 §29.116 为设计内(如有席位换径,值三分量可手算闭合)。
- **⑤ 缺什么工件**:该帧原始 ftrace 未入库(→B3,tdkit case1 轨B);复放报告即验收工件。

### A4. cust_err1 XERR1 复放——**已客户侧闭环(留档)**,残留=工件补交
- **① 案名+账本节**:XERR1。立案 §29.104.3(witness=customlogs/cust_err1.txt);修复 §29.107(四件+XCPU rider+修补轮八件);**§29.104.15 客户回放验收 PASS**:同帧重跑 ⊗ 假阻塞席(199.992ms 冒充)消失,自身 sleep 8次 64.301 诚实席+覆盖注在场,客户评价「答案和因果投影树都很正常」——四防线产线实证,客户侧闭环。
- **② 修了什么**:词首边界化噪声筛(resynced/async 出局)/值收敛 Σ(sleep+D+iowait)/词面 fork(⊖ 阻塞等待候选 vs ⊓ span包络(含运行))/WaitBudgetExceeded ⚠ 预算披露/XCPU 取收尾切入 CPU 禁 wakeup target_cpu。
- **③ 客户侧做什么**:无需再复放。仅剩 B1 工件补交(原始 ftrace)。
- **④ 预期**:n/a(已 PASS)。后续新 trace 若对「最后唤醒者(推断)」等词面有误读,按 §29.107 涟漪备案再动词面。
- **⑤ 缺什么工件**:record_trace_20260526170707@880 原始 ftrace(13.8MiB)未入库→B1。

### A5. cust_span_vs_prio 复放(RANKDIS 词族四批修后)
- **① 案名+账本节**:RANKDIS。立案 §29.104.16/.16.1(witness=customlogs/cust_span_vs_prio.txt;sweep 全文=docs/design/rankdis_sweep_20260716.md);修复 §29.113(RANKDIS-EXT 六件)+§29.117(M18 复合分数迁出 _ms 键槽)+§29.118(值词库教学批)+§29.119(反转词位批)。
- **② 修了什么**:rank 键三面分叉(state_drilldown→drill_rank/auto-window→window_rank/hotspot→density_rank/根因板行内 rank_channel=chain|adjacent 自描述);wakeup path#N→branch=N(+「branch 编号=分支身份非排名」教学句);复合分数(io_pressure/block_io/rank_impact_ms 族)迁 *_score 键+「(composite score, not wall clock)」词面(修前 146ms 窗发布 impact=635.077ms 假墙钟=本案病类);值词库映射教学+幻键 projected_total_ms 修正;反转席三面同词「优先级反转·可运行等待」。
- **③ 客户侧做什么**:新构建下重跑原问题(与 cust_span_vs_prio 同 trace 同窗;与 A3 同帧族,可与 A3 合并一次复放),重点回传**模型校验轮 transcript**(修前病=模型把 state_drilldown rank 当根因板反复 reconcile 五轮)。
- **④ 预期验收句**:
  - transcript 不再出现「rank 1-8 全是与目标无关的 s_sleep 线程」类误读(状态手递面已改 drill_rank);无双 Rank#1 reconcile 循环;正文无「排名不一致」自我调和段。
  - 复合分数值佩 composite score 词,不再以 ms 假墙钟出现在正文。
  - 类校验族两套总账(全窗跨线程 13.247 vs 目标自身 9.586)带 scope 限定词,模型不再两轮怀疑孰真。
- **⑤ 缺什么工件**:无硬缺(转录在库);复放 transcript+报告即验收工件。

### A6. endless_loop 场景复放(完成门+ELIM-1 修后)——裁定池残留件
- **① 案名+账本节**:完成门 P0。立案 §29.60/.1(witness=customlogs/endless_loop.txt,客户案=根因XX-VerifyClass);修复 §29.60.2(完成门修复批,endless_loop P0 关账:唯一驱动器=checkTier1Floor 失败→requeue+ResetInvestigationComplete 清零循环)+§29.67(RANK-U Stage 2:ELIM-1 ◎ 总览落地,本案=ELIM-1 第一客户 witness,入 cust710 复放验收清单)+§29.104.13(非致命不硬拦原则扩展成文阶段)。
- **② 修了什么**:完成门尊重模型判定(修后 7/7 首 emit 出厂、0 降级轮、0 requeue;修前 donghu 4/4 requeue×4);ELIM-1 ◎ 窗内可消除量总览(链∪◇ 纯 eff 降序,VerifyClass 13.006 抬到全板第 2 行佩「确定性优化·候选」);SELF-SEM 自身确定性语义 span 入链上通道。
- **③ 客户侧做什么**:新构建下对 endless_loop 原案(根因XX-VerifyClass)同 trace 同问题重跑,回传报告+粗略耗时(修前 46min 级死循环)。
- **④ 预期验收句**:
  - 无 requeue 循环:一轮完成,无「反复清零+同指令重发」;答案不再严重偏离。
  - VerifyClass=根因排序#2+❷ 徽章(§29.60 裁定 witness 验收句);◎ 总览第 2 行=VerifyClass 佩「确定性优化·候选」。
- **⑤ 缺什么工件**:原 trace 仅客户机(转录 endless_loop.txt 在库);复放报告即验收工件。

### A7. 对比/runnable/data 三场景新一轮回访(总括伞项,含 cmp 回探实跑)
- **① 案名+账本节**:MEMORY 候选「客户回访验证(外部依赖):对比/runnable/data 三场景新一轮回访」;对比场景=customer_dead_session_audit_20260703.md §7.5/§7.6(「对比场景实战效果待客户回访」+回访 NEW-1..NEW-5 修后确认)+§29.23(CSP-RM:「cmp 形…原件仅客户机→实跑列客户复放项」);data 场景=DR/DL 修根(memory:data 复跑归因两系统类已灭)后客户新构建回访;runnable 场景=A1 已单列。§28.8 亦挂「客户下一构建全场景回访」总项。
- **② 修了什么**:对比=CMP-A/B/C 全家桶+NEW-2(总览表门改精确信号)等回访 gap 批;cmp 回探=CSP-RM trace 钻取深度 typed 信号(首轮零 rank 观测→回探照发带 trace 词面,钻取后自灭);data=DR 60s 独立 lane+DL workflow 三发布面修+DLR 四梯级投影 ladder。
- **③ 客户侧做什么**:新构建下,①对比场景:双 trace 对比原案(如 trace_cmp_cust 同 bindApplication 案)重跑,确认对比总览表/供给列/对比下一步在场(NEW-2 修)、图例唤醒方向句(NEW-1 修)、followup 回探行为(首轮证据不足时回探一次即收敛);②data 场景:data 复跑归因原案重跑,确认 route=data 稳定+无 workflow 死锁;③runnable 场景按 A1。
- **④ 预期验收句**:对比报告 per-工件双投影+对比总览表恒在场(≥2 已编译投影即出,不再吃 LLM 分类方差);回探指令不再点名「Missing repo_map lenses」类结构性不可满足项;data 场景 15/15 route=data 级稳定、无 46min 死等。
- **⑤ 缺什么工件**:对比双 systrace 原件与 data 场景原件均仅客户机;复放报告/transcript 即验收工件。

---

## B 组:工件补交件(客户回传原始 trace,均已备 tdkit_20260715 采集包)

采集包=/Users/han/opt/customlogs/tdkit_20260715(README_采集说明.md+三 yaml+carve_window.sh;零 LLM、只读、离线、不读 providers.yaml)。轨A=tracediag 报告(体量小、basename 脱敏);轨B=carve_window.sh 时间窗切片(窗内全事件行原样保真,敏感度与原 trace 相当,需数据政策允许)。

### B1. record_trace_20260526170707@880 原始 ftrace 补交(XERR1/ELIM-GAP 帧,13.8MiB)
- **① 案名+账本节**:§29.104.3(「原始 ftrace(record_trace_20260526170707@880,13.8MiB)未入库,复放待客户补交」);tdkit case2。
- **② 已修**:XERR1 §29.107(客户复放已 PASS)+ELIM-GAP §29.112。补交目的=本地 fixture 承载(替代合成 pin)+BLIND-2 泛化臂误 admit 面定 scope(§29.104.4 ④ 明文「待客户 ftrace 定 scope」)。
- **③ 客户侧命令**(轨A,已验证参数形):
  ```
  ./codrax --tracediag collect_case2_main.yaml \
    --trace <record_trace_20260526170707@880 对应 ftrace> \
    --trace-window 925.410..925.610 --trace-tid 48517 \
    --out case2_main_report.txt
  ```
  轨B(政策允许时):`carve_window.sh` 对同窗(建议 925.3..925.7 略放宽)切片回传。
- **④ 预期**:切片入 /Users/han/opt/customlogs 后本机复放 A2/A4 全链;BLIND-2 owner-key 行 scope 定谳。
- **⑤ 缺**:原始 ftrace 本体(仅客户机)。

### B2. runnable/runnable2 原始 ftrace 补交(v2_slim,RNB+XLANE 真机 witness 族)
- **① 案名+账本节**:§29.88(「原始 ftrace 未入库待客户补交」,witness=runnable.txt)+§29.104.9 补采终判(「客户补采(v2_slim)降级为修后回放确认级,不阻塞修复批」)+§29.104.1(runnable2.txt)。
- **② 已修**:RNB-1/2/4/5A/5B+R3-IMPL+XLANE 全族。补交目的=多个「合成 pin 承载、真机 witness 待客户 ftrace」残口转真机:§29.88 件4 ELIM-SEM 保底臂真机触发 witness/E7 具体案复放/跨边二分/多跳/◇语义≥3 形(§29.90 残留)。
- **③ 客户侧做什么**:runnable/runnable2 两案对应 ftrace 按 tdkit 轨B 切片(窗按各自报告分析窗放宽 ±0.2s)或轨A 报告回传;若与 B1/B3 同 trace 可合并。
- **④ 预期**:入库后本地复放 A1 全链+RNB 残口真机 pin 替换合成 pin。
- **⑤ 缺**:原始 ftrace 本体。

### B3. record_trace_20260606021843@17686 原始 ftrace 补交(HEADLINE/RANKDIS 帧)
- **① 案名+账本节**:tdkit case1(README §3.1);对应 witness=cust_span_runnable.txt/cust_span_vs_prio.txt(§29.104.14.1/§29.104.16)。
- **② 已修**:§29.110/§29.113/§29.116-§29.119。补交目的=本地承载 A3/A5 复放与真机 pin。
- **③ 客户侧命令**(轨A):
  ```
  ./codrax --tracediag collect_case1_main.yaml \
    --trace <record_trace_20260606021843@17686 对应 ftrace> \
    --trace-window 17729.471126..17729.622508 --trace-tid 63993 \
    --out case1_main_report.txt
  # 钻取面:collect_case1_drill.yaml 同窗 --trace-tid 64305
  ```
  轨B:同窗切片回传。
- **④ 预期**:入库后本地复放 A3/A5 全链。
- **⑤ 缺**:原始 ftrace 本体。

---

## C 组:容量/低优验收件

### C1. 260M 东湖原件容量点
- **① 案名+账本节**:§29.38(「东湖标准 trace 本地验收…客户清单收缩至一条(260M 原件容量点,若有)」;本地只验了 3.5MB/233ms 切片=donghu_acceptance_20260711)。
- **② 已修**:标准转换链解析 27845 行 unparsed=0(delay= 尾字段/[module] caller 新格式无损);W-2/W-8 关账。
- **③ 客户侧做什么**:对 260M 东湖原件(若存在)在新构建下跑一次完整分析(LLM 或轨A tracediag 均可),回传报告+峰值内存/耗时粗数。
- **④ 预期**:全量解析零 unparsed、容量不爆(无 OOM/超时)、报告质量与切片一致。
- **⑤ 缺**:260M 原件(仅客户机,「若有」件)。
- 附随观察:W-8「精确同键并发形」witness 触发臂在干净基线未现,260M 原件是最可能的自然 witness 源。

### C2. cap2/berlin 新旧序数映射表重验收(低优)
- **① 案名+账本节**:§29.38(「cust710 cap2 复放缺件诚实跳过(berlin.systrace 1.16GB 仅客户侧),新旧序数映射表产出为客户重验收清单」);映射表例=VSync 周期源 tertiary #5→◇ 邻近影响#2(§29.38 P1-2 裁定)。
- **② 已修**:UXR-1 3+1 通道+显示九项(序数语义重划)。
- **③ 客户侧做什么**:按映射表清单对 berlin.systrace 跑 `collect_cap2.yaml`(examples/tracediag/)对照新序数;或直接 LLM 复放对照。
- **④ 预期**:各席位按映射表落位(周期源入 ◇ 邻近通道等)。
- **⑤ 缺**:berlin.systrace 1.16GB(仅客户侧)。**优先级注**:§29.32 用户裁定 berlin=非标准旧转换产物、容错即可禁过度拟合,且远端转换工具已修——本件为最低优先级,可并入任一次回访顺带。

---

## D 组:观察束(无需客户专门动作,随下一批新 trace/复放留意)

### D1. 「待客户新 trace 自然活体」观察清单(一束七条)
- **PERIODIC-DEDUP CWD 不可证形 Σ 双计残余**(§29.105 残留:fail-open 设计,负向 pin 在岗;「产线活体待客户新 trace」)。
- **io facet 域外锚定首个产线实锤**(§29.87 件③:file_io_hot_inode/workqueue/dma_fence 三 facet 应锚定,双旗舰窗零产线 on-chain 在场,「首个产线实锤待客户新 trace」)。
- **ε-overlap 门不加**(§29.104.17 ⑥:部分相交保链 fail-open 维持;「客户复放出现自然 ≪10% 形再启用」)。
- **C5 置信档车道常量重标定**(裁定③ §29.104.17:图例披露句已落 §29.118;「车道常量统一标尺重标定=缓,观察客户复放后再议」)。
- **XERR1-EXT 锁席侧道保席 vs 披露**(§29.116 修补轮:默认参数车道锁席帽截已配 fail-loud caveat;「侧道保席留候选=客户复放若要席再议」)。
- **完成门 keep-alive 分支 debug 实证**(§29.60.2 修复批遗留,P2 卫生:「客户侧 keep-alive 分支 debug 实证」)。
- **XERR1 词面误读观察**(§29.107 涟漪备案:「最后唤醒者(推断)」坐旧对端括号槽等四条词面观察,「客户复放若误读再动」)。

### D2. LOCKNS 客户侧 owner-key 微型普查(可选,<5KB)——已裁不阻塞
- **① 案名+账本节**:§29.104.11(「客户侧降为**可选**微型普查(event_search 全文件 owner-key 行,<5KB),不阻塞任何批」);修复 §29.111(LOCKNS-FIX 六件+G1 用户裁定 §29.104.12.1)。
- **② 已修**:ns-divergence 精确门(容器 ns-tid 撞宿主线程不再错指向量)/形A vendor 前缀词边界/形态注册表+未知形 fail-open 披露/哨兵全分支披露;本地普查已证 BLIND-2 scope 干净(全部 owner-key 行=ART 两形零 vendor 杂形)。
- **③ 客户侧做什么**(可选):对代表性 trace 全文件 grep owner-key 行(`monitor contention with owner` / `Lock contention on ... (owner tid:` / 其它含 `owner` 的锁形),回传去重后的**形态样例**(非全量,<5KB)。目的=确认客户环境无本地未见的 vendor 杂形。
- **④ 预期**:样例全部落入已注册两形(形A 三段含 blocking-from/形B 含哨兵);出现未知形则按注册表 additive 扩(fail-open 词面已保底,不会错归因)。
- **⑤ 缺**:普查样例文本(可选件)。
- 伴随:东湖容器撞号案下一次复放时顺带确认(§29.111 客户面五点:撞号错指向量灭/vendor 前缀恢复/未知形披露/哨兵披露/Android 非容器零影响)。

---

## 已核对为「不开」的项(防重复立项)
- **XERR1 客户复放**:已 PASS 闭环(§29.104.15),仅余 B1 工件补交。
- **T-span 变体解析**:客户 line 140711 邻域已回传一锤定音,根修已解锁开工(账本 §16.2),非开放回访项。
- **cust710 四场景清单(huadong/cap2/textup/cmp)**:ORD/CAP-3/G12 已于 §29.28① 三验收关账;D-10 机制闭案(双口径各真,§21 区);残余=cmp 回探实跑(并入 A7)与 cap2 序数映射表(C2)。
- **HULL-CRED/金样 LLM e2e**(§29.126/§29.125 备案「客户复放=外部回访项」):无独立场景,随 A1/A7 任意一次复放自然覆盖(D/IO keep-⛓ 降道词面「无链上凭证(逐段核验,整席降道,见图例)」在场即证)。
