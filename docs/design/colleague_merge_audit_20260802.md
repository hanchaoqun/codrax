# 同事合入全量审计(2026-08-02,MERGE-AUDIT-3)

**范围**:main 上 23c0c58ef(补记三十六批修复收官)之后的全部非本席合入,共 **470 笔**(2026-07-26 → 2026-08-02,+107,641/−8,454 行),覆盖两大战役:

- **hitraceconv 转换保真战役**(07-26→27,~111 笔;同事账本 `hitrace_conversion_fidelity_gap_audit_20260726.md`)+ 新 `cmd/trace_convert_diagnostic`(~81 笔);
- **eval 优先级战役**(07-28→08-02,B21..b47 读写回放审计波;同事账本 `eval_priority_campaign_audit_20260730.md`),大量触及答案显示面/证明车道/write 校验域。

**方法**:9 主题读者 + 逐条 2 席对抗否证(39 agent,wf_5ebeb720-bf1)+ 全仓套件基线实测 + 高危项主会话逐字亲验。**只审计不修码**(用户指令);复现状态逐条标注。产出:27 原始 finding → 对抗复核后 **13 确认(3 高危)+ 2 存疑 + 12 低危顾问**,另有基线 2 件 P0 红测为确定性复现。

**结论一句话**:两大战役方向与工程纪律总体扎实(抽检 9 条"已实现/全测绿"账本声称全部与代码相符,零虚账;红线零直接违反),但 **main 当前是红的**(P0×2,tracediag pin 未同步),且 eval 战役在我方显示裁定域引入了一个**需用户裁定的加冕降级词面**(高危 T3-1),外加两个高危机制缺陷。

---

## §1 P0:main 基线红(确定性复现:`go test ./internal/tracediag/`)

全仓套件唯二红测,均由合入引入、tripwire 正常工作而合入者未跑该包:

### P0-1 R2' schema pin 未同步(TestNonEventPrioritySchemaPins)

`internal/tracediag/render_key_first_test.go:214` 四个 struct 哈希齐漂(Result / RootCauseRankItem / WakeupCausalImpact / WindowStats)。直接诱因:e5a20e012(07-31「separate trace search coverage authority」)在 `tracequery.Result` 新增 `EventSearchCoverage` 等 typed 字段,未走 key-first 渲染器的"每个新字段显式处置(skip 或定格式)+ 重钉哈希"ritual。修复是机械的(处置新字段+重钉),但必须由修复者逐字段裁定渲染面,不可盲重钉。

### P0-2 tracediag 坐标科学计数法泄漏(TestRunBerlinMagnitudeCoordinatesFixedPoint)

`internal/tracediag/run_test.go:494`。新 `event_search_coverage` 面经反射式 key-first 渲染走 `%v`,大时间戳输出 `6.7932221e+06`(科学计数法 + 精度截断),违反 tracediag 定点坐标纪律。与 P0-1 同根(新字段未显式处置),同批修复。

---

## §2 高危确认(3 件,对抗复核 2/2 幸存 + 主会话亲验)

### T3-1 加冕头行 causalUnproven 降级:sticky 触发 + 备案词族启用 + 定义括注被移除 【需用户裁定】

- **位置**:`internal/tool/answer_document_mutation_runtime_tree.go:15341`(降级前缀)/ `answer_document_mutation_runtime.go:4071-4075`(sticky 聚合);提交 cf6b31cc5 / 0e9e0524a(08-01)。
- **机制(亲验)**:`runtimeTraceCoverageAuthority` 对本次会话**全部** trace_query 结果做 ANY-聚合——任意一次结果带 `CausalConclusion=="unproven"` 即置 `causalUnproven=true`,**之后被证实也不清除**;命中时加冕头行从裁定形 `**主根因(=已证链上单项最大可消除量):** ` 换写为 `**首要可消除候选(不等于已证帧因果):** `,图例定义条同步换写(tree.go:2519)。
- **三层问题**:①**触发信号违反"精确信号才可作硬门"红线**——探索期粗窗 wakeup_chain 零行(典型瞬态)即永久降级最终已证实的加冕头行;②**词面权属**:「首要可消除候选」启用了 B+ 定谳(账本 §17)中**备案保留**的「首要可消除…」词族——该词族按裁定仅在用户裁定 A 方案时启用;且降级形**移除了用户第二裁(§17.1)钉定的定义括注**;③降级语义本身(未证因果不宣称加冕)是正当诚实机制,**机制方向不否定**,但触发与词面须重裁。
- **建议**:触发改为"最终编译投影的加冕席自身无已证链上凭证"级精确信号(席位级,非会话 ANY);词面替换形与保留词启用**提请用户裁定**。
- **复现状态**:静态确认(代码逐字亲验);行为复现需构造"先粗窗零行、后细窗证实"会话,非确定性,未跑。

### T3-2/T5-1/T6-1(三镜头独立汇合)图表回边硬门:硬拒 prompt 自己教的 `-->>` 回复箭头

- **位置**:`internal/tool/answer_document_diagram_evidence.go:48-56` + `answer_document_pre_emit_check.go:389/454`(SoftByDefault:false);`internal/mermaidcompat/parse.go:90` 把 `-->>` 当普通边解析;而 `internal/skill/defaults.go:515` 教学示例本身含 `service-->>client: evidence`。
- **机制(亲验)**:QFCallChain 答案的 sequence/call_dag 图,每条解析边硬性要求 `relation_kind=call` 锚 + 同向 typed 调用边证据;回复箭头(callee→caller)**构造上**无法满足任一臂,也没有 return/reply 关系类可标注。模型照 prompt 画标准回复边 → 同轮硬拒 → 模型被教的形无法自愈 → 烧重试预算,结构正常答案失败。
- **红线**:硬门与教学面自相矛盾 = "对结构正常问题产生用户可见失败"的教科书形;修法方向:解析层区分回边并入软臂/豁免臂,或新增 reply 关系类。
- **复现状态**:静态确认(解析表+教学例+硬门三点齐验);eval 复现依赖模型画回复边,非确定性,未跑。

### T7-1 write 累计校验域在"恢复既有计划"重规划车道自清空

- **位置**:`internal/orchestrator/write_verification_scope.go:18`(`plan.CumulativeVerificationScope = nil` 后重建、排除 currentID)× `write_controller_scheduler.go:1290-1292 / 1421-1423 / 331-336`(重规划把**先前计划自身**重装为 active plan 的两条车道)。
- **机制**:stamp 以 `plan == priorPlan` 运行时,把先前计划自己的传递域摧毁且无从重建(checkpoint cutoff 之前的批被排除)——最终 verify 在**空缺累计域**上签绿,早批改动逃出终验覆盖。
- **红线关联**:L2 精神(write 门不可被路径侧绕过);「收窄落地范式」要求收窄必须有判决性证明,此处是隐式收窄零披露。
- **复现状态**:静态确认(两车道+清空点逐点核);行为复现需多批 replan-restore 工作流,未跑。

---

## §3 中危确认(8 件)

| # | 位置 | 要点 | 复现 |
|---|---|---|---|
| T1-1 | `hitraceconv/source_raw_scheduler_lite_join.go:329` | raw 未匹配 sched_switch 补发的去重仅靠**纳秒级坐标逐字相等**,同域前提零见证(仅 LARG-C 经验证实、且仅对捆绑版 trace_streamer 成立;darwin/slim 外部 streamer 不受 pin)。时钟域偏移时全量事件**双份**注入且 `duplicate_events=0` 假完整声称。否证席注记:精确矛盾信号(db>0∧enriched==0∧raw>0)也有合法成因,只可作软披露不可作硬门 | 静态 |
| T2-1 | `cmd/trace_convert_diagnostic.go:417` | 8192 字节截断的 sideband 保留键表是**手抄分叉**,已落后 hitraceconv 生产者 ≥4 个见证族,无双向结构测试(五表手抄病根重现) | 静态 |
| T3-3 | `tree.go:5604` | causalUnproven 降级只改 lead 行与图例,多板形下其他板主席明细仍写 `主根因(优先处理)`——同页词面混用且该词定义已被移除(与 T3-1 同批修) | 静态 |
| T4-1 | `emit_log_triage.go:496` | 三个新 verbatim 硬拒门对 **raw** AttachedLog 做 Contains,而模型能读到的一切面都先过 normalize(CRLF→LF 等)——Windows 客户 CRLF 日志上模型忠实转录漏斗原文仍被硬拒(硬门两侧表面不同源) | 静态;logtri eval PASS 不构成反证(仅在含 CR 段转录时触发) |
| T5-2 | `emit_investigation_complete_relation_canonical.go:29` | B18d 关系源身份键**只有名字没有文件轴**(member 有),同名异文件 relation source 折叠、第二行 roster 静默丢——违背其自己账本第 4 点"同名不折叠 fail-open"声称 | 静态 |
| T8-1 | `tracequery/query.go:891` | 新 EventSearchCoverage 对 line-window 查询发布**全索引包络时间窗 + scope_complete=true**,而 matched 只计 line 窗内——padding 边缘含已解析被排除事件时,完整枚举声称过界(Description 恰教模型 follow-up 用 line 窗) | 静态 |
| T9-1 | 同事 eval 账本 | 自报未清账约 30 项,其中 **EVAL-B1-R2/R3(P0,已施工、十次回放均未触发、账本明令不得声称关账)**;B1-T6/W3、B46-T12c 等 P1 待施工 | 账本原文 |
| T9-3 | 同事 eval 账本 §B26-OWN | 自记录的红线违规窗:四笔提交(06-29→08-01)让持久层**删除模型 blocks 并以系统文案顶替**(违「系统不可代替 LLM 写用户面板答案」),已在两个 choke point 收口;前向风险=收口外新增 supplement 通路复发 | 账本+代码收口已验 |

## §4 存疑(PLAUSIBLE,1/2 否证席分歧)

- **T6-2** `emit_analysis.go:1303`:三个新必填 profile 各带独立单因硬拒 + Execute 保持先败先返(~35 处)→ 每轮只暴露一个缺陷,疑似回到 EMITBURN-1(§29.173)修掉的逐轮烧重试形。分歧点:部分拒答面有聚合;需逐面清点后定性。
- **T7-2** `test_surface.go:188`:meta-runner 声明覆盖 roster 第三臂把直调脚本里**被引号引用的任意在库路径字面量**(日志文案/skip 表)当覆盖权威——`SKIP=["src/util.py"]` 反而让 util.py 记为已测。分歧点:该臂是否另有交叉门拦截。

## §5 低危顾问(12 件,未进否证轮,修复时顺带)

T1-2 越界时间戳有效 CPU 记录整丢不 fence(违其自documented合同);T2-2 86 项能力表自指 pin(行为回退不红);T2-3 alias 门只查 3/6 发布路径+路径等价弱分叉;T2-4 新诊断面在 CLAUDE.md/architecture.md 零文档(--tracediag 有而 --diagnostic-report 无);T3-4 目标状态卡 4 行静默截断无 +N 披露(自称权威 roster);T4-2 perf-triage prompt 泄漏 validator/downstream 管线机制词(行话门盲区);T5-3 relation_claims 每轮只报第一违规且缺字段值细节(EMITBURN 纪律反向);T6-3 git ls-files 车道与 walk 回退解析栈帧的文件宇宙不同(vendor/ 双解);T6-4 跨批 owner 锚无陈腐臂(后批 apply 改写文件后旧锚仍作证);T7-3 durable plan 快照缺失时 bare continue 静默收窄重建域;T9-2 转换账本冻结的 typed 保真边界(24,055 无 CPU 证明 sync span 等)是**防伪造承诺面**,后来者勿"修复";T9-4(正面)抽检 9 条账本声称全实。

## §6 处置建议(排期序,均未动码)

1. **P0 批**(机械,先行):tracediag 双红修复——EventSearchCoverage 逐字段渲染处置+哈希重钉+定点格式;同批把 T8-1 的 line-window scope 过界一并修(同一结构)。
2. **裁定请求**(T3-1):加冕降级的触发信号(会话 sticky ANY → 席位级精确)与词面(备案词族启用+定义括注移除)提请用户裁定;T3-3 同批。
3. **高危机制批**:T3-2 回边豁免/关系类扩臂;T7-1 restore 车道 stamp 语义修。
4. **中危批**:T4-1 同源化(硬门谓词对 normalize 后表面)、T5-2 源身份补文件轴、T1-1 软披露信号接线、T2-1 键表双向 pin。
5. **继承债**:T9-1 清单并入排期(EVAL-B1-R2/R3 回放最优先);T6-2/T7-2 逐面清点定性。

**方法学记录**:三镜头独立汇合(T3-2/T5-1/T6-1)再次验证多镜头审计对"硬门-教学面自相矛盾"类缺陷的判别力;同事账本抽检零虚账,其"状态:"行可信度高,后续审计可降采样。

---

## §7 T3-1 裁定落案(2026-08-02,用户批复「同意裁定」;仅方案,未动码)

### §7.1 问题的虚拟例复述(施工验收即以此为红绿判据)

会话:「主线程这帧卡了 128ms,根因?」

1. 第 1 轮 explorer 粗探 `wakeup_chain` 窗 `10.0..10.5s`(窗没对准)→ 零 typed 因果行 → 该次结果 `CausalConclusion="unproven"`(铸造点 `internal/tool/trace_query.go:750-756`,对该次查询诚实);
2. 第 2 轮定位真窗 `10.32..10.45s` 重跑 → 完整已证链 `worker-777` runnable 82.000ms,凭证齐全,加冕选举成立;最终投影完全编译自第 2 轮;
3. 渲染时 `runtimeTraceCoverageAuthority`(runtime.go:4071-4075)对**全会话**结果 OR——第 1 轮的 unproven 永久置真 `causalUnproven`,头行从裁定形 `**主根因(=已证链上单项最大可消除量):** worker-777 …` 被改写为 `**首要可消除候选(不等于已证帧因果):** worker-777 …`,定义括注与图例定义条被删,而其他板主席明细仍写 `主根因(优先处理)`(T3-3 同页词面矛盾)。

一次探索期失手探测永久降级证据完备报告的头行 = 嘈声信号驱动硬显示变更。

### §7.2 语义定位(本次裁定核心)

**B+ 定义括注只主张「已证链上单项最大可消除量」,从不主张帧因果**(closed matrix 反机理腔护栏:机理/因果主张只来自链/阻塞证据,加冕词不自生机理主张)。因此:

- **帧因果未证 ∧ 链凭证已证** → **补限定披露,不摘冠**(裁定①);
- **链本身未证/无凭证席** → 走既有「窗口内未定位」不加冕车道与「无已证链上可消除量,主根因不加冕」板注,**不新造第三词形**;
- 同事降级臂词形「首要可消除候选」启用了 §17 备案保留的「首要可消除…」词族——**按备案纪律退役**(裁定②)。

### §7.3 施工方案(已批,待排期;修复时逐项过 pin)

1. **信号收窄到席位级(根修)**:废弃会话级 OR-sticky;投影**编译时**从最终加冕席自身派生——加冕选举成立 ⇒ 头行保持裁定词形字节不动(定义为真由选举构造保证);帧因果层单独判:仅当**最终投影消费的帧证据**上 frame-flow 未证(席位级 typed 信号)→ 头行追加限定注 `(帧因果未证)`(与 caliber note 同位),前缀与定义括注不动。
2. **词面退役**:`**首要可消除候选(不等于已证帧因果):** `、图例换写条(tree.go:2519)、lead 明细 `首要可消除候选(优先验证;帧因果未证)`(tree.go:5581)三处发射形全部退役;全发射面负 pin:`首要可消除` 不得出现(词族保护)。
3. **同页单源(T3-3 同批)**:头行/图例定义条/因果位置明细词三面出自同一词源函数;帧因果限定注出现时三面同步限定,禁手抄。
4. **Pin 面**:(a) 先-unproven-后-proven 会话 → 加冕头行字节恒等(§7.1 虚拟例即判据,先红后绿);(b) 帧因果未证+链已证 → 裁定词形+限定注臂(zh/EN 双面);(c) 备案词族负 pin;(d) 同页词面一致性 pin(多板形)。
5. **兼容注**:同事降级机制的诚实动机在 (b) 臂中保留——限定注继续披露"帧因果未证",只是不再冒称加冕定义失效、不再动用备案词。

### §7.4 施工结果(2026-08-02)

状态：`implemented / internal-tool-full-pass`。

- T3-1 根因确认，修复采用 §7.3 裁定：会话级 ANY-sticky 只留在覆盖边界披露，不再驱动加冕词面；
- 新的席位索引按 ledger 编译同构规则把 result authority 绑定到 observation ID，最终只消费当选
  lead 的 `EvidenceID + MergedEvidenceIDs`；链已证且该席位 frame authority 未证时仅追加限定语；
- `runtimeTraceProjCrownWords` 成为头行、图例、明细、对比列名和下一步称谓的单一词源，T3-3 同批关闭；
- “首要可消除候选”中英文生产发射全部退役；模型原答案 block wire 保持，系统没有替答；
- 新增先-unproven-后-proven、zh/EN 限定、退役词负 pin；`go test ./internal/tool -count=1`
  通过(187.077s)。

下一批：T3-2 sequence/call-dag 回复边与 alias 证据匹配合同。

---

## §8 T3-2 施工结果(2026-08-02)

原 finding `covered`，并由 B51 r4/r5 真回放扩出同批必要的 participant identity 半场：

- sequence `-->>` 按 parser 的 typed operator 识别为 response/return presentation edge，
  不再要求反向 call anchor；若模型显式声明反向 call anchor，该 anchor 仍须独立同向证据；
- `->>` invocation 与 call_dag 的每条边继续 hard-check，未降低调用方向真实性；
- 类/actor participant 到方法级 evidence 的兼容不是模糊匹配：owner 逐字相等 + message
  operation 逐字等于 typed AnchorSymbol/Object terminal + 候选调用边唯一，歧义 fail-closed；
- canonical diagram contract、skill 教学和 reject fix hint 已同步，不再一边教 reply 一边拒 reply；
- 新增 reply 正/负、Java 多方法 disambiguation、ambiguous fail-close 等 pins；
  `go test ./internal/tool ./internal/skill -count=1` 通过(tool 175.833s，skill 0.508s)。

状态：`implemented / relevant-full-tests-pass`。下一批按 §6 处理 T7-1。

---

## §9 T7-1 施工结果(2026-08-02)

原 finding `covered`，并补上了确定性红绿见证。根因不是累计域本身的合并算法，
而是 restore 车道把 `priorPlan` 的同一对象重新装回 active 后，authority rebuild 在读取
其既有控制器快照前先清空字段；restore cutoff 前仍在 worktree 中的旧批次只能从这份
传递闭包抵达终验，因此会静默漏验。

修复采用精确对象身份，不放宽 planner 权限：

- `candidate == plan` 是 scheduler 恢复既有计划对象的精确信号；仅该对象的既有
  `CumulativeVerificationScope` 会在清空前复制并作为 rebuild 种子；
- 只同 ID、但对象不同的新 planner 计划仍先清空累计域，不能伪造旧批次路径、行为合同或探针；
- 恢复的累计域仍只供 verify/final proof 消费，不进入 active apply target，未扩大改动范围、
  风险门或审批门；
- 新增单元 pin 覆盖 restore cutoff 前的 source plan/path/contract/probe 闭包，以及完整
  `replan -> no-plan probe pass -> restored verify` 车道；既有 no-change sentinel restore
  与 planner-injected scope 负例同时通过。

验证：`go test ./internal/orchestrator -count=1` 通过(10.479s)。

状态：`implemented / full-package-pass`。下一步复核 §1 P0 与 T8-1 在审计基线之后是否已被
后续提交覆盖，再按当前代码事实排中危批次。

---

## §10 审计基线校准 + T5-2 施工结果(2026-08-02)

### §10.1 §1 P0×2 与 T8-1 已被后续批次覆盖

审计结论在 `main=e479df784` 时准确，但当前主线已由 `a064e2d33` 覆盖，不再开放：

- `EventSearchCoverage` 已由 event-search key-first header 独占渲染，反射明细副本清空；
- 秒坐标统一固定小数格式，大时间戳不再泄漏科学计数法；
- 四个 R2' schema pin 均有逐字段处置说明后重钉；
- indexed line-window 的 scope envelope 从实际选中行域计算，不再冒用全索引
  `FirstTs..LastTs`；line 边界继续按与过滤器一致的优先级压过 time 边界。

复核：`go test ./internal/tracediag -count=1` 通过(7.331s)，现有
`TestIndexedEventSearchCoverageUsesLineWindowEnvelope` 固定 T8-1。

状态：`P0-1=covered / P0-2=covered / T8-1=covered-by-a064e2d33`。

### §10.2 T5-2 关系源身份缺文件轴

finding 准确，并由红测确定性复现：两个 `SourceName=Dispatch`、分别来自不同文件的
typed relation source，在旧实现中都映射为 `source|dispatch`，第二个主成员被静默删除；
`SourceFile` 缺失时，两个有不同 support file 的行也会仅凭名字被折叠。

通用修复位于 exact typed relation identity 铸造点：

1. relation source 与 member 一样使用 `normalized name + canonical file` 身份；
2. 聚合行有结构化 support file 时，member/source 都必须逐字匹配该文件；
3. typed source 缺文件轴、同名异文件或一行同时匹配多个身份时 fail-open，不猜测折叠；
4. 同名同文件的重复 observation 仍折为一项，Members/MemberNotes/SupportRefs 同索引投影；
5. 所有 relation kinds 共用，不扫描用户问题、模型答案或函数/文件特例。

回归覆盖同名异文件保留、同文件重复折叠、缺文件轴 fail-open；完整
`go test ./internal/tool -count=1` 通过(180.528s)，新增测试后相关组复跑通过(2.805s)。

状态：`T5-2=implemented / full-package-pass`。下一中危优先审计 T4-1 硬门两侧表面同源性。

---

## §11 T4-1 施工结果：verbatim 门与模型可见表面同源(2026-08-02)

finding 准确。附件进入 prompt 和 `attached_log.txt` 前会把 CRLF、裸 CR 规范化为 LF，
但 `emit_log_triage` 的 error-message 存在性、相同 message 基数、observation evidence
三类硬门都直接读取原始 `ctx.AttachedLog`。红测中模型从 prompt 忠实复制同一个 LF
多行片段，旧实现同时报 unobserved、observed=0 和 evidence unobserved。

修复不降低 verbatim 权威门：

- newline 规范化提升为 `textfmt.NormalizeAttachedArtifactText` 单一实现，context 的
  prompt/blob 渲染和 tool 的三类硬门共用；
- 只统一平台换行编码，不 trim、不做大小写、空白、Unicode、模糊或语义归一；
- 合成 message/evidence 仍被拒，重复 error cardinality 仍按规范化表面的精确子串计数；
- `RawLogBytes` 继续记录原附件物理字节数，bug-class 等其它消费者未改；
- 不扫描用户请求、模型 thinking/final answer，也不按语言或异常类型分支。

新增 CRLF→LF 多行 error×2 + observation evidence 的完整 Execute 回归，以及共享
normalizer 的 CRLF/CR/LF pin。验证：
`go test ./internal/textfmt ./internal/context ./internal/tool -count=1` 全绿
(0.709s / 1.630s / 175.061s)。

状态：`T4-1=implemented / relevant-full-tests-pass`。下一项按 ROI 审计 T2-1
诊断 sideband 保留键表分叉。

---

## §12 T2-1 施工结果：coverage witness sideband 单一命名合同(2026-08-02)

finding 准确。红测把现有生产键 `unknown_comm_witnesses` 放入一个超过 8 KiB 的
`TraceDBCoverage` 后，该 witness 只存在于被截断的 full line，cmd 手抄的 9 键表没有
发布独立 sideband；任意未来 family 也会重复此问题。

最优方案不是继续扩表，而是把 producer 的 witness 命名本身变成共享 typed policy：

- `hitraceconv.TraceDBCoverageDiagnosticWitnessKeys` 是生产包和 report adapter 的单一合同；
- 安全小写 ASCII key 以 `_witness` / `_witnesses` 结尾，或采用
  `_witnesses_<reason>` 形时自动进入 sideband，并按 key 排序保证确定性；
- `_emitted` / `_omitted` / `_cap` 等 witness accounting 不提升，避免行预算被计数副本消耗；
- 非法字符 key 不进入 line-name 位置，保留 JSON full coverage 的原有边界；
- 每条 sideband 仍受 8192-byte 单行界限，整报告仍受 900 行界限和 receipt 约束。

新增现有漏网 family、未来 reason family、accounting 负例和非法键测试；能力清单增加
`coverage_witness_key_convention_v1`。验证：
`go test ./internal/hitraceconv ./cmd -count=1` 全绿(130.346s / 9.090s)。

状态：`T2-1=implemented / full-package-pass`。下一项审计 T1-1 raw scheduler
补发的时钟域前提，只允许增补软披露，不能把不充分矛盾信号升级为硬门。
