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

- **T6-2** `emit_analysis.go:1303`:三个新必填 profile 各带独立单因硬拒 + Execute 保持先败先返(~35 处)→ 每轮只暴露一个缺陷,疑似回到 EMITBURN-1(§29.173)修掉的逐轮烧重试形。复核结论：profile 内部缺字段已有聚合，但 scope/target/question 三份独立声明之间确实先败先返；已按依赖边界收窄修复，见 §14。
- **T7-2** `test_surface.go:188`:meta-runner 声明覆盖 roster 第三臂把直调脚本里**被引号引用的任意在库路径字面量**(日志文案/skip 表)当覆盖权威——`SKIP=["src/util.py"]` 反而让 util.py 记为已测。复核确认后续没有纠偏门，成功 Make target 会把它升级为 static changed-path coverage；已修复，见 §15。

## §5 低危顾问(12 件,未进否证轮,修复时顺带)

T1-2 越界时间戳有效 CPU 记录整丢不 fence(违其自documented合同);T2-2 86 项能力表自指 pin(行为回退不红);T2-3 alias 门只查 3/6 发布路径+路径等价弱分叉;T2-4 新诊断面在 CLAUDE.md/architecture.md 零文档(--tracediag 有而 --diagnostic-report 无);T3-4 目标状态卡 4 行静默截断无 +N 披露(自称权威 roster);T4-2 perf-triage prompt 泄漏 validator/downstream 管线机制词(行话门盲区);T5-3 relation_claims 每轮只报第一违规且缺字段值细节(EMITBURN 纪律反向);T6-3 git ls-files 车道与 walk 回退解析栈帧的文件宇宙不同(vendor/ 双解);T6-4 跨批 owner 锚无陈腐臂(后批 apply 改写文件后旧锚仍作证);T7-3 durable plan 快照缺失时 bare continue 静默收窄重建域(已修，见 §16);T9-2 转换账本冻结的 typed 保真边界(24,055 无 CPU 证明 sync span 等)是**防伪造承诺面**,后来者勿"修复";T9-4(正面)抽检 9 条账本声称全实。

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

---

## §13 T1-1 施工结果：raw/DB 时钟对齐只作 typed 软披露(2026-08-02)

审计的风险方向准确，否证结论同样成立：DB 与 raw 都非空但没有 exact join，不足以证明
时钟域偏移；它也可能来自合法的不重叠事件、过滤、DB lane 抑制或 identity/state/key
不一致。因此本批没有新增去重/抑制 hard gate，也没有用时间近邻猜配事件。

落地的是可观测性闭环：

- raw join 在完成 DB boundary census 后统计 exact `timestamp_ns + CPU` 坐标交集，
  与完整 key join 分开；
- 两侧 admitted/audited 非空且交集为零时铸造
  `raw_db_time_alignment_observation=unproven_no_exact_timestamp_cpu_overlap`；
  有交集为 `observed_exact_timestamp_cpu_overlap`，空车道为 not-applicable；
- typed observation、raw retained、DB audited、overlap cohort 通过 scheduler reconciliation
  进入 semantic-quality；只在 unproven 臂发布 caveat；
- caveat 明示零交集**不证明 clock-domain offset**、可能是合法非重叠/过滤，且本观察
  `advisory only`，没有据此 suppress/duplicate/gate 事件；
- 原 exact key join、coordinate 去重、lifecycle authority、`duplicate_events=0` 发布路径
  均未改。

回归固定无交集只披露不报错/不改变 `RowsEmitted`，以及 quality caveat 传播；完整包首次
命中 same-input accounting golden，因为非适用 raw lane 也传播了一个新审计计数，随后将
传播严格收窄到 observation 实际存在的车道，golden 恢复字节不变。最终
`go test ./internal/hitraceconv -count=1` 通过(96.379s)。

状态：`T1-1=implemented-soft-diagnostic / no-unsafe-hard-gate / full-package-pass`。
真正自动消除跨时钟重复仍需要独立 typed clock calibration 证据；在该证据出现前不得猜配。

---

## §14 T6-2 施工结果：独立 runtime profile 错误同轮 census(2026-08-02)

审计结论部分准确，逐面复核后的精确边界是：每个 profile 解析器内部已经一次列出本对象
缺失的全部 required fields；但 `runtime_artifact_scope_profile`、`runtime_target_profile`、
`runtime_question_profile` 三份彼此独立的 typed request-authority 声明仍在 `Execute` 中
依次先败先返，同一 payload 的三个局部缺陷会消耗三轮重试。不能由此推导为约 35 个检查
都应该统一聚合：后续一致性门存在明确数据依赖，盲目收集会制造级联假错误。

通用修复位于三份声明的共同解析点：

- 三份 profile 先完成无副作用解析，再以 schema 顺序一次发布所有 independently actionable
  错误；没有读取用户问题、模型解释/答案或 case/type 特例；
- `runtime_targets` roster 若自身结构非法，仍与独立 scope/question 错误同轮报告，但跳过
  消费该 roster 的 target-profile 语义检查，禁止伪造“named target 缺 target”的二次噪声；
- 只有错误 census 为空才发布 soft normalization warnings；跨 profile 的 diagnosis consistency
  仍在有效对象构造后单独校验，不被硬化或吞并；
- profile 的 enum、verbatim quote、显式时间窗、target provenance、运行时问答宽度与后续
  Trace 因果投影/自动补齐权限均未放宽。

回归固定“三份空对象一次列全六个缺字段”和“坏 target roster + 坏 question 同轮报告、
无 dependent missing-target 噪声”。`go test ./internal/tool -count=1` 全包通过(160.919s)。

状态：`T6-2=confirmed-narrowed / implemented / full-package-pass`。下一项按原排期审计
`T7-2` meta-runner 对引用路径是否会误铸测试覆盖。

---

## §15 T7-2 施工结果：meta-runner coverage 只认声明边(2026-08-02)

finding 完全准确。确定性红测使用一个成功的 `make check` 脚本，其中
`SKIP=["pkg/widget.py"]` 和日志字符串都提及同一在库文件；旧 producer 把该文件放入
`DeclaredCoveragePaths`，后续 exact candidate/target success 虽能证明 Make 命令成功，
却无法证明该字符串是读取、断言或覆盖边，最终仍会把 changed path 签为 static covered。
没有其它交叉门检查脚本是否真实消费该路径。

最优安全边界是停止把任意语言脚本正文当声明语言：

- coverage roster 只保留 Make target 的 exact existing prerequisites 与 recipe 中 exact
  existing file arguments（包括被直接执行的测试脚本本身）；
- 不再打开并扫描 `.py/.sh/.rb/.js/.go` 等脚本中的引号字符串，因而 skip list、日志、
  fixture/example、dead branch 不再能铸 hard verification authority；
- 脚本对源文件的真实动态访问将来只能由 runtime file-access receipt，或能够证明执行与
  data flow 的语言级 typed 证据扩展；不能恢复成关键字/API/字符串启发式；
- ordinary same-language project-runner coverage 不变；跨语言 Make check 仍可通过 explicit
  prerequisite/recipe argument 获得 exact-member、source-static caliber，且不得扩到 sibling。

回归覆盖 `SKIP`/日志负例、prerequisite 多成员正例、部分 roster fail-closed、Python Make
和 Python 驱动 Rust check 的完整执行车道。`go test ./internal/tool -count=1` 全包通过
(161.874s)。

状态：`T7-2=confirmed / implemented / full-package-pass`。MERGE-AUDIT-3 两项 plausible
均已定性并闭环；下一批按 ROI 审计 `T7-3` durable plan snapshot 缺失时的 bare continue。

---

## §16 T7-3 施工结果：缺失 plan snapshot 的恢复 fail-closed(2026-08-02)

finding 准确。workflow run 的 durable envelope 只保存 `PlanID/PlanRef`、batch goal、attempt
与 artifact refs；原 plan 的 target paths、具体 edits、behavior contracts、verification probes、
approval record 只存在于 companion `<plan-id>.json`。恢复时找不到该文件，旧代码仅 warning，
然后仍允许 controller 从 batch goal 重规划；goal 为空时最终还会退化成
`continue active write workflow`。这些字段不足以无损重建原 mutation/verification domain，
继续就是静默收窄而不是兼容降级。

施工采用精确信号 fail-closed：

- 仅在 typed `workflow_resumed` run 的 active batch 已记录非空 PlanID、Mutable 没有同 ID plan，
  且 exact imported/report-dir plan artifact 载入失败时触发；长生命周期 Mutable 中的异 ID
  stale plan 不能冒充 active plan；
- 在任何 controller、planner、apply 或 verify dispatch 前返回明确 verdict，列出缺失 plan ID、
  无法恢复的字段族以及两条恢复路径：补回 artifact 后重试，或 clear workflow 后用完整原始
  change request 重开；不从 goal/progress prose 猜 scope；
- durable progress 追加 `resume_plan_artifact_missing`，但 run 保持 `in_progress`，避免 artifact
  补回后仍被 terminal status 拒绝；
- 正常 durable-plan hydration、failed-verify handoff、retry budget、pending approval 与 repair-plan
  deadline resume 正臂全部保留。

端到端红测旧实现会实际进入 controller；修复后断言 controller calls=0、typed progress 在场、
run 可恢复。最终 `go test ./internal/orchestrator -count=1` 全包通过(11.136s)。

状态：`T7-3=confirmed / implemented / full-package-pass`。相邻的“plan 在场但 needs-replan
report artifact 缺失”只损失失败细节、不丢原 mutation 域，当前仍为 best-effort；登记为
`T7-4/P2` 待独立审计，不在本批未经证明扩大 hard gate。下一高 ROI 项审计 T5-3。

---

## §17 T5-3 施工结果：relation claims 同轮精确错误 census（2026-08-02）

finding 准确。`ValidateAnswerRelationClaims` 对 claim 身份、关系、加法权限、成员集、subtotal、
单位和 closure 清单全部采用首错即返。一条 claim 同时写错五个独立字段时，模型必须逐轮修复；
其中 subtotal 缺失只返回泛化要求，没有同时展示实际缺失值与 typed 期望值。这与既有
EMITBURN“一次给出所有可独立修复项”的纪律相反。

修复保持 typed authority hard gate，不引入 prose 判断：

- 按 payload 顺序、字段顺序稳定收集所有 independently actionable mismatch；每项包含 claim
  索引、authority ID、实际值和 typed 期望值；
- authority ID 缺失、重复或未知时只报告该身份错误并跳过依赖字段比较，避免对不存在的
  authority 伪造 member/subtotal 级联；
- 已识别 authority 的 physical relation、addition、member set、subtotal value/unit 可在同轮
  并列报告；cross-ruler 的 subtotal 禁令也披露实际 value/unit；
- required-for-closure 的缺席项在 claim census 后全部列出；已出现但字段错误的 authority 不再
  额外伪报 missing；
- 输出最多展示 12 项并明确 `... and N more`，总违规数始终在首部；schema 既有
  `relation_claims.maxItems=16` 未放宽；
- validator 仍只消费 model-authored structured claims 与 system-owned typed authorities，
  不读取或改写用户问题、模型 reason/final prose，也不替模型生成关系结论。

新增九违规单轮回归，固定普通 subtotal 与 cross-ruler 两类 got/want、未知 authority 和遗漏
closure authority 同席发布。验证：

- `go test ./internal/types -run 'TestValidateAnswerRelationClaims' -count=1`：通过（1.029s）；
- `go test ./internal/types ./internal/tool -count=1`：通过（21.774s / 163.137s）。

状态：`T5-3=confirmed / implemented / full-package-pass`。下一项按 ROI 审计 T6-4
跨批 owner anchor 的陈腐权限。

---

## §18 T6-4 施工结果：apply 后按 changed-path 刷新 owner authority（2026-08-02）

finding 准确。近期为了让 cumulative review/repair/后续 slice 能复用同一路径的 durable owner
anchor，`LocalizationRequirementsFromWritePlanContext` 有意取消 transient batch/slice 过滤；但
apply 成功后没有生产代码设置 `WriteContextItem.Stale`。因此后批改写、改名或删除 owner
边界后，前批 line/symbol anchor 仍能满足后续 localization hard authority。

修复保留正确的跨批复用，只撤销被 mutation 精确触及的证据：

- post-apply 在 actual patch review 已铸造 `PatchEffectRecord` 后，从 effect 的 old/new path、
  `AppliedPaths` 和 `FileChange.Apply.Status=applied` 合成稳定 changed-path 集；不采用 plan prose、
  goal 或路径猜测；
- 先把这些路径上既有 typed localization anchor 标为
  `stale=true / stale_reason=source_path_changed_after_owner_anchor`，再从当前 worktree 内容和
  actual diff hunk 重建 replacement owner anchor；
- 删除文件、无法再解析 owner 或边界已消失时没有 replacement，后续 requirement 自然 reopen，
  不沿用旧 owner；owner 名相同也必须由当前 worktree 重新定位后才恢复权限；
- 未改路径的跨批 owner anchor 保持可复用，避免退回“每批重复探索”；rename 的 old/new 两侧
  都进入失效集；
- `OwnerAnchorViewFromWriteContextPack` 显式拒绝 stale anchor，包括通常跨 scope 可见的 P0 item；
  P0 普通约束的生命周期规则不变，防止高优先级包装绕过精确撤权。

回归构造前批 P0 `old_owner`、未改 `helper_owner` 与后批实际 patch 后的 `new_owner`：旧锚陈腐且
不再进入 authority view，新锚满足下一批 requirement，未改锚仍可见。验证：

- 定向 owner refresh/view tests：通过（types 1.333s / orchestrator 0.845s）；
- `go test ./internal/types ./internal/orchestrator -count=1`：通过（22.007s / 10.134s）。

状态：`T6-4=confirmed / implemented / full-package-pass`。下一项按 ROI 审计 T1-2
越界时间戳记录的 fence/保真边界。

---

## §19 T1-2 复核结果：越界时间戳已有 typed 拒绝与配对 fence（2026-08-02）

原 finding 经生产路径复核后不成立，不应修改 renderer 放宽时间域：

- profiler envelope 的物理 timestamp 是 uint64，但 systrace sorter/renderer 的统一时间轴是
  signed int64 ns；`timestamp > MaxInt64` 不能无损表示。截断、回绕、钳到 MaxInt64 或改写为 0
  都会伪造排序/时长，正确行为是该 row 不发布；
- 拒绝不是静默：fixed event diagnostic ledger 发布 `RowsRead=1 / RowsEmitted=0`、
  `envelope_timestamp_out_of_range` occurrences/affected-frames 与有界 reason sample；因此“整丢且
  无披露”不符合当前代码；
- 对 F2FS/Block 配对事件，typed payload 与 exact lane 在 envelope 最终判决前解析；时间戳 hard
  reject 分支调用 `pairDelta.poisonAdmission`，已知 key 关闭精确 lane，未知 key 关闭 family；MMC
  采用 whole-family poison。其前后合法 endpoint 不会跨洞配对；
- 普通 instant/core row 没有跨 row lifecycle 状态，不存在可关闭的 pair lane；把单个不可表示
  timestamp 升级为 source-global fail-close 只会无依据删除同 frame 的其它合法 CPU 记录。既有
  `SourceFailClosed=false` 正是隔离损坏行而非扩大证据损失；
- 不能把 uint64 timestamp 输出为注释型伪事件：那既不恢复原事件，也会向通用查看器引入一个
  不属于原 trace 的时间点。精确 coverage 是该类不可发布值的正确保真面。

新增生产链回归把 `MaxInt64+1` 的合法 F2FS end payload 放在合法 begin/end 之间，固定 overflow
row 精确诊断、exact-lane poison、两条合法 endpoint 全部 withheld、其它 family 不升级。
`go test ./internal/hitraceconv -count=1` 全包通过（96.130s）。

状态：`T1-2=disproved / no-production-change / regression-pinned`。下一项按 ROI 审计 T3-4
权威目标状态卡的截断披露。

---

## §20 T3-4 复核与施工：把分区 cap 接到主值状态卡（2026-08-02）

原 finding 的症状成立，但需要修正机制描述。主值卡内部确有一条
`additional target-state account(s)` 分支；然而上游
`CompileTraceCausalProjectionSet` 已先把 artifact partitions 截到 4 个，因此
`len(states) > 4` 在生产构造上不可达。真正的 typed 截断信号已经存在于
`TraceCausalProjectionSet.OmittedArtifactLabels`，因果投影答案块也会披露它，唯独模型成文前看到的
principal-value recap 没有消费该信号，于是它仍会把最多 4 行误读为完整 roster。

修复只补完整性元数据，不改变任何账户、选举或结论：

- principal-value recap 直接消费 projection set 的 typed omitted-partition census；
- 有截断时发布 `principal_state_roster_coverage`，明确
  `visible_accounts`、`additional_artifact_partitions_omitted`、
  `status=capacity_truncated`、`complete=false`；
- 由于被 cap 的 partition 从未编译，不能猜其中一定有多少 state account，故明确
  `omitted_state_accounts=not_evaluated`，不伪造“总状态账户数”；
- 即使可见 state row 为 0，只要 typed cap 信号存在也保留 coverage 行；不扫描用户/模型文本，
  不拒绝或改写答案，不触碰显式时间窗、因果投影和自动补齐。

新增 5 个独立 trace artifact 的生产链回归，固定 4 个可见账户与 `+1` 分区截断披露。

验证：定向生产链回归与 `go test ./internal/agent -count=1` 全包通过（2.643s）。

状态：`T3-4=confirmed / implemented / full-package-pass`。下一项按 ROI 审计 T6-3
两条源码文件宇宙不一致。

---

## §21 T6-3 施工结果：bare 栈帧 basename 两车道同一文件宇宙（2026-08-02）

finding 准确，范围可进一步收窄到 `analysis/logtriage.GlobByBasenames`：

- 无 Git 时的 filesystem walk 会跳过 `vendor/`、`node_modules/` 和任意隐藏目录；
- 8 月 1 日加入的 `git ls-files --cached --others --exclude-standard` 快车道只尊重 gitignore，
  因而会接纳已跟踪的上述目录；
- 对只有 basename 的 Java/ArkTS/Cangjie/Kotlin 等栈帧，两条车道会得到不同候选 roster，
  vendor 同名文件可能进入排序并改变最终 resolved frame。

修复在 bare-basename resolver 内建立单一目录可见性谓词，git 列表与 walk 都消费它：

- `vendor`、`node_modules`、root 以下隐藏目录在两条车道一致排除；
- 仅收敛歧义 basename 的候选宇宙；栈帧已经携带精确 repo-relative/absolute path 时仍由
  `ResolveFrameFile` 验证，不把这一软发现策略扩成全局 deny；
- 不依赖栈帧内容、语言或具体文件名，不引入用户/模型文本硬门。

新增真实 git repo 回归：同时 track production、vendor、node_modules、hidden 四个同名文件，
git 快车道必须与旧 walk 车道一致只返回 production 文件。

验证：定向 git/walk parity 回归与 `go test ./internal/analysis/logtriage -count=1` 全包通过
（1.024s）。

状态：`T6-3=confirmed / implemented / full-package-pass`。下一项按 ROI 审计 T4-2
perf-triage prompt 的内部机制行话泄漏。

---

## §22 T4-2 施工结果：perf-triage 改用证据任务语言（2026-08-02）

finding 准确。`perf-triage-skill` 在两条关键 evidence-calibration 指令里使用
“The validator stamps ... / downstream must ... / downstream agents ...”描述内部流水线。
这些词不帮助当前模型完成抽取，还会诱导回答复述实现机制；既有答案行话检查也没有覆盖这一
prompt 输入面。

修复不新增用户/答案禁词硬门，也不放松任何 typed authority：

- jank 指令直接说明 `trigger_span/reason/tags` 是 model-extracted navigation candidates，
  不是 causal proof； measured slow interval 与候选保持分离，直到 deterministic trace_query
  证明连接；
- observations 指令直接说明其用途是定位 trace region，数值、scheduler class、mechanism、
  causality 仍以 deterministic trace_query 为准；
- 删除 validator/downstream 的组织结构叙述，保持原有提取字段、阈值、因果边界和工具 schema
  字节语义；静态 skill 测试同时固定正向任务语义与三项旧内部短语不回退。

验证：定向 authority guidance 回归与 `go test ./internal/skill -count=1` 全包通过（0.316s）。

状态：`T4-2=confirmed / implemented / full-package-pass`。下一项按 ROI 审计 T2-3
诊断目录 alias/path equivalence。

---

## §23 T2-3 施工结果：诊断 sideband 消费转换保留路径单源（2026-08-02）

finding 准确。`--diagnostic-report` 在转换前创建文件，但旧实现只与 trace input、systrace
output、显式 trace DB output 三个字符串比较；tracebundle、perftrace、自动 DB companion 没有
进入候选清册。比较本身仅 `filepath.Abs+Clean`，也无法识别尚不存在的输出经 symlink parent
映射到同一物理位置。

修复把权限收口到转换包的两个单源 API：

- `ConversionPathReservations(opts)` 按 route-neutral 方式给出 input、systrace、tracebundle、
  可用 perftrace、retained DB 与 DB companion 的完整保留路径；CLI 不再复制 sidecar 公式；
- `TracePathsAlias` 复用转换发布的 canonical path + `os.SameFile` 规则：存在文件比较物理身份，
  prospective leaf 解析最近存在祖先，Windows 保持 case-insensitive；
- diagnostic report 在 `O_EXCL` 创建前逐项避让清册。route 最终未使用某席也只会保守拒绝一个
  sideband 文件名，不会改转换结果或证据内容。

测试枚举清册每一席，并新增“output 经 symlink directory、diagnostic 用真实目录同名且两边 leaf
均不存在”的回归，固定必须在创建前拒绝。

验证：清册/软链接定向回归以及 `go test ./cmd ./internal/hitraceconv -count=1` 全包通过
（7.022s / 92.890s）。

状态：`T2-3=confirmed / implemented / full-package-pass`。下一项审计 T2-4 文档契约。

---

## §24 T2-4 施工结果：把 conversion diagnostic 写入日常与架构入口（2026-08-02）

finding 准确：CLI help 与测试已有 `--diagnostic-report`，但 `CLAUDE.md` 日常命令和
`docs/architecture.md` trace diagnostics 章节只写了转换后的 `--tracediag`，没有告诉维护者或
客户如何采集“转换本身失败”的一次性报告。

本批零代码改动：

- `CLAUDE.md` Build & Run 增加一条可复制命令；
- architecture §13.7.1 说明 success/failure 均写、900 物理行硬帽、单行上限、typed 内容边界、
  `O_EXCL` 不覆盖、六类 route-neutral 路径避让和 status-only 不可组合；
- 明确 diagnostic-report 是转换诊断清册，不是原始 trace；转换成功后的定向取证仍使用
  `--tracediag`，避免两个能力继续混称。

验证：`go test ./cmd -run 'TestTraceConvertCLIContract|TestTraceConvertDiagnosticReport' -count=1`
通过（1.320s）；文档命令与当前 Cobra 参数逐项冷读一致。

状态：`T2-4=confirmed / docs-implemented / targeted-test-pass`。下一项复核 T2-2 capability
清册自指 pin 的维护成本。

---

## §25 T2-2 施工结果：能力声明与本次执行证据分权（2026-08-02）

finding 的“自指 pin 不能证明行为”准确，但需收窄性质：86 项 capability 清册是构建产物的
协议/词表声明，不是本次转换实际走过这些路径的证明；静态清册本身不是转换保真 bug，也不应
为每个名字复制一套同源自证测试。

本批保留清册用于版本和客户环境比对，同时紧邻发布 typed authority：

- `scope=build_advertisement`，明确它只描述当前构建声明的能力词表；
- `proves_observed_execution=false`，禁止把名字在场升级为本次执行成功；
- 明列本次执行权限来自 provider decision、artifact、coverage、DB coverage 与 typed error 行；
- `capability_count` 从清册长度派生，防止声明和权限附注独立漂移。

这不改变转换、恢复、筛选或输出内容，也没有用能力名扫描用户/模型文本。测试验证 typed 权限
边界和实际清册长度，但不再声称静态 pin 能替代各生产路径自己的行为回归。

验证：capability authority 与 build identity 定向回归通过（1.250s），`go test ./cmd -count=1`
全包通过（6.452s）。状态：`T2-2=confirmed-as-authority-gap / implemented / full-package-pass`。
下一项独立复核 `T7-4` 缺失
needs-replan report artifact 的降级范围。

---

## §26 T7-4 施工结果：缺失 verify report 时保留有限 typed handoff（2026-08-02）

独立审计后，原“只损失失败细节、best-effort 即可”的判断为 partial：durable plan 在场确实保住
原 mutation/verification domain，因此不能照 T7-3 整体 fail-closed；但恢复态仍为
`needs_replan`，旧代码在 report 文件缺失时不创建 `VerifyFailureHandoff`，planner 会同时失去
失败 reason、attempt diff、test surface 入口以及“哪些详情未知”的边界。这是可见的修复证据
静默退化。

本批采用有限权限恢复，而不是扩大 hard gate：

- failed ChangeReport 可读、`Passed=false` 且 Report.PlanID 与 active PlanID 完全相等时，继续
  构造完整 report-projected handoff；新增 PlanID 合取，错报告不能为另一计划提供失败权限；
- report ref 缺失、文件不可读、报告却为 passed 或 PlanID 不符时，从 durable attempt 只携带
  plan/batch/attempt、typed reason、diff ref 与 surface ref；
- carrier 标记 `report_evidence_status=unavailable` 与精确 reason code；planner 头部降为
  `bounded durable evidence`，把 commands、failing tests、build errors、diagnostics、confidence、
  runner output 和 next-surface 明确列为 `not_evaluated`，禁止从 reason code 猜造；
- durable progress 写 `resume_verify_report_evidence_unavailable`。原 plan 域仍可恢复并重规划，
  不因辅助 report 丢失把整个 workflow 变成不可恢复 blocker。

回归覆盖完整 report、缺失 report 的真实 resume→replan→apply→verify 链，以及 durable plan
缺失仍 fail-closed 的相邻红线。验证：定向 orchestrator 回归通过（1.481s）；
`go test ./internal/types ./internal/agent ./internal/orchestrator -count=1` 全包通过
（24.536s / 2.926s / 14.504s）。

状态：`T7-4=confirmed / implemented / full-package-pass`。MERGE-AUDIT-3 中高危、两项 plausible
及本轮逐项复核的中低危生产 gap 已闭环；继承 eval 债继续由统一 campaign 按 ROI 回放，不把
低价值顾问项硬化为生产门。

---

## §27 T3-2 闭环后回放：reply 已修，限定方法端点仍有 typed identity gap（2026-08-02）

按统一 campaign 规则严格并行回放 `sr_java_call_chain` 与
`qf_called_by_typed_relation_query`。runner 均 PASS，但人工审计分别为 FAIL/PASS：

- called-by 在 108s 内一次成文、零 reject，2 个 production caller 清册正确；
- Java call-chain 从旧基线 26 rejects/11 patches 改善到 12 rejects/5 patches，证明 T3-2 对
  sequence reply `-->>` 的豁免已生效；但 6 轮成文仍全部被同一 edge authority 拒绝，300s 后
  发出 previous rejected draft 与 19.6KB raw finalizer thinking，不能算交付成功。

本轮完整核对 3459/2398 行日志、最终答案和 fixture 源码后，剩余机制收敛为另一条泛化 gap：

1. citable call evidence 精确记录 caller，但 callee 仅为 call-site 可见短操作名，例如
   `VisitController.create -> schedule`；
2. 同一证据池另有 citable definition `VisitService + schedule`；
3. 图为了表达每一跳使用 `VisitService.schedule`，旧 exact lane 不会把两条 typed 事实合成同一
   展示端点；模型改用 class participant、限定 method participant 或 alias 都无法闭合；
4. reply edge 已完全退出拒绝清册，故不是 T3-2 回退，也不涉及 Trace/root-cause diagram family。

施工采用 pair-atomic typed convergence，而不是方法名模糊匹配：

- 调用方向仍必须由一条 citable `ClaimCallEdge` 精确证明 caller；
- 仅当目标是 `Owner.Operation`、call Object 非空时完全等于短 `Operation`（Object 为空时才由
  完全相等的 AnchorSymbol 承载），且证据池中恰有
  一个 citable `ClaimDefinitionFact(Subject=Owner, AnchorSymbol=Operation)` 定义身份时，允许把
  短 callee 投影为限定图端点；
- call Object 为其它短名/已限定到其它 owner、定义缺失、owner 不同或同 owner+operation 存在多个定义位置
  时全部 fail-closed；definition 单独不能证明方向；
- 不读取 edge message 的模糊 token，不扫描 request/final/thinking，不改变反向边、reply 语义、
  `QFRootCauseTrace`、显式时间窗因果投影或自动补齐。

回归固定 unique-definition 正臂，以及 missing/wrong-owner/overload/contrary-qualified-object/
contrary-bare-object 五个反臂；原 reverse、reply、class ambiguity 和 Trace-family isolation pins 保留。

验证：定向 6 个新正反臂通过（1.117s），`go test ./internal/tool -count=1` 全包通过
（167.765s）。

状态：`T3-2=covered`；新增 `T3-2-ENDPOINT-IDENTITY=implemented / full-tool-test-pass /
same-pair-replay pending`。

---

## §28 B52 r2 与全语言调用图层审计（2026-08-02）

同 pair 在 `bf6114879` 上回放后，called-by 继续人工 PASS；`sr_java_call_chain` runner PASS
但人工 FAIL，仍有 12 次 `call_edge_unproven`、5 次 patch，并最终泄漏 rejected draft/raw
thinking。reply `-->>` 已完全退出拒绝面，故 T3-2 本体没有回退。

### §28.1 为什么 B52a 仍是 partial

r1 的 call Object 是裸 `schedule`，B52a 用唯一 citable definition 补成
`VisitService.schedule`；r2 的真实调用证据则是 `service.schedule`、
`config.resolveMaxVisits`、`repository.insert`。这不是裸 operation，旧补全臂正确地把它视为
“已限定到另一个 owner 的相反证据”，所以模型即使画出源码真实的限定方法端点仍无法通过。

若把 diagram matcher 放宽为“最后一段相同即可”，会把动态对象、重载和同名方法猜成任意类，
违背精确信号硬门。根修必须上移到 typed relation：静态声明唯一时把 receiver expression
解析为 receiver type，再由 Graph.ResolveCallTarget 得到定义端点；不能唯一解析时保留源码表达。

### §28.2 15 语言能力矩阵与冷读结论

| 语言 | 修复前调用图层 | 本批权限形 | 状态 |
|---|---|---|---|
| Go | call + 参数/receiver 类型 | 唯一类型 → 定义端点 | 既有 covered |
| Python | attribute 被压成裸方法 | 保留动态 receiver expression，不猜类型 | fixed |
| JavaScript | member 被压成裸方法 | 保留动态 receiver expression | fixed |
| TypeScript | member 被压成裸方法 | 唯一类型注解 → 定义端点；否则源码 receiver | fixed |
| Java | method invocation 无 receiver | field/param/local 唯一声明类型 → 定义端点；冲突保留源码 receiver | fixed |
| Kotlin | 无普通 call relation | 新增 call；唯一参数/属性类型 → 定义端点 | fixed |
| Rust | field/scoped call 丢 receiver | 参数/field 唯一类型 → 定义端点；associated type 原样保留 | fixed |
| C | 裸函数调用在场 | 保持函数调用；field-expression 保留 receiver | fixed |
| C++ | field/scoped call 丢 receiver，qualified_identifier 未覆盖 | 参数类型 → 定义端点；补 qualified_identifier | fixed |
| Ruby | 无普通 call relation | 新增 call，保留动态 receiver | fixed |
| Swift | 无普通 call relation | 新增 call；唯一参数/属性类型 → 定义端点 | fixed |
| Lua | 只识别 require/import | 新增 function_call，保留 `.`/`:` receiver | fixed |
| Proto | `rpc` 声明关系在场 | declarative RPC，不伪装成 executable call | N/A by design |
| ArkTS | TS 主路径有裸 call；post-pass 自身无 call | Tier 1 继承 TypeScript 类型端点；TS parser 不可用/超时的 Tier 2 不猜造 call | fixed / fail-closed |
| Cangjie | declaration parser 把函数体当 opaque | 同一 comment/string-aware token 流新增 call；唯一 `name: Type` → 定义端点 | fixed |

矩阵由 `SupportedReadLanguages()` 全量 pin：新增语言若没有明确落入 semantic/source/function/
declarative 之一，测试直接失败。Proto 的 declarative 身份有独立负 pin，禁止为凑齐矩阵伪造调用。

### §28.3 通用实现与不变量

1. `normalizeCallEvidenceDirection` 先消费同一 callsite 的 Graph.ResolveCallTarget；只有唯一解析成功
   才发布 `Owner.Operation`，否则依次回退源码精确 target 与 relation target；
2. 静态语言的名字→类型 census 全部采取“同文件同名声明必须一致”策略；冲突时删除类型权限，
   保留源码 receiver，不做最近声明/频次猜测；
3. 动态语言只补回被 extractor 丢掉的 receiver expression，不提升类型；
4. 新增关系均由 AST 或 Cangjie comment/string-aware token parser 产生；ArkTS parser timeout 路径
   不用 regex 关系参与 diagram hard gate；
5. QFCallChain 之外的 Trace/root-cause family、显式时间窗因果投影与系统补齐完全不进入该合同；
6. 不扫描 RawRequest、模型 prose/thinking 或 edge label 关键词，不把 noisy confidence 用作硬门。

验证：`go test ./internal/tool/repomap/index -count=1` 通过（1.367s）；call-direction/
diagram 定向工具测试通过（1.019s）；最终 `go test ./internal/tool -count=1` 全包通过（173.722s）。

状态：`B52a=partial superseded`；`B52b-CROSS-LANGUAGE-CALL-IDENTITY=implemented /
index-full-pass / tool-full-pass / exact-pair-replay pending`。

---

## §29 B52 r3：全语言端点闭环与成员轴/观测轴引用错配（2026-08-02）

在 `b50f49233` 上严格并行 2 个回放，runner 2/2 PASS，二者均为
`finalizer reject=0 / patch=0`。Java 见证中的四条 typed call evidence 已精确归一为：

- `VisitController.create -> VisitService.schedule`；
- `VisitService.schedule -> ClinicConfig.resolveMaxVisits`；
- `VisitService.schedule -> VisitRepository.insert`；
- `VisitRepository.insert -> AuditLog.record`。

因此 `B52b-CROSS-LANGUAGE-CALL-IDENTITY` 从 replay pending 更新为 `covered`。Java 最终答案
仍把 side branch 写成串行 hop、把内存 append/println 说成持久化，并出现“5 跳/6 行”矛盾；
这是模型对 typed 拓扑和用户前提的解释质量残余，不得由系统改写答案来掩盖。

同轮 called-by 暴露确定性系统 gap `B52c-MEMBER-OBS-AXIS`：模型完成态正确表达 2 个 caller
members，但 `support_refs` 忠实保留 3 个 call-site observations（同一 caller 有两处调用）。旧
接线只有 `len(refs)==len(members)` 的 positional repair；cardinality 不等时首成员失去逐行位置，
最终 mutation 又把第二成员 line 321 错配给首成员，并追加仅 1 项的系统清单，把正确模型答案
改成“2 项/1 项”自相矛盾。

施工冻结为通用 typed 归一，不识别 caller、函数名或语言：

1. 对 cardinality 不等的 member-set，把每个现有 ref 作为单成员 positional probe，只在它对
   grounded support index 精确解析到唯一 member 时取得权限；
2. 每个 member 选择原序最早的唯一匹配 ref，构造成一成员一主引用；其余观测仍完整留在
   Evidence Pool/relation lane，不删除证据；
3. 任一 ref 同时匹配多个 member，或任一 member 无精确 ref 时 fail-open，保持原 payload；
4. 已是 positional 的等长数组 byte-preserve，并继续由既有 swapped-ref repair 负责；
5. 全过程不读 RawRequest、模型正文/thinking，不改变 Trace 因果投影、显式窗或自动补齐。

回归覆盖多观测正臂、全歧义反臂、成员缺证反臂和既有 positional byte-preserve。

状态：`B52b=covered`；`B52c=implemented / directed-pass / full-tool-test-pass /
same-pair-replay pending`。

---

## §30 B52 r4：成员轴闭环，mixed call-DAG 把控制流误判为调用（2026-08-02）

`16e8c13b9` 同 pair 回放结果：called-by 106s、人工 PASS；Java 359s、人工 FAIL。

called-by 的最终主清单只有 2 个 caller，引用分别为 line 294 与 line 321；同一 caller 的 line 295
仍留在 evidence ledger，但不再占据第三个成员席，也没有系统追加 1 项清单。因此
`B52c-MEMBER-OBS-AXIS=covered`。

Java 此轮选择了比 r3 更丰富的 mixed call-DAG：三条真实方法调用边加一条
`VisitService.schedule -> Reject` 条件拒绝边，并为后者精确声明
`relation_kind=guard / claim_form=guard_condition`。最后一次草稿中三条 call edge 均已通过，唯一
拒绝是系统仍要求 guard edge 携带 call anchor。连续 12 reject / 5 patch 后降级出厂并泄漏约
21KB raw thinking。

登记 `B52d-MIXED-CALL-DAG-RELATION`（P1）。根因是
`DiagramCallEdgeEvidenceMismatches` 对 call-DAG body 使用“所有有向边都是 call”的旧默认，完全
没有消费已经存在的 typed edge relation；这与 relation legality 层“typed relation 优先于 label”
的架构相互矛盾。

通用施工规则：

1. 对 call-DAG 每条 parsed edge，先按精确 `(from_node,to_node)` 读取 typed edge-anchor relation；
2. 存在 `call` relation 时仍必须通过 grounded typed call evidence；
3. 仅存在 schema-valid 的非 call relation 时退出 call authority，由既有 relation legality 层验证
   guard/import/precedence/observe/contain；不读边文案关键词；
4. 无 typed relation、anchor 端点不匹配或 relation unknown 时保持旧 call fail-closed；
5. sequence 的 `->>` invocation 不因 guard anchor 放宽，`-->>` reply 豁免保持；
6. QFRootCauseTrace、显式时间窗因果投影和自动补齐继续完全隔离。

实现位于语言无关的 AnswerDocument contract 层，故同一规则覆盖全部 15 种支持语言，不建立
Java/ArkTS/Cangjie 分支。定向回归含 mixed guard 正臂、错端点 guard 反臂、sequence 反臂、
原 missing-anchor 红线与真实 pre-emit 接线。

状态：`B52c=covered`；`B52d=implemented / directed-pass / full-tool-test-pass /
orchestrator-diagram-pass /
same-pair-replay pending`。

---

## §31 B52 r5：mixed call-DAG 闭环与精确关系主席位竞争（2026-08-02）

在 `daf2c4268` 上严格并行 2 个回放，runner 均 PASS，且都为
`finalizer reject=0 / patch=0`。Java 图完整发布三条 typed call edge 与一条 typed guard edge，
证明 `B52d-MIXED-CALL-DAG-RELATION=covered`；答案对 hop 数、方法名和 println/落库的解释仍有
错误，归为模型证据消费波动，系统不得替模型改写结论或按答案词面加硬门。

called-by 的 direct caller 集仍精确为 2 项，但同一探索的第二次 completion 又携带一个“间接
upstream” member-set，未显式填写 `role`。精确 direct 集已经获得
`system:typed_relation_principal_member_set`，旧默认却让 omitted-role 兄弟集也取得 principal
席位，导致 finalizer 和两种系统 carrier 重复发布间接成员，并把 direct relation 的回答边界扩大。
登记 `B52e-TYPED-RELATION-PRINCIPAL-OWNERSHIP`（P1）。

通用施工规则如下：

1. 仅当某个 member-set 已通过 exact typed candidate 匹配并获得系统 principal provenance 时，
   它拥有默认 relation principal axis；
2. 同次 completion 中其它 role 未填写、且按旧默认会成为 principal 的 member-set 改为
   `supporting_coverage`，保留其 members、refs、notes 和模型辅助分析；
3. 模型显式填写 `role=principal_answer` 时字节级保留，系统不否决真正的多主席位判断；没有 exact
   typed authority 时所有 omitted-role 集保持原状；
4. 确权先于 relation source-scope 校验，防止辅助集在尚未分类前被误当第二主席位硬拒；
5. 判定只读 typed candidate identity、aggregate role 与系统 provenance；不读取 RawRequest、label、
   completion reason、模型正文/thinking，也不针对 called-by、Java 或具体函数拟合；
6. 全部 relation kinds 和项目支持语言共用；Trace/root-cause family、显式时间窗因果投影及系统补齐
   不进入该路径。

回归覆盖 implicit sibling 降级、explicit second principal 保留、无 exact authority 不介入，并增加
真实 `EmitInvestigationComplete.Execute -> StableInvestigationAggregateFacts` 接线 pin。

验证：定向含生产 `Execute` 接线回归通过（1.289s）；`go test ./internal/types -count=1`
通过（22.829s）；`go test ./internal/tool -count=1` 全包通过（167.698s）。

状态：`B52d=covered`；`B52e=implemented / directed-pass / full-tool-test-pass /
same-pair-replay pending`。

---

## §32 B52 r6：主席位无回退，已读调用边未完成证据交接（2026-08-02）

在 `fb3c2ffe5` 上严格并行 2 个回放：called-by 193s，runner/human PASS，最终只有 2 个 direct
production caller，零系统重复清单或 upstream 扩界。该轮模型没有再次发出 omitted-role sibling，
故 B52e 的真实答案只证明无回退；确定性触发由 production
`EmitInvestigationComplete.Execute -> StableInvestigationAggregateFacts` pin 验收。

Java 165s，runner PASS / human FAIL。模型读取全部 5 个答案文件，但没有为
`VisitController.create -> VisitService.schedule` 发射 citable call-edge，`countOpenVisits` 关系也只以
recovered/lead 形进入 finalizer。class-participant sequence 连续 4 次校验失败、2 次 patch 后被删除。
现有 matcher 已有“同一 class participant 间多条 exact message operation”正反 pin，因此这不是
Mermaid 表达器或 Java receiver identity 回退，而是更上游的 evidence handoff 缺席。登记
`B52f-CALL-EDGE-EVIDENCE-HANDOFF`（P1）。

本批采用 typed soft guidance，不放宽硬门也不自动猜造证据：

1. 只在 closed `QFCallChain` family 注入指导，不扫描用户请求或模型输出；
2. repo_map relation 仍是导航。只有源码行已经 read 验证后，才指导模型为每个 load-bearing direct
   invocation 发独立的 grounded relationship row，携带 exact caller/callee、call anchor、operation、
   source/line/snippet；
3. 同一 class/actor 端点间多个 operation 必须逐边交付，definition、read coverage、路径 member-set、
   closure prose 均不能替代方向证据；guard 与 call 分账；
4. 动态/歧义 receiver 保留源码 surface 并披露边界，不猜 owner；Proto RPC 保持 declarative，禁止
   为追求图形完整伪造成 executable call；
5. 规则覆盖 Go、Python、JavaScript、TypeScript、Java、Kotlin、Rust、C/C++、Ruby、Swift、Lua、
   ArkTS、Cangjie 等全部 executable source lanes；不要求必须画图，也不授权系统替模型下结论；
6. Trace/root-cause、显式时间窗因果投影和自动补齐完全不进入本指导。

状态：`B52e=implemented / production-pin-pass / r6-no-regression`；
`B52f=implemented-soft-guidance / directed-pass / agent-full-pass / same-pair-replay pending`。

验证：typed family 正/反定向测试通过（2.612s）；`go test ./internal/agent -count=1`
全包通过（3.445s）。

---

## §33 B52 r7：调用边交接闭环、Markdown 主载体重复与全语言图语义（2026-08-02）

在 `759f1c859` 上严格并行回放同一对高优先级用例，runner 2/2 PASS：

- Java call-chain 253s，explorer 已把四条 load-bearing direct invocation 全部作为 grounded typed
  call edge 交给 finalizer，method-qualified sequence 成功出厂，因此
  `B52f-CALL-EDGE-EVIDENCE-HANDOFF=covered`；
- called-by 322s，direct production caller 仍精确为 2 个，主席位没有回退；但模型已经给出完整
  两行 Markdown 表后，系统又追加同样两行的 ordered list，登记
  `B52g-PRINCIPAL-MARKDOWN-CARRIER-DUPLICATION`（P1）。

Java 答案仍有两类模型错误：消息标签把 caller/示例 payload 写成 callee 调用，以及把 stdout
表述为落库；首稿还把纯 capacity guard 画成 invocation self-edge，导致 10 次 reject / 5 次 patch。
这些错误不能通过扫描答案文字加硬门，也不能由系统改写模型结论。本批只增加 closed
`QFCallChain` 的 soft authoring guidance：调用消息对应 typed callee operation；参数、literal、selector、
operator 只能取自 cited callsite；纯 guard 用 `Note/alt/opt` 或 flow branch，不伪造 self-call；动态
dispatch 不猜 owner。

`B52g` 根因不是枚举缺失，而是载体选择只承认 `Items[].Label`，不承认已经通过 exact accepted-row
coverage 的模型 Markdown table。因此关系 label 分支找不到 primary carrier，又创建第二个系统清单。
通用修复：

1. Markdown table 只有同时满足 `surface_role=principal`、typed enumeration facet、并逐行覆盖该 fact
   的 accepted display rows，才能作为 primary carrier；
2. 命中后只在原表补 typed relation title/facet/claim annotation，不追加新 block、不改表格正文和模型结论；
3. 缺任一行、缺 typed 身份或跨 fact 只偶然提到成员时不命中，既有 missing-row/system carrier 继续生效；
4. 判定源是 typed fact/display rows/block annotations；不读 RawRequest、模型 thinking，不把消息 label
   文案变成新的 hard reject；
5. 规则位于语言无关的 answer compiler。Go、Java、Kotlin、JavaScript/TypeScript/ArkTS、C/C++、
   Rust、Python、Ruby、Swift、Lua、Cangjie 等 executable languages 共用；Proto/RPC、import、
   inheritance/implements、annotation 保持其 declarative typed relation，不伪造成 call；
6. QFRootCauseTrace、显式时间窗 Trace 因果投影和自动补齐不进入该 soft guide；枚举 carrier 修复只
   消除重复，不删除 accepted rows。

定向回归覆盖完整 Markdown 主表原位承载、缺行反臂、既有 structured principal list、Cangjie
package/class 多维表“成员只在详情列出现”反臂，以及全语言 soft guide 的语言矩阵/非结论权限边界。

完整回归：`go test ./internal/tool ./internal/agent -count=1` 通过
（tool 161.350s；agent 2.999s）。

状态：`B52f=covered`；`B52g=implemented / directed-pass / full-tests-pass /
same-pair-replay pending`。

---

## §34 B52 r8：全语言图语义通过，调用点引用归属错配（2026-08-02）

`51cb91ffc` 精确构建后同 pair 并行 2/2 PASS：Java 119s、called-by 155s。

图层目标通过：Java 最终 call-DAG 发布五条 exact callee-operation edges；capacity guard 没有再伪造
self-call；finalizer churn 从 r7 的 10 reject / 5 patch 降到 2 / 1。called-by 最终仅两个 direct
production caller，零第二系统清单。该轮模型选择 structured list，因此 B52g 的真实回答只证明无回退，
r7 Markdown 精确触发继续由 production 正反 pin 验收。

人工审计发现新确定性 gap `B52h-CALLABLE-LINE-CITATION-OWNERSHIP`（P1）：模型首稿给五个 hop
的 `citation_ref=0..4` 本来逐项正确；generic label/citation normalizer 把 `Owner.method:line` 当普通
symbol label，按 endpoint name 重绑。调用链中同一方法既是上一条 edge 的 object，又是下一条 edge
的 subject，故 `VisitService.schedule:21` 被错绑 Controller:18，`VisitRepository.insert:23` 被错绑
Service:21，并产生一条无谓 detach caveat。

通用修复：

1. 新增结构化 `qualified_callable:line` 解析；先剥离可选展示 qualifier，再以最后一个冒号解析行号，
   callable 仍必须通过 code-identity grammar；
2. 明确排除 `*.go/*.java/*.ets/*.cpp/*.rs/*.cj:line` 等真实 source-location label，防止文件路径被
   误判为 callable；
3. 载体匹配要求 citation 行号相同，并由该位置的 typed evidence `subject/owner_symbol` 或 canonical
   enclosing function 证明 callable owner；仅 method 名相似、上一跳 object 相同均不取得权限；
4. 唯一精确匹配时既可 byte-preserve 正确模型引用，也可修复错误引用；歧义/缺 typed owner 时
   fail-open，既有 hard alignment 继续负责；
5. 同一 helper 进入 normalizer、detach guard 与 pre-check，避免先保留后被另一阶段删除；
6. 不扫描 RawRequest、item prose 关键词或模型 thinking，不判断模型结论。该结构规则覆盖 Go、Java、
   Kotlin、JS/TS/ArkTS、C/C++、Rust、Python、Ruby、Swift、Lua、Cangjie 等全部 callable lanes；
   Trace 因果投影/显式窗口不进入本修复。

定向回归复刻 r8 五条 Java hop：正确 0..4 必须零改动/零 detach/零 pre-check hint；全置 0 时必须
恢复为 0..4。另有跨语言 callable grammar 与 source-file 反臂。

完整回归：`go test ./internal/tool -count=1` 通过（163.592s）。

状态：`B52g=implemented / no-regression-replay / production-trigger-pin-pass`；
`B52h=implemented / directed-pass / full-tests-pass / replay pending`。

---

## §35 B52 r9：call-chain typed 形状漂移与复合 guard 图表达（2026-08-02）

`820525016` 同 pair 并行 runner 2/2 PASS，但人工 0/2：

- called-by 的 closure 与 typed relation projection 都明确是 2 个 unique caller functions；模型 aggregate
  却把 3 个 callsite observations 当 members，最终重复列出同一函数；
- Java 首稿把 condition 内真实的 `countOpenVisits` 调用替换成抽象 `CapacityCheck` 节点，又让该节点
  “调用” post-guard insert；修补后直接删掉 `countOpenVisits` edge。最终图仍不完整。

登记 `B52i-REQCALLCHAIN-RELATION-IDENTITY`（P1）。根因是两个 typed selector 漂移：
`TypedRelationKindsForRequest` 已把 `AnalyzerHints.Kind=ReqCallChain` 映射到 exact called-by provider；但
`HasTypedRelationMemberSetShape` 还额外要求 `PredicateAxis=call` 或
`Predicates.IsRelationalLookup=true`。r9 analyzer 发出 `question_kind=call_chain`，却把后两者都留空，导致
同一请求“可以查询 exact relation、却不能按 exact relation 归一 member-set”。

通用施工：

1. `HasTypedRelationMemberSetShape` 直接消费 closed `ReqCallChain`，与 relation kind selector 单源对齐；
2. exact provider 仍必须存在，且 principal fact 每个 member 必须唯一匹配 exact member/source candidate；
   因此这不是“所有 call-chain 都硬去重”；
3. 重复 callsite observation 折到 unique candidate identity，首个 exact support ref 保留，证据池中的全部
   callsites 不删除；同名不同文件、source/member 歧义继续 fail-open；
4. 若 `member_notes` 数量不等于原 member 数，其位置轴未经类型证明；canonicalize 后清空这些 optional
   advisory notes，防止把第一函数说明错贴到第二函数。member/support/evidence 主事实不受影响；
5. 不读 RawRequest、fact label、reason、thinking 或 final prose，Go/Java/Kotlin/JS/TS/ArkTS/C/C++/
   Rust/Python/Ruby/Swift/Lua/Cangjie 等全部 relation providers 共用。

图表达继续走 soft guide，不增加 hard label gate：复合 guard 内若有 grounded invocation，保留真正
caller→callee edge，再单独用 Note/branch 表示 comparison；禁止抽象 guard 取代 callee，也禁止 guard
成为 post-guard operation 的 caller。系统不自动重画模型图、不接管结论。

B52h 本轮没有出现 `callable:line` item labels，故是 no-trigger replay；跨语言 deterministic
preserve/repair/source-path 反臂仍是其验收 authority。

状态：`B52h=full-tests-pass / deterministic-pin-covered / replay-no-trigger`；
`B52i=implemented / directed-pass / full-tests-pass / replay pending`；
`compound-guard-soft-guide=implemented / directed-pass / full-tests-pass / replay pending`。

完整回归：`go test ./internal/types ./internal/tool ./internal/agent -count=1` 分包通过
（types 19.776s；tool 159.961s；agent 3.266s）。

---

## §36 B52 r10：`::` 图边消失与 non-call anchor 绕过调用权限（2026-08-02）

`e23168999` 精确构建同 pair 严格并行 2/2 runner PASS：called-by 107s、Java 220s。

人工结果：called-by PASS。最终 principal table 只有两个 unique caller functions；同一函数的两处
callsite 保留为同一行详情，typed aggregate 已是 `value=2`、两 members、294/321 两 support refs，
无重复 system carrier、无短 `member_notes` 错贴。`B52i` production trigger 闭环。

Java 图恢复了真实 `VisitService.schedule -> VisitRepository.countOpenVisits` 调用，说明复合条件 soft
guide 生效；但该可视 call-DAG edge 只携带 `relation_kind=guard`，没有 `call` anchor，仍被硬校验放行。
进一步用跨语言矩阵构造反例时发现更深的共享解析 gap：`mermaidcompat.ParseEdges` 在整行第一个 `:`
处分割 sequence message；Rust/C++/Ruby/Cangjie 的 `::` 限定名会让 flowchart 行在箭头前被截断，
整条边不进入 typed 校验。

登记 `B52j-COLON-CALLDAG-RELATION-AUTHORITY`（P1），通用修复：

1. 先识别 Mermaid body family；只在 `sequenceDiagram` 中、且完成 arrow split 后，从 target 后分离
   message colon；跳过 `::` namespace separator；flowchart 节点标签中的任意 colon 保持 byte-exact；
2. QFCallChain + `call_dag` 中，如果同向端点已有 citable typed call-edge evidence，则可视边必须携带
   `relation_kind=call`；一个 `guard/import/observe/...` anchor 不能隐藏该 exact call；
3. 复合条件允许同一 pair 同时携带 call + guard，或更清晰地把 guard 画成独立 branch/Note；
4. 真正的 guard-to-outcome、import、precedence、contain、observe 边在没有 same-endpoint call proof 时
   继续合法，不被粗暴重分类；
5. hard gate 只消费 QF family、diagram kind、schema-validated relation enum、parsed endpoints 与 citable
   typed evidence；不扫描 RawRequest、edge label prose、thinking/final text，不由系统重画图或改结论；
6. QFRootCauseTrace、显式时间窗 Trace 因果投影和自动补齐继续在该 source-call 合同之外。

测试按仓库权威语言矩阵覆盖 Go、Python、JavaScript、TypeScript、Java、Kotlin、Rust、C、C++、
Ruby、Swift、Lua、ArkTS、Cangjie 的 executable callable surface；Proto 保持 declarative relation，
没有 typed call evidence 时不会被升级为调用。

状态：`B52i=production-replay-covered`；
`B52j=implemented / directed-cross-language-pass / full-tests-pass / replay-pending`。

完整回归：`go test ./internal/mermaidcompat ./internal/render ./internal/tool ./internal/agent -count=1`
通过（0.759s / 2.011s / 165.286s / 3.010s）。

---

## §37 B52 r11：principal path 已选调用边的图完整性（2026-08-02）

`c32510694` 精确构建同 pair 严格并行 2/2 runner PASS：called-by 103s、Java 139s。

人工结果：called-by 再次 PASS，`B52i` 连续两轮 production replay 均稳定。Java 中 `B52j` 已触发：
guard-only 版本被拒，最终保留 `VisitService.schedule -> VisitRepository.countOpenVisits` 的真实调用。

但最终 call-DAG 把 typed evidence 与模型 principal list 都已选择的
`VisitService.schedule -> VisitRepository.insert` 改画成 `capacity Check -> VisitRepository.insert`。
现有合同只验证“每条可视 call edge 有证据”，不验证“模型已选择的 principal path typed calls 全部可视”，
因此图仍可能用控制流节点替换实际 caller。

登记 `B52k-PRINCIPAL-CALL-DIAGRAM-COMPLETENESS`（P1），通用修复：

1. 仅在 QFCallChain 且存在 strict `sequence/call_dag` 时启用；
2. principal endpoint universe 只来自模型自己发射的 structured items：block 必须
   `surface_role=principal`、携带 `facet_id=principal_path_edge` 与 `claim_form=call_edge`；
3. 仅当 universe 内两个 exact code identities 之间存在 citable typed call-edge evidence，才要求图中有
   同向 parsed edge + `relation_kind=call`；这闭合的是模型已选集合内部一致性，不增加系统成员；
4. 只有一端在 principal universe 的 supporting/background call 不扩张图；identity 不唯一时 fail-open；
5. 不读取 item text、summary、RawRequest、thinking/final prose，不从语言关键词猜关系，不生成或重画图；
6. Go、Python、JS/TS、Java/Kotlin、Rust、C/C++、Ruby、Swift、Lua、ArkTS、Cangjie 共用；Proto
   declarative relation 与 QFRootCauseTrace/显式时间窗/自动补齐不进入本合同。

定向测试覆盖 r11 缺边、非 principal supporting-call 反臂以及 14 个 executable language identities。

状态：`B52j=production-trigger-covered`；
`B52k=implemented / directed-cross-language-pass / full-tests-pass / replay-pending`。

完整回归：`go test ./internal/tool ./internal/agent -count=1` 通过
（加入发布接线 pin 后最终 tool 复跑 156.126s；agent 3.004s）。

---

## §38 B52 r12：全语言图闭环通过，edge-shaped principal 载体补齐（2026-08-02）

`f50f21f85` 精确构建同 pair 严格并行 2/2 runner PASS：called-by 114s、Java 138s。

人工结果：called-by PASS，仍只有两个 unique production caller；同一函数的多 callsite 只作行内详情，
无第二 system carrier。Java PASS：最终 sequence diagram 完整保留五条 typed invocation，`-->>` 仅作
reply，容量判断使用 `alt/else`，没有 self-call 或抽象 guard 取代真实 callee。说明 `B52j/B52k`
production replay 已覆盖。

r12 同时暴露 `B52l-PRINCIPAL-CITATION-CALL-COMPLETENESS`（P1）：模型 principal ordered-list
中多行把 `label` 写成 `caller → callee`，而不是两个独立 exact code identities。`B52k` 正确地拒绝
解析模型 label/text 作为 hard gate，因此只从少数 node-shaped items 建立完整性 universe；edge-shaped
行可能绕过“模型已选择调用必须出现在图中”的闭包。

通用修复：

1. 继续只在 `QFCallChain + strict sequence/call_dag` 启用；Trace 根因、显式时间窗因果投影和自动补齐
   不进入该合同；
2. principal carrier 仍必须同时满足 `surface_role=principal`、`facet_id=principal_path_edge`、
   `claim_form=call_edge` 且使用 structured items；
3. 不解析 item label/text。改用 item 的 typed `citation_ref`，以 canonical file + exact call-site line
   反查 citable `EvidenceItem{claim_form=call_edge}`，从其 Subject/Object/AnchorSymbol 取得方向；
4. 同一 citation 命中多个不同调用方向时 fail-open，不猜测、不重画；同方向重复 evidence 去重；
5. 已出现的图边允许 exact method labels，也允许 class participant + exact message operation 的唯一
   typed 映射；回复边、声明、guard/import/observe 等 non-call relation 不被升级为调用；
6. supporting citation 不扩大 principal 图；定义引用不生成 call edge。系统只校验模型自己选择的
   principal typed call，不添加成员、不修改总结或结论；
7. 测试直接绑定 `repomap/types.SupportedReadLanguages()`：14 种 executable languages（Go、Python、
   JavaScript、TypeScript、Java、Kotlin、Rust、C、C++、Ruby、Swift、Lua、ArkTS、Cangjie）全覆盖；
   Proto 作为第 15 种声明式读语言有明确反臂，禁止从 definition/declaration 伪造 invocation。未来新增
   可执行语言而未补图语义 fixture 时测试直接失败。

r12 另记录独立 advisory `B52m-RELATION-ROW-CITATION-ALIGNMENT`：Java h3 显示
`VisitService.schedule → ClinicConfig.resolveMaxVisits`，却引用 line 18 的 `countOpenVisits`；pre-emit
已经依据 typed role/evidence 精确指出应为 line 17，但 soft advisory 没有阻止发布。核心调用链与图仍
正确，故该轮 human=PASS with caveat。该项不是图缺边；后续只允许修 citation metadata，不得由系统
改写模型结论或用 label/原文关键词建立 hard gate。

状态：`B52j=production-replay-covered`；`B52k=production-replay-covered`；
`B52l=implemented / supported-language-registry-pinned / directed-pass / full-tests-pass / replay-pending`；
`B52m=filed-advisory / implementation-pending`。

完整回归：`go test ./internal/tool ./internal/agent -count=1` 通过
（tool 161.590s；agent 2.725s）。

---

## §39 B52 r13：全语言 citation-call 闭包真实触发验收（2026-08-02）

`73f15610c` 精确构建同 pair 严格并行 2/2 runner PASS：called-by 91s、Java 103s。

人工结果：called-by PASS，两个 distinct production callers 与 typed roster 一致。Java PASS：首稿图遗漏
`schedule -> countOpenVisits`、`schedule -> insert`、`insert -> AuditLog.record` 三条模型 principal
列表已经选择、citation 已绑定的 typed calls；`B52l` 新门逐条发出 `principal_call_edge_missing` 并拒绝。
模型一次 patch 后最终 flowchart 完整保留五条 invocation，容量 guard 不再充当 caller。finalizer rejects
由 r12 的 4 降至 2，证明该修复不是只在 synthetic fixture 生效。

语言覆盖裁定：仓库权威 `SupportedReadLanguages()` 当前 15 种。Go、Python、JavaScript、TypeScript、
Java、Kotlin、Rust、C、C++、Ruby、Swift、Lua、ArkTS、Cangjie 共 14 种 executable lanes 必须通过同一
principal citation-call fixture；Proto 走 declaration-no-call 反臂。图验证是语言无关 typed authority，
而不是按 Java/ArkTS/Cangjie 关键词分别拟合。

非图层残余分账：

- `B52m` 仍开放且范围更清楚：一个 principal item 可能同时描述 caller→callee 与 callee 内部行为，
  单一 `citation_ref` 只能证明其中一侧。最优方向是 typed per-claim/multi-citation carrier 或引导模型拆项；
  不得解析模型 prose 后硬改引用或替换结论；
- `B52n-NODE-EDGE-CARDINALITY-WORDING`（advisory）：typed aggregate 是 6 nodes，模型写“6跳”。
  当前没有 typed noun-axis 字段，不能扫描中英文“跳/节点”词做 hard gate；保留为模型措辞波动；
- `AuditLog.record` fixture 实际 `System.out.println`，答案沿用户“审计落库”措辞继续称落库。没有
  typed sink-persistence 证据，系统不得自行改写结论；可由后续模型证据引导改善。

状态：`B52l=production-trigger-covered / closed`；
`all-supported-language-graph-matrix=covered`；
`B52m=filed-advisory / carrier-design-pending`；`B52n=model-wording-advisory`。

全语言基础设施回归：`go test ./internal/tool/repomap/... ./internal/mermaidcompat ./internal/render -count=1`
全部通过（含 language registry、parser/extractor、relation/retrieve、Mermaid compatibility 与最终 renderer）。

---

## §8 MERGE-AUDIT-4 增量审计(2026-08-03):45 笔新合入 = 修复响应波 + 跨语言调用链战役

**范围**:73e93b2fa..HEAD 非本席合入 45 笔(08-02 19:23 → 08-03 03:52)。两簇:①对 MERGE-AUDIT-3 findings 的修复响应(~28 笔,提交名与 finding 一一对应);②全新跨语言调用链/调用图战役(~17 笔,含 repomap 12 语言提取器 +966 行、新 `extract_navigation_calls.go`、eval 跑批器改造)。**方法**:8 主题读者+逐条 2 席否证(34 agent,wf_be3f6487-29d)+全仓基线实测+高危亲验;只审计不修码。

### §8.1 基线与修复合规总评(正面)

- **main 复绿**:P0-1/P0-2 已修——四 struct 哈希重钉且**逐字段带渲染处置**(非盲重钉),科学计数法改 `strconv.Fo…` 定点单点;全仓套件零失败(本席实测)。
- **30 项修复合规通过(fixed_ok)**:MERGE-AUDIT-3 的确认/存疑/低危项几乎全数被正确修复,其中亮点:T7-2 直接**删除**引号字面量臂(最干净形);T4-1 三硬门全部改测 normalize 后同源面;T5-2 源身份补 `source|name|canonicalfile` 文件轴;T6-2/T5-3 违规聚合一次报全;T1-1 按否证席要求落**软**披露(typed metadata,非硬门)。
- **§7 T3-1 裁定合规**:五项中 ①席位级触发(`buildRuntimeTraceProjectionSeatAuthorityIndex` 按 unproven 结果证据键控,先探后证虚拟例 pin `TestEarlierUnprovenProbeCannotDecrownLaterProvenSeat` 在案)②词族退役(生产+负 pin)③前缀字节恒等+限定注并行④三面单源 全部落地;pins (a)(b)(c) 齐。**唯一缺口=R1-1**(见 §8.3)。
- **红线自纠**:同事整体删除了其旧战役的 `normalizeCallChainReachabilityAuthority`(系统改写模型结论块=「系统不可代替 LLM 写用户面板答案」违规)——连同测试一并退役。
- **eval 跑批器改动(a33febe7b)核验为提杆非降杆**:`EXPECT_PRIMARY_*` 严格 opt-in,仅 sr_java_call_chain 一例迁移,主体域断言更严;runner_lib_test 钉住新域切分。

### §8.2 高危确认(5 件,均双席否证幸存)

| # | 位置 | 问题 | 亲验/复现 |
|---|---|---|---|
| R2-1 | `answer_document_diagram_evidence.go:170` | **主对完备性车道绕过别名解析**(T3-2 同类第二例):`visibleCallSymbolPairs` 用裸小写标签键比对,而兄弟门走别名/定义解析——教学形 class participant(`participant C as VisitController`)标签与 principal 项标签形不同即硬拒 `principal_call_edge_missing`,拒的是图里**画着**的调用 | 否证席 go test -overlay **可执行复现** |
| R6-1 | `answer_document_diagram_evidence.go:362` | **call_dag 非调用锚免检臂被归一化器自铸锚击穿**:pre-emit 先跑 `normalizeDiagramEdgeAnchorMetadata` 自动铸 Guard 锚,后跑免检判定——虚构调用边借系统自铸锚逃过 typed 调用证据契约(**硬门反向失效**:该门要防的恰是虚构边) | 静态(调用序逐点核) |
| R7-1 | `repomap/index/cache.go:229` | **12 语言提取器输出语义变更零 `extractorVersions` bump**(cache.go 停在 07-09):暖缓存库 Kotlin/Swift/Ruby/Lua/Cangjie 零调用边、typed receiver 全缺,与新能力矩阵测试声称直接矛盾,静默陈腐 | **本席亲验**(git log 双向确认) |
| R7-2 | `repomap/index/extract_java.go:329` | **Java `var` 被当声明类型绑定**:`var x = new Foo()` 铸 x→'var',调用面渲染 `var.run`,不同 receiver 折叠进同一伪身份(Java 10+ 遍地 var) | 否证席对 vendored 语法树实证 |
| R7-3 | `repomap/index/extract_c.go`(census 应用面) | **C receiver census 只扫参数声明却全文件生效**:局部变量被改写成不相关身份 | 静态 |

### §8.3 中危确认(7 件)

- **R1-1** §7 裁定 pin (d) 只交付了单板形——多板(multi-cluster)一致性 pin 缺失;当前多板行为经核**构造上正确**(seatAuthority 正确穿线),但同文件已存在 nil-authority 兄弟构造器,未来改动无红可踩(正是 T3-3 病类)。**§7 收尾件,补 pin 即关**。
- **R2-2** `diagram_evidence.go:84`:证据池存在反向 typed 调用时,豁免的 `-->>` 回边被"再捕获"回硬锚契约——已证反向调用反而使回复边被拒。
- **R2-3** `diagram_evidence.go:349`:回边豁免**只键在箭头拼写**——未证正向调用画成 `-->>` 即零锚零证逃逸(与 R2-2 同臂不对称,一收一放都错)。
- **M4-R3-1** `write_controller_scheduler.go:3476`:T6-4 半修——anchors 只在 run.ContextPacks 标 Stale,Mutable 侧孪生未标,同一 apply 事务内 AND-merge 把撤销冲掉。
- **R6-2** `pre_emit_check.go:2488`:callable:line 引用归属回退臂剥掉 owner 限定词比对——`A.create:N` 可"拥有"`B.create:N` 的引用。
- **R7-4** `extract_navigation_calls.go:97`:Kotlin/Swift 声明类型取全子树**第一个** type_identifier——注解/初始化器里的类型名可冒充声明类型。
- **R7-6** `extract_cangjie.go:111`:`ident : ident` 三元组全当声明绑定——具名实参 `f(width: w)` 铸 width→w 伪绑定。

### §8.4 低危顾问(10 件,修复时顺带)

R1-2 proven 臂明细词仍在单源外手写;R2-4 sequence 消息切分序改动后消息体内箭头串污染边解析;R2-5 新主图完备硬契约零 prompt 教学(教学-硬门不同步的反向形);M4-R3-2 material 页尾部 UTF-8 trim 二次方(2MB 载荷 REPL ~10s);R4-1 T2-3 残留(raw perf.data sidecar 族+序号变体不在保留 census);R6-3 canonicalizer 修剪 SupportRefs 无位置性守卫(MemberNotes 有);R7-7 CallersOfID 保守包含契约被裸源表达式 receiver 反转;R7-8 Lua 无空格免括号糖铸垃圾 callee;R8-1 snippets 域只经引用传递排除;R8-2 primary 域边界字面量未与渲染器发射 pin 双向绑定。

### §8.5 处置建议(未动码)

1. **高危批一(图表门,R2-1/R6-1/R2-2/R2-3 同文件同批)**:完备性车道与边门同源别名解析;免检臂改结构判定(回边须镜像已画正向边)并排除自铸锚;两个不对称一并修。
2. **高危批二(repomap,R7-1..R7-6/R7-7/R7-8)**:先 bump `extractorVersions`(止血陈腐缓存),再修各语言 census 谓词(var/参数域/子树第一类型/具名实参)。
3. **§7 收尾**:R1-1 多板 pin + R1-2 单源归位,一小批。
4. **write 半修**:M4-R3-1 Mutable 侧同步标记。
5. 低危随批;R8-1/R8-2 在下一次 eval 战役排。

**方法学**:修复响应波的"提交名↔finding 对应"极大降低了核验成本,30/42 项一次通过;新战役(repomap census 族)则重现了"新谓词族未过既有纪律(缓存版本/教学同步/别名同源)"的复发类——**修复批的工程纪律显著好于新功能批**,后续审计资源应向新功能面倾斜。

---

## §9 MERGE-AUDIT-4 复核与施工进度（2026-08-03）

### §9.1 复核结论

对 §8 的生产代码、调用顺序和可执行反例重新核验后，R2-1、R6-1、R2-2、R2-3 均准确，且属于同一份
typed 图关系权限合同的三处分叉，不是 Java 或某个 Mermaid 模板的单点问题：

1. R2-1 的第一条 citation-carrier 车道已经复用 typed call resolver，但第二条 principal item pair 车道仍用
   `visibleCallSymbolPairs` 裸字符串键；因此方法级 principal 项和类级 participant 虽由同一 call evidence 证明，
   后者仍会被误判为缺边；
2. R6-1 的调用序成立：`normalizeDiagramEdgeAnchorMetadata` 先按 edge label 调
   `InferRelationFromLabel` 自行追加 Guard/Import 等 anchor，之后 call-DAG 门把该 anchor 当模型提供的 typed
   权限，形成“系统先铸权限、系统再验权限”的反向失效；
3. R2-2/R2-3 来自同一个仅看 `-->>` 拼写的豁免：证据池恰有反向 call 时回复边被重新捕获；而一个没有
   镜像正向 invocation 的孤立 `-->>` 又能自称回复逃逸。

### §9.2 M4-A 图表权限同源批（已交付）

- **R6-1**：归一化器只规范模型已经提交的 `edge_anchors`（节点别名、relation/claim 同步），彻底删除从
  Mermaid label 新增 anchor 的能力。标签仍可做展示和 soft guidance，不能成为 typed hard-gate 权限来源；
- **R2-2/R2-3**：sequence reply 改为结构判定——只有与图内反向的实线 invocation 配对、且自身没有
  显式 call anchor 的 `-->>` 才是回复。会话中偶然存在反向 call evidence 不再改变其角色；孤立虚线边继续
  进入 fail-closed call 合同；
- **R2-1**：删除 principal 完备性车道的裸标签 pair 快路，先把 principal pair 解析回具体 typed call
  EvidenceItem，再使用与可见边相同的 class/actor + exact operation resolver 核对；不引入模糊匹配；
- **作用域不变**：入口仍严格限定 `QFCallChain`。Trace 显式时间窗因果投影、自动补齐和运行时根因图不进入
  此源代码调用图合同；没有扫描原始请求、模型 prose 或最终渲染文本；没有系统改写模型结论。

新增四组回归臂：class participant/principal method alias 正向；已有反向 call evidence 的结构回复正向；
孤立 `-->>` 反向；执行完整 normalization 后 label-shaped Guard 仍无权、必须报 `missing_call_anchor` 反向。

状态：`R2-1=closed`；`R6-1=closed`；`R2-2=closed`；`R2-3=closed`。

下一批按 §8.5 进入 repomap：先验证并 bump extractor cache epoch，再修 Java `var`、C 参数作用域、
Kotlin/Swift 声明类型边界与 Cangjie named argument，逐语言保留正反 fixture；不会把一种语言的字符串规则
复制到其他语言。

### §9.3 M4-B repomap receiver 权限与缓存代际批（已交付）

复核确认 R7-1、R7-2、R7-3、R7-4、R7-6 均成立。R7-1 的失效域需从“改了多少提取文件”换算成
“多少持久化语言缓存会消费这些实现”：`b50f49233` 改动实际影响 Java、Python、JavaScript、TypeScript、
ArkTS、Cangjie、Kotlin、Ruby、Swift、Lua、Rust、C、C++ 共 **13 个** `extractorVersions` 域；共享的
`extractJS` 与 `extractCCpp` 不能只 bump 一个表面语言。上述 13 域已各提升一代，并增加 floor pin，暖缓存会
整体失配后重建，不再静默沿用旧 call/receiver 结果。

谓词修复保持各语言自己的结构边界：

- **R7-2 Java**：只有 Java AST `type_identifier` 的精确值为 `var` 时拒绝把它当声明类型；保留源 receiver，
  不从 initializer 猜类型，也不影响显式类型、泛型容器和数组；
- **R7-3 C/C++**：参数 receiver census 从 file-wide 收窄到调用所在 `function_definition` 的 declarator；
  另一函数的同名参数不能改写本函数局部变量，C++ 共享提取面同步受益；
- **R7-4 Kotlin/Swift**：类型只从 AST `type` field 或直接的 `user_type/type_annotation/nullable_type` 等
  声明 carrier 读取；不再递归扫描整棵参数/属性子树，annotation 和 initializer 中的类型无权铸造绑定；
- **R7-6 Cangjie**：`ident : ident` 只有位于 `let/var/const` 声明或函数/init/main/type 声明参数表时才
  进入 receiver census；调用参数 `submit(width: payload)` 明确不能覆盖 `width: Width` 的声明身份。

新增 fixture 覆盖：两个 Java `var` receiver 不折叠成伪类型 `var`；C 参数身份不跨函数污染同名局部变量；
Kotlin/Swift initializer type 不冒充声明类型且显式参数仍能解析到定义；Cangjie named argument 不破坏参数
类型；13 个缓存代际全部有下限 pin。

状态：`R7-1=closed`；`R7-2=closed`；`R7-3=closed`；`R7-4=closed`；
`R7-6=closed`。R7-7/R7-8 已在 §9.7 通过独立可执行反例闭环。

### §9.4 M4-C §7 裁定收尾（已交付）

- **R1-1**：补双工件真实 multi-cluster pin：一个席位 `frame_causality=unproven`、另一个席位已证；两板均
  保持同一 `主根因(=已证链上单项最大可消除量)` 前缀，且“帧因果未证”只出现在消费该 authority 的席位，
  防止未来 nil-authority/会话 ANY 分支重新污染同页其他板；
- **R1-2**：detail proven 臂不再手写中英文 `主根因(优先处理)` / `primary (handle first)`，统一读取
  `runtimeTraceProjCrownWords(...).DetailPosition`；headline、legend、detail 与 comparison 的裁定词源继续集中。

该批仅补结构 pin 与词源去重，不改变选举、值、排序、Trace 自动补齐或模型正文。

状态：`R1-1=closed`；`R1-2=closed`。

### §9.5 M4-D write owner-anchor 双载体撤销（已交付）

M4-R3-1 判断准确。`refreshAppliedPlanOwnerAnchors` 原来只在 `run.ContextPacks` 把变更路径的旧 owner anchor
标为 stale；`Mutable.WriteContextPack` 是防御性复制的另一份快照，并未同步。由于同 fingerprint item 合并时
`Stale = existing.Stale && incoming.Stale`，任一未撤销孪生副本都会把旧 owner 权限恢复为可用。

修复把“按 normalized changed path 标记 owner anchor stale”提成单一函数，并在重新从当前 worktree 铸造新
anchor 前同时应用到 run 与 Mutable 两份载体。测试先制造两份旧 owner 副本，验证 refresh 后两侧均撤销，
再执行一次真实 `MergeWriteContextPack`，确认旧 anchor 不会复活、未变更 helper anchor 仍可复用、新 owner
成为唯一当前权限。

状态：`M4-R3-1=closed`。

### §9.6 M4-E callable:line owner 归属（已交付）

R6-2 判断准确。`preEmitCallableSurfaceOwnsCitationWithContext` 先尝试完整 callable 匹配，但失败后无条件把
qualified callable 拆成 member，再只比较 member；因此同为 line 10 的 `A.create` 与 `B.create` 都能被
`A.create:10` 接受。修复规定：qualified callable 的 owner 是显式精确信号，只能由前面的 typed evidence
或完整 `Citation.EnclosingFunction` 匹配证明；完整匹配失败即 false，只有原本未限定 owner 的 `create:10`
才可走 member fallback，并在多候选时保持歧义、不猜选。

新增同 line、不同文件、不同 owner 的双 citation fixture：`A.create:10` 与 `B.create:10` 各自唯一归属，
`create:10` 明确保持 ambiguous。

状态：`R6-2=closed`。

### §9.7 M4-F repomap 保守 caller 与 Lua sugar（已交付）

R7-7/R7-8 均由可执行反例确认：

- **R7-7**：新提取器把动态调用的源 receiver（如 Python 的 `tool`）保存在非空 `ToEP.Receiver` 后，
  `CallersOfID` 把所有非空不等值都当“已解析为另一类型”排除，破坏了其文档承诺的 unresolved conservative
  include。修复不回退 receiver 信息，也不新增语言启发式：当 receiver 字面量与目标不等时调用现有
  `ResolveCallTarget`；只有确实解析到另一 concrete symbol 才排除，解析不到的源表达式继续作为潜在 caller，
  解析到当前 target 则保留；
- **R7-8**：Lua 原实现读取整个 `function_call` 后按括号/空白截断，合法无空格糖 `printer"text"` 与
  `repo:load"key"` 会把 argument 拼进 callee。修复直接消费 AST 的 identifier children，并在
  `string_argument/arguments/function_call_paren` 参数 carrier 前停止；普通 `repo.save()` 与两种 sugar 共用
  同一结构路径。Lua extractor version 再升一代（4→5），避免刚生成的旧 call identity 暖缓存残留。

测试同时钉住：unresolved dynamic receiver 对多个同名方法保持保守候选，已解析 ToolB receiver 不会污染
ToolA；Lua 三种调用均输出纯 callee/receiver，argument 字节不得进入 `ToEP.Name`。

状态：`R7-7=closed`；`R7-8=closed`。

### §9.8 M4-G Mermaid 消息边界与合同教学同步（已交付）

- **R2-4**：sequence 解析原先先按 operator 优先表搜索整行，再递归处理 `to` 中的箭头；消息
  `A->>B: compare C-->>D` 会因消息里的 `-->>` 优先级更高而被改写成伪边。现在 sequence 专用解析只取
  源码顺序中第一个最长 arrow，剩余字节交给 target/message 分割，消息中的任意 Mermaid-looking arrow
  保持普通消息文本；flowchart 的 chained-edge 兼容路径不变；
- **R2-5**：把生产硬合同同步到三个 LLM 可见面：通用 relation contract、finalizer Tier-B edge rule、
  current-source mechanism authority。明确 structural reply 必须镜像已画正向 invocation，孤立 `-->>` 不自证；
  call_dag 的非调用边也必须由模型显式 typed anchor 声明，label 永不自铸权限；principal-path structured
  items 已选择 grounded call 两端时，同向调用必须留在 sequence/call_dag，未选择的 supporting calls 不强制画。
  legacy label vocabulary 仅是非严格图的显示关系兼容，不能创建 edge anchor 或满足调用权限。

新增 sequence message 同时含 `-->>` 与 `->>` 的反例；prompt SST 钉住 mirror、standalone、no-auto-mint 与
principal completeness 四条教学，不读取用户/模型原文做硬门。

状态：`R2-4=closed`；`R2-5=closed`。

### §9.9 M4-H：material 页 UTF-8 尾部验证线性化（已交付）

复核确认 `M4-R3-2`。旧实现对 2 MiB 截断材料逐字节缩短并对整个剩余前缀重跑
`utf8.Valid`；若非法字节位于正文而非截断尾部，会形成二次方扫描。现把候选裁剪严格限制在
UTF-8 单个编码单元最大可能缺失的 0..3 字节：任一候选完整即保留，四个候选均不完整则拒绝
整份材料。这样不放宽正文合法性，也不替换或修补材料字节，最坏扫描量由 O(n²) 收敛为
4×O(n)。2 MiB 正文早部非法、尾部仍合法的反例与三字节截断尾正例均已固定。

状态：`M4-R3-2=closed`。

### §9.10 M4-I：转换 perf sidecar 有限族保留（已交付）

复核确认 `R4-1`。standalone profiler 最多接纳 256 个块，每个 perf-eligible 块可能通过转换包的
`numberedSidecarPath` 发布一份 raw `.perf.data`，并在 adapter 启用时发布对应 `.perftrace`；第一个无
序号，后续为 `_2.._256`。旧的预路由清册只登记首个 `.perftrace`，因此 diagnostic sideband 可以提前
占用 raw 或编号席，直至转换中途才因输出已存在而失败。

现由转换包在内容探测前按同一命名函数和同一 256 上限发布完整有限清册；CLI 仍只消费该 API，不复制
suffix/序号公式。关闭 perf adapter 时仍保留全部 raw 席，但不虚报不会生成的 `.perftrace`。首席、第二席、
末席与上限外反例均已固定，诊断文件在创建前 fail-closed，不改变转换路由或工件内容。

状态：`R4-1=closed`。

### §9.11 M4-J：aggregate member 三元槽位对齐（已交付）

复核确认 `R6-3` 的错位风险，但修正其范围：`normalizeAggregateStringSlots` 同时处理
`Members / SupportRefs / MemberNotes`，旧实现会分别删除每个数组中的空字符串；所以稀疏 support ref 会
左移到错误成员，稀疏 note 也会发生同样问题，并非 MemberNotes 已有独立守卫。

现恢复该函数真正的 slot 语义：在固定上限内保留中间空槽，只裁去无语义的尾部空槽；随后已有
`normalizeAggregateMemberSetMemberSupportNoteSurfaces` 以成员为主键成组删除空 member tuple、去重并合并。
回归分别固定“成员 A 无 ref/note、成员 B 有 ref/note”不得左移，以及空 member 连同其 orphan ref/note
一起删除。该修复只消费 structured arrays，不扫描用户输入或模型答案原文。

状态：`R6-3=closed`。

### §9.12 M4-K：eval primary 尾域显式边界与 renderer 双向 pin（已交付）

复核确认 `R8-1/R8-2`，并发现真实 recovery 发射面比摘要更明确：`scope_primary_stdout` 直接停止于
citations，但 snippets 排在 citations 之后，因而仅在 citations 存在时被传递性排除；无 citation 的答案可由
snippet-only 符号误刷绿。runner 还监听“模型最后一轮原文”，而当前 renderer 的真实面板头是
“系统保留内容 / System-preserved content”，无 citation 时 recovery 原文同样会泄入 primary 域。

现为 citations、snippets、recovery panel 各自增加中英文精确边界；保留旧 recovery 字面量用于历史工件，
但不再依赖它。新增无 citation 的 snippets+recovery 反例，并由 Go renderer 真实渲染中英文六个标题，反向
检查 `eval/run.sh::scope_primary_stdout` 必须逐一消费；runner 若新增不存在的生产标题或 renderer 改标题却
未同步，契约测试都会失败。该门仅属于 opt-in eval oracle，不进入产品路由、模型上下文或答案修改链。

状态：`R8-1=closed`；`R8-2=closed`。

### §9.13 MERGE-AUDIT-4 收账（2026-08-03）

§8 的确认项已全部按独立根因批交付并推送：

| 批次 | 提交 | 关闭项 |
|---|---|---|
| M4-A | `ca56bdb65` | R2-1、R6-1、R2-2、R2-3 |
| M4-B | `1f74c2e00` | R7-1、R7-2、R7-3、R7-4、R7-6 |
| M4-C | `2fe108f2c` | R1-1、R1-2 |
| M4-D | `0394678e0` | M4-R3-1 |
| M4-E | `5923d3237` | R6-2 |
| M4-F | `db77f8cbc` | R7-7、R7-8 |
| M4-G | `f38501a08` | R2-4、R2-5 |
| M4-H | `c66a7e284` | M4-R3-2 |
| M4-I | `e433dafd5` | R4-1 |
| M4-J | `6ff1a7bb0` | R6-3（并补正 MemberNotes 同根错位） |
| M4-K | `156d3aa2f` | R8-1、R8-2 |

施工回归还发现 B53 mixed runtime/source coherence 新消费者绕过 authority chokepoint；
`45e44acc9` 已改接共享 request-only authority API 并同步 B53 文档。该项不是 §8 新 finding，但属于本轮
组合回归发现的确定性红灯，未留到下一战役。

最终验证基线 `main@156d3aa2f`：`make test`（`CGO_ENABLED=1 go test ./...`）全仓通过；其中
`internal/hitraceconv` 108.420s、`internal/tool` 194.409s、`internal/tracequery` 86.633s、
`internal/types` 30.040s，所有 repomap 语言包、read/write orchestrator、Trace 投影、Mermaid、renderer 与
eval runner contract 均为绿。实现未扫描用户原始输入或模型输出做新硬门，未由系统替写模型结论，显式
时间窗 Trace 因果投影与自动补齐链保持既有权限。

收账状态：`MERGE-AUDIT-4=closed / all-confirmed-findings-delivered / full-repo-pass`。

---

## §10 MERGE-AUDIT-5 增量审计(2026-08-04):62 笔 = §9 交付核验 + 所有权 sweep + 调用链方向 + trace/current-source 波

**范围**:6a1998d9e..HEAD 非本席合入 62 笔(08-03 06:41 → 08-04 01:37)。**方法**:7 主题读者+逐条 2 席否证(37 agent,wf_74480175-eae,含 go test overlay 可执行复现)+全仓基线实测+高危亲验;只审计不修码。产出:12 确认(5 高)+7 低+3 项被否证席驳回(未入账);**55 项核验通过**。

### §10.1 基线与 §9 交付核验总评(正面)

- **main 绿**(本席实测零失败,与 MERGE-AUDIT-4 前的红基线对照,合入纪律已恢复)。
- **§9 delivery ledger 22 项 closed 声称经三个主题独立核验全部属实**——R2-1 完备车道与边门同源化、R6-1 自铸锚整块删除(结构性修而非调序补丁)、R2-2/R2-3 回边豁免改结构判定(镜像已画正向实线边)、R7-1 十三语言域全部 bump、R7-2/R7-3/R7-4/R7-6/R7-7/R7-8 逐语言谓词修、R1-1 多板 pin(`TestMultiArtifactSeatsKeepFrameAuthorityAndCrownWordingIsolated`)、M4-R3-1 双载体撤销、R8-1/R8-2 eval 域边界显式化,零虚账。
- S6 波多项**提杆**:散量权威去子串臂改 typed 同源、系统 metric 复发布通路删除、完成门关系确认收敛为软(retry storm 修)。
- 3 项 reader 指控被否证席驳回(S4-1/S4-2/M5-S3-3),对抗复核有效。

### §10.2 高危确认(5 件)

| # | 位置 | 问题 | 复现 |
|---|---|---|---|
| M5-S1-1 | `answer_document_diagram_evidence.go:141` | **调用锚车道扩域到全家族但没带别名解析**(T3-2/R2-1 类**第三例**):4dc46e393 把图表调用证据门从 QFCallChain 扩到所有非 RootCauseTrace 家族,但严格 body 车道(带 message-operation 别名解析)仍只在 QFCallChain 开——非调用链家族只跑锚环且 edgeLabel 恒空,教学形 class participant 与 sibling-carrier 节点 ID 双双硬拒;同一证据池同一图在 QFCallChain 过、在 QFGeneric 拒 | 否证席 **go test overlay 可执行复现**(双席) |
| M5-S3-1 =M5-F1(双镜头汇合) | `change_plan_validate.go:1269` | **新定界符失衡硬门误 lex Rust 生命周期**:通用分支把每个 `'` 当字符串开符,`fn parse<'a>(input: &'a str)` 被"确定性"判失衡→合法计划硬拒 `planned_source_delimiter_imbalance`,无 typed escape 车道(违「硬门必配 typed escape」§1.6);Kotlin/Swift 插值同病 | 否证席可执行复现 |
| M5-S3-2 | `change_plan_validate.go:375` | **过针探测冻结门未绑 post-apply**:planStageProbeReports 全 Run 追加、latestPlannerProbeReport 取最后一条无 PlanID/代次绑定——round-1 规划期环境探测通过 ≠ apply 后仍健康,verify 真失败后正确的 replan 修复被硬拒 | 静态(载体逐点核) |
| M5-S5-1 | `emit_investigation_complete.go:12215` | **定向可达门从实体表顺序铸调用方向**:endpoints[0]→endpoints[len-1] 当 source→sink,但 ExactTargets/MentionedEntities 是 LLM 提及序,无任何 schema/validator 约定顺序编码方向——"main 如何到达 handleRequest?"若模型先提 handleRequest 即要求反向路径,已证正向路径被判不可达 | 静态 |
| S6-1 | `command_measurement_evidence_path_authority.go:58-76` | **codrax 自身内部架构文本被硬编码进通用 prompt**:`internal/tool/builtin.go` 的 `execCommandMeasurement`/`CompileObservationLedger` 等内部函数/文件路径逐字注入 explorer 中环提示与 finalizer 首指令,按通用 typed 触发对**任意客户仓**生效——双红线(LLM prompt 禁内部管线信息 + 错仓架构污染:模型解释客户仓时会引用 codrax 内部件) | **本席亲验**(66/67/75/76 行逐字) |

### §10.3 中危确认(6 件)

- **S4-3** `system_crosscheck_appendix.go:110`(裁定敏感):所有权 sweep(72146af38)一并退役了**用户既裁的 appendix 软披露臂**——CR-3 件① P6 墙钟守恒(2026-07-12 裁「只披露,永不 violation/rewrite」)、HEADLINE-ELIM、FREQDIR-1、散量残差等——方向(去 gate 化)对,但这些是裁定的**软披露**目的地,非 gate;错误算术/漏方向词现在全仓零披露。**建议按既裁恢复为 typed-fact 车道内的软行,或提请改裁。**
- **S2-1** `extract_java.go:334`:var 修复让空 typeName 静默跳过而不记冲突——var 遮蔽同名带类型声明时跨作用域错捕获(修复自身的新缺陷)。
- **S2-2** `extract_navigation_calls.go:20`:R7-3(census 全文件误用)只修了 C——**Kotlin/Swift 同类同批未修**(同批不同语言不同纪律)。
- **S2-3** `graph.go:286`:R7-7 残留——bare-function 回退解析获得排除权威。
- **M5-S5-2** `emit_investigation_complete.go:12417`:短名端点歧义(≥2 qualified 匹配)时 resolveEndpoint 返 nil→硬报"端点不在图中",尽管已证路径存在(歧义≠缺席)。
- **M5-S5-3** `diagram_evidence.go:635`:新 qualified-caller 接受臂三个条件都不验 caller owner 的证据绑定(接受面无证据锚,与整批"绑证据"方向相反)。

### §10.4 低危顾问(7 件)

M5-S1-2 sequence 分割器闭集运算符(`--)`/`-x` 异步形不可见,门/教学双盲);M5-S3-4 歧义尾端点全候选可达仍硬 unresolve;S4-4 模型所有权 AST tripwire 只扫直调(一层包装即绕);M5-S5-4 token 边界匹配吞跨分隔符拼写等价;S6-2 dba723a30 退役了同波刚立的桌面目标操作负 pin;M5-F2 M4-K"双向 pin"声称过度(runner 侧方向未钉);M5-F3 同事 B54..B71 账本新自报残留 10 项(继承债清单,详见其账本"状态:"行)。

### §10.5 处置建议(未动码)

1. **高危批一(写模式门,同文件)**:M5-S3-1 定界符 lexer 按语言族修(或加 typed escape 车道)+ M5-S3-2 探测报告绑 PlanID/apply 代次——两件都是新硬门无 escape 的 §1.6 违例。
2. **高危批二(图表门三修)**:M5-S1-1 扩域必须连别名解析一起扩(或收回扩域);M5-S5-3 接受臂补证据绑定;M5-S1-2 运算符集镜像 parse 表。
3. **高危批三**:S6-1 整段内部架构文本改为运行时从 typed 车道派生或删除(错仓污染即刻止血);M5-S5-1 方向从 typed 关系边派生,禁从实体序推断。
4. **裁定请求(S4-3)**:既裁软披露臂恢复 vs 改裁,提请用户。
5. S2-1/S2-2/S2-3 repomap 补修批;中低危随批。

**方法学**:①同事修复合规率连续两轮 100%(22/22、30/30 核验通过)且**零虚账**——"提交名↔finding 对应+交付账本"协议已稳定;②新缺陷持续集中于**新硬门/新扩域**(T3-2 类第三例、§1.6 无 escape 两例、方向铸造一例)——建议同事侧把「新硬门四查」(教学同步/同源别名/typed escape/精确信号)纳入其提交前清单;③否证席驳回 3 项虚指控+两处 overlay 可执行复现,对抗层持续产出判别力。

### §10.6 S4-3 裁定落案(2026-08-04,用户批复「同意」;仅方案,未动码)

**裁定**:评判臂维持退役,既裁披露职能以「typed 事实并置」形回归幸存车道(`proseTypedFactJuxtapositionFindings`,orchestrator/system_crosscheck_appendix.go:110 唯一挂点)。这不是"恢复 vs 改裁"二选一,而是**既裁披露的实现形升级**——同事 sweep 的成果(系统只说 typed 事实、永不评判/改写模型文本)与 CR-3 件① P6 / HEADLINE-ELIM / FREQDIR-1 三项既裁(披露必须存在)全部保留,零裁定推翻。

**结构拆分定谳**:旧臂的两个组成分离处置——①**检测逻辑**(扫 prose 找算式/headline 词/方向词,嘈声信号)保留但**降格为选择器**,只决定"并置哪些 typed 行"(「精确硬门/嘈声软引导」红线恰好允许);②**评判句式**(「模型算术不符」类裁断)维持退役——即使在软面,系统对模型答案下判词也与所有权纪律冲突,且 prose 扫描误判会以"系统指控"形暴露给用户。

**三场景回归形**:
- **P6 墙钟守恒**:prose 检出数值算式 → 并置 typed 对账行 `对账参考: 窗口 100.000ms · 链上已归因 82.000ms [E3] · 背景 9.100ms [E7]`——错误算式与正确 typed 账同页同尺并置,读者自行对比;
- **HEADLINE-ELIM**:结论段检出板相关量词 → 并置 `同尺并置: ◎ TOP1 = <席位> <值> · 修向 <方向词> [E#]`;
- **FREQDIR/散量残差**:同形,并置该场景 typed 席位/残差账行。

**选择器精度分级**:数值 token 与 typed 行 3 位小数精确匹配≈精确信号,放心触发;算式形状扫描=嘈声,只可决定并置与否,永不进发射文本。

**施工 pin 面**(留给修复批):①appendix 评判词负 pin(禁「误/错/缺/遗漏/不符」指向模型答案的裁断词族;固定中性框架词 `对账参考:`/`同尺并置:`);②每条并置行必须引用 ledger typed 行并带 [E#](拒渲绝不造数);③P6 场景 e2e 先红后绿(模型 prose 带错算式 → appendix 必出对账行);④所有权 tripwire 维持不动(appendix 永不改模型块),并注意 S4-4(tripwire 只扫直调)在同批加固。

### §10.7 主线复核与首批止血（2026-08-04）

更新到 `main@e4588c9c5` 后对 §10 五个高危逐项冷读。结论均成立，但应按独立权限面拆批：

| finding | 主线复核 | 处置 |
|---|---|---|
| S6-1 | confirmed。通用 route/profile + command-measurement carrier 即可触发；中英文 prompt 逐字包含 Codrax 自身文件、函数和内部 carrier，客户仓无需含这些符号 | 首批独立止血，已实现 |
| M5-S3-1 | confirmed。`.rs` 通用 lexer 把两个 lifetime apostrophe 之间的内容当 quoted literal，吞掉 opening `(` 后把后续 `)` 判为 definite imbalance；矩阵测试只有“额外 `}` 必拒”，没有合法 lifetime/label/char 反例 | 下一写模式门批，按 extension-aware lexical rule 修，不用请求/计划 prose escape |
| M5-S3-2 | confirmed。`VerifyFailureHandoff` 已带 PlanID/Attempt/GeneratedAt，planner probe report 也有 PlanID/GeneratedAt；但资格函数只取全 Run 最后一条 Channel=planner_probe，不绑定 handoff generation | 与 S3-1 同批，使用 typed PlanID + post-handoff timestamp/attempt 边界 |
| M5-S1-1 | confirmed。strict parsed-edge 车道把 Mermaid edge label 交给 alias/operation resolver；通用 edge_anchor 车道同一节点对固定传空 label，扩域后形成家族不对称 | 图表批统一从同一 parsed edge/typed anchor resolver 取 identity，禁止再复制一套别名判定 |
| M5-S5-1 | confirmed。`CallChainRequestedEndpointHints` 的合同只有 identity lane 优先级与去重，没有 source/sink 顺序语义；hard reachability 却直接取 first→last | 单独演进 typed ordered endpoint carrier；载体缺席时不得从实体提及序铸方向 |

#### S6-1 delivery（closed / prompt-only / no customer-repo internals）

`renderCommandMeasurementEvidencePathAuthority` 保留它真正有权提供的通用软信息：当前存在 deterministic command measurement；该测量与
model-authored summary 是独立 evidence carriers；兼容计数可单向核对，但载体并置不证明客户仓 call/data-flow/ownership edge。机制结论必须从本轮
实际读取并引用的客户仓源码推导，证据不足则保留边界或继续读源码。

中英文发射面已删除 `internal/tool/builtin.go`、`execCommandMeasurement`、`ToolResult.CommandMeasurement`、
`CompileObservationLedger`、`observationRecordForCommandMeasurement`、`emit_investigation_complete.go` 等全部 Codrax 仓专属名。
没有改激活 typed 条件、没有请求/模型/答案关键词扫描、没有 completion/answer hard gate，也没有系统答案改写。正向 pin 固定 evidence-authority
边界，负向 pin 固定上述内部名不得再进入通用客户仓 prompt；Explorer one-shot hint 与 finalizer prompt 两个真实消费面同时覆盖。

验证：定向 agent tests 1.127s；`go test ./internal/agent -count=1` 2.949s，全绿。状态：`S6-1=closed`。

后续施工冻结：`M5-A=S3-1+S3-2` → `M5-B=S1-1+S5-3+S1-2` → `M5-C=S5-1 typed ordered endpoints` →
`M5-D=S4-3 裁定实现+S4-4 tripwire` → repomap 中危批。每批独立提交推送；相关 GAP 清完后再恢复严格并行 2 个异构 eval。

### §10.8 M5-A：语言感知定界 lexer + planner probe 代次权限（已交付）

#### M5-S3-1：Rust apostrophe 不再被通用 quote 分支误铸

确认高危反例，修复放在 source lexer 的 typed extension 分支，而不是用户请求、计划 rationale、patch prose 或模型答案的关键词 escape：

- `.rs` 遇到 apostrophe 时先识别 Rust lifetime / label token（`'a`、`'static`、`'_`、`'outer:`）；
- token 后紧跟 closing apostrophe 时保持 character literal（`'a'`、`'\''`、`b'a'`）语义，继续交给原 quote scanner；
- identifier 消费支持 Unicode letter/digit/mark，避免只对示例 `'a` 做单字节拟合；
- Rust raw string、nested comment、普通/byte char 与真正 opening/closing delimiter 的既有顺序不变。

新增真实 `validateNewSourceDelimiterImbalance` 正反 pin：带 lifetime 的合法 Rust edit 必须通过，同一源码追加额外 `}` 仍必须拒绝；另钉 lifetime、loop label、普通 char、escaped apostrophe 共存。审计摘要所称 Kotlin/Swift“同病”需收窄：两者的 interpolation 当前位于被整体跳过的 string carrier 内，表现为保守 fail-open，不会复现 Rust 的合法源码 hard reject；新增 Kotlin `${...}` 与 Swift `\(...)` 合法形 pin，后续若要检查 interpolation 内部失衡，应作为独立 lexer 能力演进，不能借本案放宽/猜解析。

状态：`M5-S3-1=closed`（确定性 false reject 已消除；Kotlin/Swift interpolation 深层解析为独立 fail-open 能力项）。

#### M5-S3-2：冻结权绑定 active PlanID 与 verify-failure generation

确认旧 `latestPlannerProbeReport` 只按 `Channel=planner_probe` 取全 Run 最后一条，陈腐环境探测可获得后续 replan 冻结权。现统一资格为：

1. report `PlanID` 必须等于 `VerifyFailureHandoff.PlanID`，且 active prior plan 若有 ID 也必须同一；
2. timestamped handoff 只接受 `GeneratedAt >= handoff.GeneratedAt` 的 report，未打时间戳的旧 report 对新 handoff 无权限；
3. `installRunTestsReport` 在 planner probe 未带 PlanID 时从 active `ChangePlan.ID` 自动盖 typed stamp；
4. qualification 返回 exact `ProbePlanID + ProbeGeneratedAt`，多文件 protected-path coverage 必须消费同一 report，禁止资格函数与路径函数各自再选一次“latest”；
5. 双零 timestamp 仅保留 durable legacy carrier 兼容，生产 timestamped handoff 不会降格到此臂。

回归覆盖 failure 前通过的 probe、错误 PlanID probe 均不得冻结；同 plan 且 failure 后的新 probe 才可授权；新的 failing probe 仍会重开 mutation lane；真实 `run_tests(dry_run=true, verification_probe=...)` 自动带 active plan ID。完整相关包验证：`internal/tool` 167.053s、`internal/writeflow` 0.344s、`internal/types` 20.526s，全绿。

状态：`M5-S3-2=closed`。本批没有读取用户/模型 prose，没有改变 read mode、Trace 时间窗因果投影、自动补齐或模型结论所有权。

### §10.9 M5-B：图表 alias、owner 绑定与 sequence 运算符同源（已交付）

#### M5-S1-1：全部非 Trace 家族复用同一 operation resolver

确认扩域后的家族不对称。修复没有收回“显式 `relation_kind=call` 必须有 typed call evidence”的精确信号硬门，而是让 anchor 车道复用 parsed-edge 车道已有的严格解析：

- 为当前 diagram 建立 local node/edge registry；为 sibling structured carrier 建立 document-level registry；
- node ID 只有在可见 label 唯一时才投影，跨图或同图冲突时删除别名、fail-closed；
- call anchor 先走 exact typed endpoint，只有 exact lane 不成立时才读取同方向 body edge 的唯一 structured message operation；
- 同一 node pair 出现多个不同 message label 时不猜选；body label 只作 discriminator，不自铸 call 权限；
- `QFRootCauseTrace` 继续在独立 runtime relation authority，显式时间窗 Trace 因果投影与自动补齐不进入 source-call 合同。

回归固定同一 class-participant graph + 同一 evidence pool 在 call_chain / generic / architecture / role_lookup 四家族结论一致；edge anchor 位于 sibling block 时可消费唯一 diagram identity；新增冲突 label 后必须拒绝。

状态：`M5-S1-1=closed`。

#### M5-S5-3：qualified caller 不再由异源短名拼接

确认旧 fallback 的 required mechanism anchor 只证明“请求选择了 `gate.Run`”，不能证明某条 `Run -> RunWith` 的短名 call site 就属于 `gate`。现把该窄 fallback 收紧为四个 typed 事实合取：

1. `gate.Run` 是 exact required mechanism anchor；
2. citable definition 唯一绑定 owner=`gate`、operation=`Run`；
3. short caller call site 与该 definition 的 `Source` 文件相同，且 call direction/operation 唯一；
4. qualified callee surface 在 citable call evidence 中精确出现。

缺 caller definition、definition 多义、短 call 来自其他文件、同文件多 call location 任一成立均 fail-closed。没有从路径名推 owner，没有扫描 Mermaid prose、请求原文或答案文本。

状态：`M5-S5-3=closed`。

#### M5-S1-2：parser / renderer / teaching 共用 sequence operator 合同

原 semantic parser 只识别 `->>/-->>` 等窄表，renderer 却另有完整表，导致 `-)/--)/-x/--x` 与 `+/-` activation suffix 可显示但对 evidence gate 不可见。现由 `mermaidcompat.FindSequenceArrow` 发布唯一 source-order/longest-match 表，semantic `ParseEdges`、normalizer 与 terminal renderer 全部复用；`SequenceArrowBase` 只为结构 reply 判定剥离 activation suffix，Edge 仍保存完整 operator。

finalizer Diagram Contract 同步教学：async/lost/activation 都是 visible structured edge，必须有同方向 typed relation/evidence，不能当装饰或逃逸；仅与已证 forward invocation 成对的 reverse `-->>`（含 activation suffix）保留 response 语义。24 种 operator 正表逐值 pin，另有 activated invocation + reply 的 authority 回归。

完整验证：`internal/tool` 168.231s、`internal/agent` 2.654s、`internal/mermaidcompat` 1.312s、`internal/render` 1.659s，全绿。

状态：`M5-S1-2=closed`。`M5-B=closed`；没有系统改写模型答案或新增 prose hard gate。

### §10.10 M5-C：调用链方向权威与歧义候选语义（已交付）

#### M5-S5-1：实体集合不再铸造 source → sink

复核确认高危 finding。`ExactTargets / MentionedEntities / PrimaryEntities / Entities` 的合同只定义身份、
来源与优先级，从未定义数组顺序是调用方向；旧消费者却在定向可达、principal span、no-directed-path
边界和 completion note 多处取 `first → last`，所以模型仅仅先提到被调端就会把已证正向图硬判为反向不可达。

本批把方向从身份集合中结构性拆出：

- `RequestModel.CallChainEndpointProfile{source,sink}` 是唯一有序方向载体；`emit_analysis` schema 明确
  `entities / exact_targets` 均为无序集合，非 source-code call-chain 发空值；
- 载体只接受已通过 request-proven typed identity lane 的两个不同代码符号，不能由路径、模型推理文本、
  用户关键词扫描或答案原文补造；无效 optional 载体被审计性丢弃，方向硬门停用而非重试轰炸；
- reachability、principal-span 窗、no-directed-path typed boundary、completion caveat 四个方向消费者全部改读
  同一 API；载体缺席时不得回退到实体提及序。身份展示/软导航仍可消费无序集合，但不能获得方向否决权；
- `AnalysisIRVersion` 升至 `v19`，让缺少新载体的暖缓存失效，避免旧 IR 在新方向合同下静默复用；
- 正反 pin 固定：同一 identities/exact-targets 即使顺序不变，model-authored `source/sink` 反转也必须原样保留；
  非 request-proven sink 不得落盘；仅有两个 entity 也不得启动方向 hard gate。

该载体只服务 source-code `ReqCallChain + AxisCall`。Runtime Trace 的唤醒链、显式时间窗因果投影、系统补齐
与帧因果权限仍走既有 typed Trace 车道，不进入本合同，也没有系统替写模型结论。

状态：`M5-S5-1=closed`。

#### M5-S5-2 / M5-S3-4：歧义不是缺席，候选全集覆盖才可证明

旧 `resolveEndpoint` 遇到两个以上 qualified tail 匹配直接返回 nil，把“短名有多个候选”错误渲染为“端点
不在图中”；即使每个候选都有同向 typed path 也必拒。现 resolver 返回 `{candidates, ambiguous}`，诊断分离
`resolved / ambiguous / absent`：

- 唯一 qualified 匹配维持原路径；class/actor owner 聚合维持多语言 participant 语义；
- start 有歧义时，每个 start candidate 至少参与一条到某个 end candidate 的同向 typed path；end 有歧义时，
  每个 end candidate 至少被某个 start candidate 同向到达；两侧同时歧义时两项覆盖条件同时成立才通过；
- 只覆盖部分候选时保持 fail-closed，但披露为“已解析且有未覆盖歧义候选”，不再谎报端点缺席；
- 候选覆盖完全由 accepted citable `ClaimCallEdge` 图计算，definition、近邻、prefix sibling、label 或 prose
  都不能铸边。

测试覆盖 15 种当前 executable language surface、短名双侧全覆盖与部分覆盖、先提 sink 的反向顺序、
no-directed-path 系统补充抑制、principal span/proactive repair 和 semantic-view boundary。完整
`internal/tool` 回归 159.148s；定向 `internal/types` 21.933s，全绿。

状态：`M5-S5-2=closed`；`M5-S3-4=closed`；`M5-C=closed`。

### §10.11 M5-D：S4-3 中性 typed 并置 + S4-4 所有权可达性看护（已交付）

#### S4-3：披露职能恢复，但系统不评判模型结论

按 §10.6 裁定，旧 `proseWallClockConservationFindings` / `proseHeadlineElimFindings` 等评判器继续退出
production。发布面新增的不是“模型错了”判词，也不改正文，而是 display attachment 中三类中性行：

- `对账参考:`：模型 prose 出现 typed 三位小数或算式形状时，只并置目标线程已闭合的全窗状态分区；
- `同尺并置:`：出现 headline 形时，只并置最终投影同一选举板的根因排序 #1 席位及有效归因；
- `同尺并置:`：出现修向枚举形且该 typed 修向尚未被点名时，只并置 #1 席修向与该席有效归因。

prose 扫描在这里仅是“是否显示哪条真值”的软选择器：它不提供 subject、数值、关系、修向或结论，不进
violation/retry/hard gate，不引用用户请求原文，也不会把扫描到的模型词句带进发射文本。中英文发射均固定为
中性框架；回归负 pin 禁止 `错误/不符/遗漏/正文将/正文称` 以及 `wrong/error/missing/omitted/model says`
等评判词进入这三类行。appendix 仍是 `AnswerDisplayAttachment`，模型块 JSON 在收集前后字节不变。

#### OM-5 同批闭环：四态账进入投影原生 E# 索引

§10.6 要求每条并置行必须带真实 `[E#]`，冷读发现四态账只有 `EvidenceID`、没有持久 locator，正是历史
开放项 OM-5。若在 appendix 自编编号会制造悬空引用，因此本批先根修证据通道：

- `TraceCausalProjectionTargetStateAccount` 从被选中的 `target_window_states` 观测原样携带
  `SupportRefs/LineStart/LineEnd`；
- 仅当既有“目标一致 + 同窗 + 显示精度 Σ==窗”准入成立且 locator 非空时，账户才在普通 causal node
  编号完成后加入该投影的同一 `runtimeTraceCausalProjectionEvidenceIndex`；
- 四态首行显示自己的 `[E#]`，证据索引出现同号 `predicate=target_window_states` 条目；locator 缺席则
  整个 tag/并置行静默，绝不降格为 `trace_query` 虚定位；
- 多工件每节仍独立从 E1 编号，appendix 同时带工件 basename 消歧；只有 shipped document 已实际存在
  runtime Trace projection system block 时才导出并置 row，避免指向未发布/被块预算裁掉的证据区。

`RuntimeTraceReconciliationRows` 在 tool 层复用与页面完全相同的 projection compile、tree model、rank board
和 evidence index，向 orchestrator 只暴露 typed carrier。榜首/修向也必须通过 `evidence.has(node)`，任何看不见
的席位都不能因 appendix 调用而新铸 E#。信息契约把 Account `EvidenceID` 从 `known_gap` 翻为 `displayed`，
OM-5 从开放 registry 删除（开放项 13→12）。

#### S4-4：从“搜两个文件直调”升级为 production root 可达性

旧看护只在 `contract_check.go/system_crosscheck_appendix.go` 搜禁止函数的直接调用，加一层 wrapper 即可绕过。
本批做两层结构修正：

1. production typed-fact provider 与 offline prose-verdict provider 物理拆函数；不再用
   `includeProseVerdicts=false` 运行同一大函数。生产调用图中不存在一个“运行时 false 才安全”的隐蔽分支；
2. 测试用 Go AST 建全 package 函数调用图，从 `runContractCheck`、`collectSystemCrossCheckFindings`、
   `attachSystemCrossCheckAppendix` 三个生产根做传递闭包，任意层到达退役 conclusion provider 即失败。
   synthetic mutation fixture 明确钉住 `root → neutralWrapper → proseHeadlineElimFindings` 必须被捕获。

这保持“精确信号才可硬门”：AST 看护约束的是系统自己的静态所有权拓扑，不扫描用户输入、模型答案或具体
case 的期望措辞。显式时间窗 Trace 因果投影、自动补齐和模型结论权限均未改变。

验证：`go test ./internal/types ./internal/tool ./internal/orchestrator -count=1` 全绿，其中
`internal/types` 19.702s、`internal/tool` 160.569s、`internal/orchestrator` 11.982s；另有 OM-5
正反索引 pin、三类中性行 zh/en pin、错误算式 e2e、无 visible projection 拒悬空、wrapper mutation pin。

状态：`S4-3=closed`；`S4-4=closed`；`OM-5=closed`；`M5-D=closed`。下一批按冻结顺序进入
`S2-1/S2-2/S2-3` repomap 中危补修。

### §10.12 M5-E1：receiver 作用域权限与 bare fallback 排除权（已交付）

#### S2-1：Java 推断声明是“遮蔽事实”，不是“无事发生”

复核确认 `javaDeclaredTypeName(var)==""` 后，旧 `add` 会静默跳过该声明；文件内另一个显式同名声明仍可把
类型借给 `var` 局部变量。现保持“不从 initializer 猜类型”的精确信号边界，但把已出现的推断声明记为
冲突/遮蔽：删除同名 file-census authority，并令该冲突 sticky。因而 `var repo = new Other()` 不会铸造
`Other`，也不会借用另一作用域的 `Worker repo`；调用保持源码 receiver `repo`。Java extractor epoch 5→6，
旧暖缓存强制失效。

#### S2-2：Kotlin/Swift 从文件 census 收窄到最近词法作用域

确认旧 `navigationUniqueReceiverTypeBindings` 把参数、类参数、属性全部压成一张文件 map。典型反例是
`typed(repo: Worker)` 与另一函数的 `val/let repo = Other()`：无类型局部被静默跳过，后者的调用被改写为
`Worker.run`。现预编译 `(scope identity, binding) -> authority` 索引，调用从最近 function/lambda/closure/init
向外逐层查 class/object/protocol/root：

- 最近作用域显式唯一类型才可晋升 receiver；
- 最近作用域存在 inferred/untyped 或冲突声明时，明确阻断外层同名 authority，但保留源码 receiver；
- 参数不跨函数，局部不污染兄弟函数；函数内未遮蔽时仍可读取外层 class property；
- 索引构造 O(declarations)，每个调用只沿 AST ancestor 链查询，未退化为 calls×declarations 全扫描。

Kotlin epoch 5→6、Swift epoch 4→5。正反 fixture 同时钉住显式参数仍解析到定义端点、另一函数的 inferred
同名局部保持源码身份，以及 initializer type 不得冒充声明类型。

#### S2-3：bare function 不得获得 method caller 的排除权

审计结论准确，且代码与注释直接相反：`ResolveCallTarget` 注释规定 receiver 为空才匹配 bare function，旧实现却
在 receiver **非空**时 fallback 到同包同名 bare function。`CallersOfID` 随后把这个弱匹配当“已解析到另一个
symbol”，排除本应保守保留的 method caller。现删除该逆向 fallback：空 receiver 原本就由第一步 exact
`MethodKey{Receiver:""}` 解析；非空 receiver 只能由 exact/cross-package receiver key 解析，失败即保持
unresolved/name-level，不能取得排除权。回归构造同包 `ToolA.Execute` + bare `Execute` + `tool.Execute()`，
确认调用不被 bare symbol 抢占，且仍进入 `ToolA.Execute` 的保守 caller roster。

完整验证：`go test ./internal/tool/repomap/... -count=1` 全绿（8 个包）。本批不读取请求/模型/答案 prose，
不改变 read/write 路由、显式时间窗 Trace 因果投影、系统补齐或模型结论所有权。

状态：`S2-1=closed`；`S2-2=closed`；`S2-3=closed`；`M5-E1=closed`。

#### 同类扩域审计（新登记 S2-4）

按“所有受支持语言”继续扫 receiver 权限面后，Go 与 C/C++ 已按函数域绑定但尚不识别内层 block 遮蔽；
Python/JavaScript/Ruby/Lua 只保留源码动态 receiver，不铸声明类型；Java 以本批 sticky conflict 保守 fail-open。
另确认 TypeScript/ArkTS、Rust、
Cangjie 仍各有独立 file-wide typed census，存在同类跨函数授权风险，不能因 S2-2 只点名 Kotlin/Swift 就宣称
全语言闭环。登记 `S2-4=P1`，下一独立批把三种解析族统一到 lexical-scope authority，并逐域 bump cache epoch；
不把某一语言的 AST/token 规则复制给其他语言。Go/C/C++ block 遮蔽另登记 `S2-5=P1`，不得用“已按函数域”
错误收账。

### §10.13 M5-E2：TS/ArkTS、Rust、Cangjie 词法 receiver 权限（已交付）

#### 共用权限代数，不共用语言语法

新增语言无关的 `lexicalReceiverAuthorities`，只负责 `(scope,binding)` 索引、最近作用域查找、unknown shadow
和同作用域冲突语义；哪些 AST/token 是声明、哪些节点/brace 是作用域仍由各语言 extractor 自己决定。这样统一
修复“文件 census 获得跨函数权限”这一类问题，同时避免把 TypeScript AST 规则硬套给 Rust/Cangjie。

- **TypeScript/ArkTS**：参数、变量、field 归属最近 function/arrow/method/class；无类型局部在本作用域阻断
  外层同名类型，兄弟函数互不污染。共享 `extractJS`，但两个持久化域分别 bump：TypeScript 5→6、ArkTS 5→6；
- **Rust**：parameter/let 只归属最近 `function_item/closure_expression`。`field_declaration` 不再跨 struct/impl
  凭同名获得全文件授权；缺少精确 struct↔impl/field 证明时 `self.repo` 保持源码 receiver。Rust epoch 4→5；
- **Cangjie**：token lane 建 brace parent stack。函数/class 参数只装入其 declaration body brace；let/var/const
  装入当前 brace，未标注类型仍作为 shadow；调用从当前 brace 向 parent 查 authority。named argument 继续无权，
  跨函数同名参数/局部不再串线。Cangjie epoch 4→5。

四语言同构 fixture 均含 `typed(repo: Worker)` 与兄弟函数 inferred `repo`：第一条必须提升为 `Worker`，第二条
必须保持源码 `repo`。完整 `go test ./internal/tool/repomap/... -count=1` 八包全绿；cache floor pin 同步提升。

状态：`S2-4=closed`；`M5-E2=closed`。`S2-5`（Go/C/C++ 内层 block shadow）仍为下一独立批，未收账。

### §10.14 M5-E3：全静态语言 nested scope + declaration-order 闭环（已交付）

#### S2-5：函数域不是词法域

继续否证 §10.12 的“Go/C/C++ 已按函数域”后，确认参数绑定仍会越过内层 block：Go `repo := ...`、
C/C++ `Other repo` 均可能被外层 `Worker repo` 参数改写。现三域统一消费 shared lexical authority，但语言
声明规则仍独立：

- Go parameter/receiver 为 callable-wide；short declaration/range/var_spec 归属最近 block，inferred local
  只遮蔽、不猜 initializer 类型；
- C/C++ parameter 为 function-wide；block-local declaration 读取自身 declarator/type，归属 compound/for/
  if/switch/while/try/catch 最近边界；显式 `Other *repo` 可精确提升为 `Other`，不再退化为源码短名；
- epoch：Go 6→7，C 4→5，C++ 4→5。

#### S2-6：预编译 scope map 还必须尊重声明起点

冷读 shared carrier 又发现同一 block 合法 shadow 的时间轴风险：Rust 允许先调用参数 `repo`，再
`let repo: Other`；若只按 scope 聚合，声明前的调用也会被后声明改写。现 authority 条目原生携带
`declaration.StartByte + scopeWide`：参数/field/class parameter 从 scope 起点生效；local 只在 declaration
之后生效；同一位置的多声明冲突 fail-closed；查找先选最近 scope，再选该 scope 内已生效的最近声明。

该根修同步覆盖并以真实语法 pin 验证：

- Rust 同一 block 遮蔽前 `Worker`、遮蔽后 `Other`；
- TypeScript/ArkTS class field + nested statement block；
- Kotlin class parameter + nested lambda；Swift class property + nested closure；
- Go、C、C++ 外层参数 + 内层 block；
- Java 从旧 file-wide sticky conflict 升级到 field/class、parameter/callable、local/block 与 declaration-order
  权限：两个 class 的同名 field 可分别解析 Alpha/Beta，`var` 只遮蔽其局部作用域且仍不从 initializer 猜类型。

因此 §10.12 的 Java “全文件冲突后保守降级”只是 E1 止血形，现已被精确 scope 形取代。相关新输出再次 bump
持久化代际：Java 6→7、TypeScript/ArkTS 6→7、Kotlin 6→7、Swift 5→6、Rust 5→6。

#### 全语言权限矩阵收账

- 具备 typed receiver promotion：Go、Java、TypeScript/ArkTS、Kotlin、Swift、Rust、C/C++、Cangjie，均已
  绑定语言原生 lexical scope；
- Python、JavaScript、Ruby、Lua 为动态源码 receiver 车道，不从声明猜类型，不存在 typed census 越域；
- Proto 仅 declarative RPC relation，不存在 executable receiver；
- 所有 `SupportedReadLanguages()` 均在 call capability matrix，cache floor 覆盖每个持久化语言域。

完整验证：`go test ./internal/tool/repomap/... -count=1` 八包全绿。没有请求/答案关键词 hard gate，没有改变
Trace 因果投影、自动补齐、read/write 路由或模型结论所有权。

状态：`S2-5=closed`；`S2-6=closed`；receiver authority 全语言安全面 closed；`M5-E3=closed`。

### §10.15 M5-E4：端点 token 边界、eval 双向边界 pin 与低危清账（已交付）

#### M5-S5-4：跨语言分隔符等价不能退化为双向子串

复核确认旧 `callChainCodeTermMatches` 同时使用 `Contains(candidate, endpoint)` 与反向 `Contains`，并再对
tail 做一次子串匹配。它虽然偶然兼容 `Foo.run / Foo::run`，却也会把 `Other::run` 当成 `Foo.run`，以及把
`runWith` 当成 `run`；prefix sibling 或不同 owner 因而可以获得端点/跨度权限。

现由 `AnswerCodeIdentitySurfacesCompatible` 提供唯一 identity 代数：

- `. / :: / # / -> / / / \\` 只被归一成结构化 segment 分隔符，不做扁平字符串删除；
- short identity 可以对应 qualified identity 的最后一段，两个 qualified identity 则必须逐段完全一致；
- 每段必须是合法 code identity，空段、prose、空白与 path/location 拼接均不取得身份权限；
- `callChainEndpointCompatible` 与 evidence endpoint matcher 复用同一函数，删除各自的 tail/substring 旁路。

回归覆盖 Java/Kotlin/Swift/Go/C/C++/Rust/ArkTS/Cangjie 常见限定拼写等价、short↔qualified 正臂，以及
owner mismatch、prefix/suffix sibling、普通 prose 负臂。完整回归同时发现一个旧测试用 `gate.RunWith` 冒充
请求端点 `gate.Run` 才能构造 principal span；fixture 已改为 exact `gate.RunWith`，并用断开的 caller 保持原本
“member_set 不得铸造有向可达性”的测试目的。生产门没有为测试放宽。

状态：`M5-S5-4=closed`。

#### M5-F2：M4-K 的反向 runner 契约确实缺席

审计结论准确。旧 `TestEvalPrimaryAnswerTailBoundariesMatchRendererEmissions` 只遍历六个 production 标题，证明
“renderer 发出的标题都被 runner 消费”；若 `scope_primary_stdout` 额外加入一个生产端不存在的精确标题，测试
仍会通过，所以 §9.12 的“runner 新增不存在标题也会失败”属于过度声称。

现测试反向解析 `scope_primary_stdout` 的全部 `$0 == <exact literal>`，并与 production-owned closed set 比较：
六个 renderer citations/snippets/recovery 标题，加上 agent evaluator 仍发射的中英文 legacy raw-final 标题。
多一项、少一项、重复一项均失败。Trace projection 使用独立 regex 边界与既有 projection contract，不被误塞进
literal 集。至此 renderer→runner 与 runner→production 两个方向都有结构 pin，eval oracle 仍只在 opt-in eval
生效，不进入产品路由、模型上下文或答案修改链。

状态：`M5-F2=closed`；§9.12 的双向声明现与代码一致。

#### S6-2：当前 main 已由后续负控覆盖，无需再改路由

该 finding 对 `dba723a30` 时点成立，但当前主线已有两类独立正反 pin：真实 typed desktop operation（含
`operation_kind=desktop`）继续 `route=operation`；只有声明 current-source obligation 却携带泛化 computer-
operation 漂移的请求才降级为 repository pipeline。另有 dispatch 级回归固定历史生产漂移。因此不能再按旧
时点描述改路由，否则会误伤真实桌面目标操作。

状态：`S6-2=closed-by-current-main / tests-confirmed / no-code-change`。

#### M5-F3：B54..B71 继承债按当前主线重新盘点

“残留 10 项”是审计时点统计，不能在后续多批已交付后原样沿用。逐节对照当前账本，B54 的答案所有权项已
全部 closed；B55..B70 的确定性 code gaps 均已有 implemented/closed 记录，剩余多数是 replay-next 或模型波动
观察，不应伪装成待编码任务。当前仍明确 open 的泛化项只有：

1. `EVAL-B71-CASESCOPE1`（P1/eval-quality）：case 未声明 recursion/scope，动态主值缺 typed oracle；下一批最高
   ROI，先修 eval 载体，不进入产品答案硬门；
2. `EVAL-B64-COUNTBIND1`（P2）：pre-emit advisory 中 scalar 与 member_set 的 block/fact identity 归属仍可能
   错配；需要 typed identity 绑定，不能靠数值或答案词面相似；
3. `EVAL-B59-INVROW1`（P1/watch）与 `EVAL-B60-CLOSURECHURN1`（P1/watch）：现有单语言/单关系 witness 不足，
   先用异构回放确认可泛化根因；不得为了降低单 case reject 数放松 typed call/inventory authority。

施工顺序冻结：先 `B71-CASESCOPE1` 的 eval 基础设施，再严格并行 2 个异构 case 回放；若日志证明
COUNTBIND1 或 INVROW1/CLOSURECHURN1 跨 case 复现，再按 typed carrier 根因独立立批。显式时间窗 Trace 因果
投影、自动补齐、read/write 路由和模型结论所有权不因本清账改变。

状态：`M5-F3=re-audited / 4 explicit opens / replay debt separated from code debt`；`MERGE-AUDIT-5` 已确认
code finding 至此全部闭环，下一阶段转入 eval-quality 与异构回放。

### §10.16 继承债首批：B71 动态 scalar/scope eval authority（已交付）

按 §10.15 排序先处理 `EVAL-B71-CASESCOPE1`。新增能力完全位于 `eval/run.sh + runner_lib.sh`：case 以 typed
ID 声明动态计算 command、data-scope provenance、被校验 answer surface 与 `{{VALUE}}` binding regex；runner
在每轮实际 checkout（含 fixture/data/multirepo scratch）执行命令，要求单一 uint，并写独立 TSV receipt。动态值
缺席/陈腐、command 失败、scope/binding 不完整均只令 eval fail，不进入产品路由、prompt、成文 retry 或答案修改。

原 read combo case 现在明示 recursive scope，并从 checkout 动态统计 `internal/tool` 全子目录非测试 Go 文件；
不再把当前仓固定数值写入 case。正值、陈腐值、receipt provenance 与真实 `run.sh` 接线均有 shell contract pin。
详细设计与状态见 `eval_priority_campaign_audit_20260730.md` §75。

状态：`EVAL-B71-CASESCOPE1=implemented/eval-only/runner-contracts-pass`；下一步严格并行两个异构高优先级
case（write + operation），不重复上一轮 Trace/Read 对。

### §10.17 MERGE-AUDIT-5 当前主线独立复核 + B84 operation 边界补件（已交付）

更新并确认 `main@aa62cb906` 与 `origin/main` 零分叉后，对 §10 原五个高危及相邻所有权/跨语言面做生产实现与
定向测试复核：

- S6-1 通用 prompt 仅保留 typed evidence authority 边界，不再发射 Codrax 仓内部路径/函数；
- M5-S3-1 Rust lifetime/label/char 与额外 brace 正反 pin 通过；M5-S3-2 active PlanID + verify-failure generation 正反 pin 通过；
- M5-S1-1 class participant/sibling carrier 的 family-independent alias 权限、M5-S5-3 owner 绑定与 sequence operator 单源 pin 通过；
- M5-S5-1 typed ordered endpoint 是唯一方向权限；S4-3 中性 typed 并置与 wrapper AST 所有权 tripwire 通过；
- Java、TS/ArkTS、Kotlin、Swift、Rust、Go、C/C++、Cangjie 的 lexical receiver/shadow/declaration-order 矩阵 pin 通过。

定向并行包测试覆盖 `internal/tool`、`agent`、`writeflow`、`types`、`orchestrator`、`mermaidcompat` 与
`tool/repomap/...`，全部绿色；§10 的 12 个 confirmed code finding 可独立确认 closed，而非只按账面收账。

随后按 §10.16 严格并行 write + operation。write 全链路通过；operation 暴露新确定性补件
`EVAL-B84-OPSTRUCT1`：stringified `steps` 内部存在非法 nested JSON escape 时，旧 flexible decoder 把整个
command-object array 降格为 shell，lint/executor 又因 `json.Valid=false` 漏检，最终执行结构化文本。

本批以 schema-owned structural signal 根修，而非 case/URL/答案关键词：解析层对 malformed command-step container
触发 compact typed repair；operation lint/executor 共用同一识别函数并 fail-closed。合法 serialized steps、普通 shell、
shell test expression 均有正向 pin，直达 executor 负控证明命令未运行。`go test ./internal/operation ./internal/repl -count=1`
全绿。

状态：`MERGE-AUDIT-5-original-findings=independently-verified-closed`；
`EVAL-B84-OPSTRUCT1=implemented/tests-pass/replay-next`。没有新增请求/答案 prose hard gate，没有系统答案代写，
显式时间窗 Trace 因果投影与自动补齐权限不变。

### §10.18 B85 production replay：operation closed；inventory composite row identity 继续开放

`main@e014c06f4` 上严格并行 operation 原 witness 与 Cangjie source-inventory。两席 runner/human 主答案均 PASS：

- operation 80s 内完成首页→typed `user_guide.html` 两轮读取；terminal 把 complete receipt、20 pages、118,802
  visible runes、248,161-byte source identity 与目标 locator 绑定，`EVAL-B84-OPSTRUCT1=production-proven/closed`；
- Cangjie 53s、零 finalizer reject，五个 principal rows 的 family/package/file:line 与五条引用均准确。

Cangjie 同时再次见证 `EVAL-B59-INVROW1`：两个 `Cart` 分别是 class@14 与 extend@30，主表已区分，soft
principal-member checker 却产生 4 advisories，系统追加“部分证据较弱/补充钻取未执行”的失真不确定性。该问题应从
`source + line + declaration family/role` typed composite identity 修，不得从 `Cart`/`extend` 等模型答案词面或语言
关键词硬门。状态：`EVAL-B59-INVROW1=confirmed-repeat/root-audit-next`。

### §10.19 B85-C：inventory row identity + localization authority（已交付，待生产回放）

冷读纠正了 §10.18 的单根因表述。四条 advisory 由两层共同造成：aggregate principal member 先按
`source + label` 把 `Cart@14/@30` 合并为一个席位；随后 checker 又只认 `items[].citation_ref`，不认模型已写出的
markdown table `block.Text + exact file:line + citation pool`。而“补充钻取未执行”来自独立的 pre-finalize localizer：
source-inventory 单源 authority 已是 `mechanical_landing_ready/complete`，通用导航计划仍把未跑 `relation_map` 当 floor debt。

根修已完成：

- aggregate row 按 `source + line + label` 保留复合身份；definition evidence 的 equivalent-anchor 合并语义不变；
- principal markdown 覆盖由只读 matcher 以“成员 + 同块精确位置 + 同位置 citation”判定，前缀行号和 label-only 均失败；
- 禁止的 `normalizePrincipalSupportMemberCarriers` 没有接回 shipping path，模型答案与 citation_ref 均不被系统改写；
- generic localizer debt 仅在现有 typed authority 明确可机械落地且无 inventory followup 时抑制；需要 summary/source text、
  不完整/截断/分页清单仍保留 follow-up。

ownership tripwire、定向正负臂及 `internal/types`、`internal/tool`、`internal/orchestrator` 三包全量全绿。
状态：`EVAL-B59-INVROW1=implemented/tests-pass/replay-next`。下一步严格并行两个异构 eval，验证 advisory=0、无虚假
termination caveat，同时人工核对主清单与引用逐席完整。

### §10.20 B86 production：复合行 identity closed；construct family 扩域根修（已交付，待回放）

严格并行 Cangjie/ArkTS 两个 source-inventory case 后，B85-C 在 Cangjie production witness 上完整生效：五个复合行与
五条 citation 逐席一致，coverage advisory 与虚假 localizer caveat 均为零，`EVAL-B59-INVROW1=closed`。

ArkTS 暴露的新 P1 与 MERGE-AUDIT-5 的“精确信号作硬门”原则同根：parser typed row 已给出四组 `@Entry` 与两组
`@Builder` surface，但旧 family 铸造不接受单 marker/并列 marker，coarse `function/method` role 因而被误当成 requested
universe，系统强迫答案追加 20 个无关成员。现已将 family 从单值升级为 row-local 多值：parser marker 各自保留身份，
base+specific 仍折叠；candidate query、principal projection、complete lens、closure/gap 统一消费 exact family 交集。
无 marker 的 helper/lifecycle 不再因同 role 获权，普通无 family inventory 不受影响。

同时确认一个 eval-only 假绿：`entry_page` rowset ID 未绑定可见 `@Entry` section 时，count=4 退化为“命中四个期望行”，
没有拒绝额外 `EntryAbility`。已登记 `EVAL-B86-EVALROW1`，下一小批通过 case 显式 typed section anchor 修复，不把
`@Entry` 或其它答案关键词写进 runner。

验证：`internal/types` 全量、SourceInventory tool 族、`internal/tool/repomap/...` 全绿；convergence 旧文件 ceiling 均未
提高。状态：`EVAL-B86-SURFFAM1=implemented/tests-pass/replay-next`；`EVAL-B86-EVALROW1=eval-fix-next`。Trace 显式窗、
因果投影、自动补齐与模型结论所有权均未改变。

### §10.21 B86-E：inventory section 精确计数 oracle（已交付，待回放）

`EVAL-B86-EVALROW1` 已以 case-owned typed section label 修复：声明 exact row count 的 rowset 可显式绑定一个可见
markdown section；runner 只在该 section 内计数，且声明 section 缺失时 fail-loud，不再静默退回全文。section
边界按 heading level 计算，因此 H3 下的 bold 表格标题不会提前截断。

该能力完全位于 eval case/runner，不读取产品原始请求或模型答案来决定 shipping gate，也不改产品 prompt、答案或权限。
未声明显式 section 的既有 case 保持兼容。ArkTS 旧输出现稳定报
`inventory_count_mismatch:entry_page:got5:want4`，对应的 extra-row、missing-section 与 `run.sh` wiring pin 全绿。

状态：该 section-only 第一形已由 §10.22 收窄，固定标题不再是内容正确性的必要条件。显式时间窗 Trace 因果投影、
自动补齐与模型结论所有权均未改变。

### §10.22 B87 production：construct family closed；eval 格式过硬纠正

严格并行 ArkTS/Cangjie 后，两个产品答案均人工正确：ArkTS 从旧 20 行扩域与错误 `EntryAbility` 收敛为精确 4+2；
Cangjie 的五个 base+specific family rows、package、位置与 citation 无回归。两席均零 finalizer reject/advisory，故
`EVAL-B86-SURFFAM1=production-proven/closed`。

ArkTS 的记录 verdict 为 FAIL 仅因 §10.21 第一形要求固定 section 标题，而正确答案使用无标题有序列表。这是 eval
格式门过硬。已新增 case-owned `EXPECT_INVENTORY_ROW_MARKER_<ROWSET>` 等价载体：section 存在时按 section 计数；否则
只统计 terminal primary answer 内同时携带 marker 和精确 source location 的 markdown 清单行。renderer citations、
deterministic supplement 与普通 prose 均不能满足行身份；extra row 仍由 exact count 拒绝。

旧五行错答离线回放保持 `got5:want4`，本轮正确 4+2 答案离线回放 PASS；真实 runner wiring 与正负 contract pins 全绿。
状态：`EVAL-B86-EVALROW1=implemented/runner-contracts-pass/artifact-replay-pass`。该修复仅影响 eval 判定，不新增 shipping
prose gate，不触碰 Trace 因果投影、自动补齐或模型答案所有权。

### §10.23 B88：call endpoint prefix authority 根修 + count oracle 重签

严格并行调用链/多集合计数后，runner 2/2 PASS 但人工 0/2。最高风险项是结构化 citation identity：
`label=gate.Run` 可借用仅证明 `buildAnalysisIR -> gate.RunWith` 的 citation，因为公共 code-surface matcher 使用 substring。
图边 gate 已拒绝伪方向，item gate 却放行，最终答案把真实 `Run -> RunWith` 反写为 `RunWith` 包装/等价 `Run`。

现已把 snippet occurrence 收紧为完整 code token，把 endpoint 对齐收紧为 exact 或 exact symbol tail；prefix sibling 不再获权，
短名/限定名合法桥保留。定向 pin 与 `internal/tool` 全量全绿。状态：
`EVAL-B88-PFXCIT1=implemented/full-tool-pass/replay-next`。

计数 case 的旧 `2[4-6]` oracle 被 `grammar.go:26` 行号满足，已改为 checkout 动态计算并按类别绑定 3/5/30；
`EVAL-B88-COUNTORACLE1=implemented/eval-only/artifact-replay-pass`。另保留两个 P1：完整 typed row set 被系统追加泛化弱证据
caveat（`EVAL-B88-SUPPCAVEAT1`），以及调用链 518s/36 Explorer 轮的 task-window/closure churn。后续只从 typed
carrier/display authority 与 task provenance 根修，不扫描或改写模型 prose，不影响 Trace 显式窗、因果投影和自动补齐。

### §10.24 B88：source-inventory 前后 carrier 合同统一

复核 `EVAL-B88-SUPPCAVEAT1` 后确认是红线级合同冲突：Finalizer 教学与 deterministic reviewer 都要求 typed
`items[].label/cells`，pre-emit hard gate 却允许自由 `blocks[].text` 满足同一 principal member set。结果是前门通过、后门报
`principal_items=0/missing=30`，并向用户投影为错误的“证据较弱”caveat。

现已把 typed source-inventory 主清单硬门统一到结构化 item identity；自由正文不再确权，类别 block 防跨桶借位；非 inventory
narrative soft lane 保持兼容。系统不补写/删除/重排模型答案，缺行时仅把 typed obligation roster 交回模型自修。定向正负 pin
与 `internal/tool` 全量全绿。状态：`EVAL-B88-SUPPCAVEAT1=implemented/full-tool-pass/replay-next`。

两处旧 pin 固定了 summary/Markdown table text 冒充 typed rows 的旧行为，已改为相同用户可见内容的真实 structured rows；
这是合同重签，不是降低验证。未触碰 Trace query、时间窗、因果投影、自动补齐或模型结论权。

### §10.25 B88：source-inventory 请求路径 provenance 闭环（已施工）

`EVAL-B88-SCOPEPROV1` 已确认：analyzer 对用户明确目录执行了正确的 typed source_inventory prescan，但 IR 只有源码类别
scope，没有目录边界 carrier；RequiredFiles 又只接受请求中精确文件，导致合法目录请求在后续被退化成 repo-wide。
合并后的 observation 于是把探索 cursor 和全仓 source-class samples 当成主范围，产生无关补查与 16 轮 completion churn。

现新增系统铸造的 `SourceInventoryRequestedPathScopes`：仅接受“当前请求 verbatim MentionedEntity × analyzer-stage 成功
repo_lens tool scope”的 canonical 精确交集。它与 production/test/docs/auxiliary 的 `SourceScopeProfile` 正交；explore scope、
`.`、不匹配或越界路径均 fail-closed。repo-wide 判定、completion、follow-up、aggregate universe 和 principal row lanes
统一消费该 carrier，边界外行只进 audit，截断恢复只在原请求目录内继续。
当 EvidencePlan 同时含 prescan 文件样本时，请求目录边界优先，避免导航样本反向缩窄用户声明的完整目录范围。

实现按 convergence ratchet 拆入独立 path-boundary concern，未抬高旧大文件 LOC ceiling。定向 pin 已覆盖持久化、错误阶段、
不匹配、根 scope、补查范围及 sibling row 污染。状态：
`EVAL-B88-SCOPEPROV1=implemented/types+tool-full-pass/replay-next`（`internal/types` 18.730s，`internal/tool` 166.728s）。
没有 raw request/final prose 关键词硬门，没有系统代写答案；Trace 显式窗、因果投影与自动补齐权限不变。

### §10.26 B89 production：scope carrier 真实形补洞；call endpoint 第四例合同冲突

严格并行调用链与多集合计数后，计数主答案 3/5/30 人工正确，但耗时 667s、16 次 source lens、11 次 completion。
§10.25 的 producer 只接受 `MentionedEntities`；真实 analyzer 将完整目录保存在已通过 verbatim 校验的
`SourceScopeProfile.SourceQuotes`，entity lane 只留下 basename/文件，故请求路径 carrier 未铸成。已把 verified typed quote 纳入
analyzer-stage lens 的精确交集；不扫描 RawRequest，探索 scope/rationale 仍无权。`internal/tool` 全量 167.514s 通过。状态：
`EVAL-B88-SCOPEPROV1-R1=implemented/full-tool-pass/replay-next`。

调用链 case 则稳定复现新的 P0：同一 `Run -> RunWith` typed call 在限定 participant
`gate.Run -> gate.RunWith` 形被 body gate 拒绝，同时被 principal completeness 以裸形强制，连续六次成文 reject 后降级。
这证明先前 family/class alias 修复尚未统一“短 owner context → 限定 endpoint”的共享 resolver。
`EVAL-B89-CALLEDGEQUAL1` 已冻结为下一施工批：sequence body、sibling anchor、principal citation 三面共用 exact-first 的 typed
identity join，并以唯一 OwnerSymbol/source location/required endpoint 消歧；任何多位置、重载、异 owner 均 fail-closed。
不得通过模型输出关键词、语言名或本 case 函数名放行。

现已交付：call-site `OwnerSymbol` 保留 repomap enclosing callable 的 parser-owned package/module/receiver 限定身份；resolver
只有在 exact required caller、same-owner qualified endpoints、exact grounded OwnerSymbol、唯一 source:line 与裸 callee operation
同时成立时，才把短调用投影为限定展示。既有 unique-definition 臂未放宽；无 owner、异 owner、contrary object、多位置均拒绝。
`.`/`::`/`#` 与 14 个可执行语言 owner-stamp fixtures、完整 `internal/tool`（167.735s）全绿。状态：
`EVAL-B89-CALLEDGEQUAL1=implemented/full-tool-pass/replay-next`。

本轮未触碰 RootCauseTrace runtime relation authority、显式时间窗因果投影、自动补齐或模型结论所有权。

### §10.27 B90 production：限定边合同复验通过；typed 上下文仍有两处污染

严格并行两个 read case 后，`EVAL-B89-CALLEDGEQUAL1` 的 production 结论是“实现成立”：错误反向边被拒、缺失 principal
边被拒、最终 `gate.Run -> gate.RunWith` 限定形通过，旧不可满足合同没有复发。不能把这两次真实事实修正记成 validator 嘈声。

但 runner PASS 的调用链答案人工仍判 FAIL：typed graph 与最终图均表明 `buildAnalysisIR -> gate.RunWith` 和
`gate.Run -> gate.RunWith` 是汇入同一 callee 的并列关系，正文却宣称前者间接到达后者。根因是 no-directed-path boundary
已经铸造，模型先前自报的“完整调用链” aggregate/summary 仍以 principal 形进入 Finalizer，形成冲突上下文。登记
`EVAL-B90-CALLBOUNDCTX1=P0/confirmed`，只能从 typed aggregate authority/上下文单源修复，禁止扫描或系统改写模型正文。

现已实现答案权威投影：raw accepted closure/aggregate 仍保存在 Mutable/TurnA 审计态；typed no-path 激活时，Finalizer plan 不再
消费无方向 member_set、当前/旧窗口 closure prose，成功 tool result 只发布 exact endpoint boundary。grounded call triples 仍在，
模型继续拥有最终结论。`internal/types`、`internal/agent`、`internal/tool` 全量全绿。状态：
`EVAL-B90-CALLBOUNDCTX1=implemented/full-pass/replay-next`。

计数 case 再次证明 §10.26 的可选 quote 补洞不稳：同一真实 analyzer 本轮未把目录保存在 quote/entity，carrier 未铸成，
10 次 lens 后仍漏掉当前第 5 个 production 导出函数。登记 `EVAL-B88-SCOPEPROV1-R2=P1/confirmed`：使用 analyzer-stage
成功 scope × 当前请求 lexical exact path identity 铸造边界；这是完整 canonical path 的 token-boundary 比较，不是关键词/语义
扫描。wrong-stage/root/unmentioned 均继续拒绝。`EVAL-B90-INVENTORYFRESH1` 随 scoped complete roster 一并验收。

R2 已实现：共享 exact-entity boundary matcher 只核对 typed observation scope 的完整路径 token；可选 analyzer entity/quote 缺席
不再丢边界，前后缀碰撞、explore/root/unmentioned 仍 fail-closed。`internal/types` 与 `internal/tool` 全量全绿，LOC ratchet
保持。状态：`EVAL-B88-SCOPEPROV1-R2=implemented/full-tool-pass/replay-next`。

两案均未改变 Trace 显式窗、因果投影、自动补齐和模型结论所有权。

### §10.28 B91 production：scope 坐标丢失、test 污染与调用证据处理次序

`main@0a7e7838e5ba` 严格并行两个 read case 均失败：sequence 563s/6 次 Finalizer reject；bounded inventory 1201s 超时/
16 次 lens。详细工件与人工结论见 eval campaign §95。

新增三项高危与一项中危：

1. `EVAL-B91-SCOPECOORD1=P0/confirmed`：selected-sub-repo 把 operational scope 归一成 `.`，observation 未保留 repo-root
   query coordinate，故 exact request-path carrier 无法铸造并把全仓截断债误套到目标包；
2. `EVAL-B91-SOURCECLASS1=P0/confirmed`：typed inventory 没有 production/test source class，5 个生产函数与 51 个测试入口
   合成 function=56，Finalizer 被硬合同强制发布测试符号并超时；
3. `EVAL-B91-CALLGROUNDORDER1=P0/confirmed`（纠正原 `CALLEDGEWIRE1`）：`gate.go:135` 的真实 evidence 是
   `grounding_status=recovered`，diagram gate 拒绝正确；系统先按 caller 锚 grounding、后规范 call direction 且未重跑 grounding，
   Finalizer handoff 又隐藏 recovered 状态，才造成“权威边被误拒”的假象；
4. `EVAL-B91-WAIVERRATIONALE1=P1/confirmed`：no-path typed success summary 后仍拼接模型自由 rationale，审计态与答案态
   隔离不完整；
5. `EVAL-B91-RANGECLOSURE1=P1/confirmed`：Explorer DAG 节点用 current-window replacement 覆盖 run 级 read ranges，持久快照
   丢失早期 `gate.go:134-233`；这不是第 3 项的直接原因，但会污染后续覆盖/闭包权限。

施工按 B91-A/B/C 三批冻结。B91-B 改为前置 parser-backed call canonicalization、明确 handoff grounding 权限并累计 run 级 read
ranges，绝不放宽 diagram resolver。范围载体来自工具 query provenance，来源类别来自 parser/index；禁止以用户、模型或答案原文关键词
作 hard gate。Trace 显式时间窗、因果投影、自动补齐与模型结论所有权不变。

B91-A 已交付：新增 engine-derived repo-root `QueryPathScopes`，与 operational sub-repo `Scopes` 分轴；request authority 仍需
analyzer-stage 成功 provenance × 完整 canonical path current-request exact identity。no-path 自由 rationale 只留 Mutable 审计态，
不再进入 completion note/tool summary。LOC ratchet 未提高；`internal/types`、`internal/tool/repomap`、完整 `internal/tool` 全绿。
状态：`SCOPECOORD1 + WAIVERRATIONALE1=implemented/full-pass`。

B91-B 已交付：parser-backed call direction/anchor 规范化前置到首次 grounding，caller-shaped 锚在已读 same-line relation 上直接
落为 grounded；diagram resolver 保持 fail-closed。Finalizer handoff 显式携带每条 evidence ref 的 grounding 权限和汇总，不再把
recovered 行展示成统一 accepted 权威。Explorer read set/ranges/totals 改为同一 run 内单调累计，消除后续 DAG 窗口覆盖早期读取。
`internal/types`、`internal/agent`、`internal/tool`、`internal/orchestrator` 全量全绿。状态：
`CALLGROUNDORDER1 + RANGECLOSURE1=implemented/full-pass`。

B91-C 已交付：生产证据表明 typed row projection 已选出 5 个 production functions，超时的直接机制是旧等价判定只要求
`wanted ⊆ model`，使包含 51 个 test entry 的 56 行 model superset 仍获 principal authority。现由 index 为每个 inventory
candidate/attribute/observation row 铸造并贯通 `SourceClass`，模型查看的每行也显示该 typed lane；旧持久行仅按 canonical file
path 兼容回填。受 scope/source-class/family 约束的 principal fact 比较改为规范 row-key 集合严格等值，out-of-universe extras
不能再变成硬发布义务；默认 repo-wide 与显式 auxiliary universe 不变。

新增生产 lens、legacy fallback、混合 production/test superset、mixed-role 与 explicit-universe 正负 pin，并按 convergence
要求把转换逻辑拆出独立 concern，既有大文件 ceiling 未提高。状态：`EVAL-B91-SOURCECLASS1=implemented/tests-pass/replay-next`；
待 B91 两例严格并行回放验证 lens/retry/时长收敛。未扫描用户/模型 prose，未触碰 Trace 因果投影和模型结论所有权。

### §10.29 B92 production：source class 隔离成立，完整枚举仍被隐式 selector 清空

`main@b77e6fa9c768` 严格并行两例均人工 FAIL。B91-C 已证明能把 `_test.go` rows 从 production principal roster 隔离，旧
56-function 强制发布未复发；但 inventory profile 的 compound role universe 被 enum-only facet 无条件折叠，且 source lens 将
空 query 自动补成分类 quote + analyzer prescan entities，5 次均返回零 rows，最终仅列 2/5 production functions。

新增 `EVAL-B92-INVENTORYSELECT1=P0/confirmed`：完整 category enumeration 的隐式 quote/entity token query 必须退役，显式 tool
query、typed roles/scopes 与 parser-owned surface family 保留；enum string+const facet 只可在 type(+support constant) 单一 role
universe 生效。全部判定只消费 typed profile，不读 request/final prose。sequence 另确认
`EVAL-B92-DIAGIDENT1=P1/confirmed`：同 canonical participant 的重复 alias 让正确 call anchors 被误报 missing，应先发射精确
duplicate-identity 修复再做 edge gate，不能放宽 evidence authority。

详细数据与人工判断见 eval campaign §96。Trace 显式窗、因果投影、自动补齐和模型结论所有权均未改变。

B92-A 已施工：category enumeration 的 repo_map 默认查询与 advisory snapshot 查询统一不再消费 quote/entity 作为隐式 member token；
显式 tool query 保持。typed quote/entity 只能与 parser-owned `SurfaceTerms` 相交后扩展/筛选构造族，覆盖语言由 parser 能力矩阵自然
决定。enum facet 新增 raw-role 适用域判定，compound roles 不再被误折叠，numeric const-qualified type 兼容语义保留。新增复合
3/5/2 production roster、test 排除、显式 query、ArkTS surface、snapshot 与 convergence pins。状态：
`EVAL-B92-INVENTORYSELECT1=implemented/full-pass/replay-next`；`internal/types`、`internal/agent`、`internal/tool/repomap`、
`internal/tool` 全量通过（tool 167.338s），不涉及 Trace 或系统答案代写。

B92-B 冷读补正并施工：生产稿用 canonical symbol 锚 diagram alias，本就违反 schema 的 verbatim node-ID 合同；不能把全部
`missing_call_anchor` 归因于重复 alias。确认的新缺口是同一 citable typed endpoint `gate.RunWith` 以 `RW/RW2` 两个实际 call
participant 出现时缺少先验 identity 诊断，误导模型继续改方向/造 `RunWith2`。现先发射精确
`duplicate_participant_identity`，要求复用一个 alias 且 body/anchors 同 ID，然后继续原 evidence gate。仅 exact typed call endpoint +
两个参与 call 的 alias 触发；class/actor + 不同 operation、unused declaration 保持可用。sequence/call_dag、自由函数及多语言限定符
均有正负 pin；RootCauseTrace 不进入此合同。diagram 定向矩阵与 `internal/tool` 全量通过（172.707s），状态：
`EVAL-B92-DIAGIDENT1=implemented/full-pass/replay-next`。

### §10.30 B93 production：bounded inventory 答对但作用域债扩散；no-path waiver 接线存在可执行矛盾

`main@0d4cf7329907` 严格并行两个 read case 后，inventory runner/human PASS，sequence runner/human FAIL。详细证据与人工审计见 eval campaign §97。

- `EVAL-B92-INVENTORYSELECT1` 已获 production 正证：目标包 3/5/30 roster 与 const-block 依据均正确；
- 新 P0 `EVAL-B93-SCOPELINEAGE1`：bounded complete lens 与先前 repo-wide incomplete lens 被合并成无 identity 的复合 observation，completion 把全仓缺页/source-class debt嫁接到目标包，造成 16 次 lens、28 个 Explorer 轮；
- 新 P0 `EVAL-B93-WAIVERWIRE1`：模型三次将 `principal_span_waiver=no_directed_path` 放入 stringified aggregate tail；兼容解码器恢复 aggregate 与部分 sibling，却不恢复 waiver，随后 gate 又要求相同 waiver，形成确定性不可满足重试；
- 新 P1 `EVAL-B93-CALLIDENT2`：parser-grounded package call 的短 Subject/Object 与限定 participant 展示身份不对称，Finalizer 在六次重试中持续抖动；
- `EVAL-B92-DIAGIDENT1` 本轮未形成两个 active duplicate aliases，故没有生产触发证据，保持 implemented/full-pass/replay-next。

优先级冻结为先修 WAIVERWIRE1（小而确定、直接消除自矛盾），再修 SCOPELINEAGE1（最高时长/context ROI），最后统一 CALLIDENT2。所有方案只消费 schema/engine/parser typed carrier，不扫描用户或模型原文作 hard gate，不放宽调用事实，不触碰 RootCauseTrace、显式时间窗因果投影、自动补齐或模型结论所有权。

WAIVERWIRE1 已交付：兼容解码的 schema sibling 集合补齐两个 typed waiver family 与 clear flag；top-level family 优先，尾部互斥/非法值仍走原 validator。生产同形、顶层优先、clear+new 冲突、unknown nested field 与既有 no-path 正臂均有 pin，完整 `internal/tool` 172.639s 通过。状态：`implemented/full-tool-pass/replay-after-scope-batch`。

SCOPELINEAGE1 已交付：complete lens 新增仓库根 `query_path_scopes`，selected-subrepo 的回基
坐标同时写入返回值与 durable carrier；合并归一化不再从全局 union 二次铸造 complete
lens。两条 follow-up authority 共享同一 typed 闭合谓词：requested path × 全部 principal
roles × executable tool provenance × `count=total` 全满足才消除旧 root debt；路径错位、
analyze-only、缺角色、partial count 与 repo-wide 请求均 fail-closed。完整 `internal/tool`
161.762s 通过，既有 LOC ceiling 未抬高。状态：`implemented/full-tool-pass/replay-next`。

CALLIDENT2 已落地定向修复：共享 call diagram resolver 现在消费 grounding 后的 parser-owned
`OwnerSymbol`，允许限定 caller 与同行短 callee 的无损 presentation 对齐，visible edge、
anchor 与 principal completeness 不再各自抖动。无 owner/错 owner/跨 owner/多位置负臂保持
fail-closed；实现不扫描 request/model/final prose，不按语言或案例特判，RootCauseTrace 仍
在该 source-call 合同之外。完整 `internal/tool` 174.463s 通过。状态：
`implemented/full-tool-pass/replay-next`。

### §10.31 B94 production：两项 B93 根修生效，另确认 complete-lens parity 与 code-mark identity 断线

`main@3907d9995` 严格并行两个 read case，runner/human 均 FAIL。详细工件与人工判断见 eval campaign §98。

- `SCOPELINEAGE1` 生产生效：bounded inventory 从 B93 的 16 lens/28 Explorer/405s/42% context 降至
  5 lens/8 Explorer/205s/23%，无关全仓债务扩散消失；
- `WAIVERWIRE1` 生产生效：top-level `no_directed_path` 当轮被接受并发布 exact boundary；
- 新 P0 `EVAL-B94-LENSPARITY1`：typed complete lens 已有 30/30 constant rows，模型 24 行 member_set 仍被 completion/Finalizer
  接受。requested-path lineage 尚未接到 principal row projection；
- 新 P0 `EVAL-B94-DIAGRAMCODEMARK1`：Mermaid participant 的合法 inline-code label 未做 presentation normalization，反引号进入
  endpoint identity，13 条 grounded 真边被成批误拒，7 次成文 reject 后无答案；
- 新 P1 `EVAL-B94-CALLFANOUT1`：no-path boundary 闭合后仍有 4 dispatch/34 Explorer/19 midloop 的同端点重复调查，先按
  task-node/generation 收窄根因，不做关键词 hard skip。

施工冻结为 B94-A 请求绑定 complete-lens row parity、B94-B Mermaid exact code-wrapper normalization，之后恰好双例回放。
两案只消费 typed lens/parser/diagram carrier，不改写模型结论，不放宽 call evidence，不触碰 RootCauseTrace、显式时间窗因果投影或
自动补齐。

B94-A 已交付：请求绑定 complete lens 只有在 requested path、全部 principal roles、可执行 tool-query provenance、源码类别、语言、
surface family、`count=total` 与 typed row count 全部精确闭合时，才可驱动 principal row projection，旧 broad incomplete 状态不再
压掉同窗 30/30 roster 的机械等值校验。八类错 lineage 继续 fail-closed，completion landing M4 pin 与完整 `internal/types`
（21.943s）、`internal/tool`（162.979s）均绿；relation/repo-wide/Trace 权限面未改。状态：
`EVAL-B94-LENSPARITY1=implemented/full-pass/replay-after-B94-B`。

B94-B 已交付：Mermaid declaration 的共享 presentation normalizer 仅移除完整包裹单一合法 code identity 的一对反引号；失衡、prose、
多 token 与嵌套包装不做猜测。sequence/call-DAG body、sibling anchor、principal completeness、duplicate identity 共用该结果；14 种
可执行语言及自由/`.`/`::`/`#` 形全绿，反向边与无证据边仍拒绝，RootCauseTrace 入口隔离不变。完整 `internal/tool`
161.197s 通过。状态：`EVAL-B94-DIAGRAMCODEMARK1=implemented/full-pass/replay-next`。

### §10.32 B95 production：B94 correctness gate 生效，但前置归一化与 DAG closure 仍不可恢复

`main@a58ee0d06` 严格双例回放得到 sequence runner PASS/human FAIL（496s）与 inventory TIMEOUT（1500s）。B94-A 已阻止 24/30
错 roster 静默通过，B94-B 旧 code-mark 批量误拒未复发；但确认两个 P0：

- `EVAL-B95-PRENORMPARITY1`：模型已携带 30 个 Kind 名称却保留 value=24，aggregate Normalize 在 request-bound typed projection 前
  先拒绝，精确 roster 无法提供安全机械校准，最终形成 10 次 completion reject；
- `EVAL-B95-DAGCLOSURE1`：首节点已闭合的 bounded inventory/no-path endpoint credential 不跨 sibling DAG node 复用，后节点重新挂回
  root debt；inventory 扩至 34 lens/65 Explorer/9 prune，sequence 也重复读取同一 endpoint 区域。

另记 P1 `EVAL-B95-ENDPOINTFOCUS1`：巨大同层 call roster 淹没 typed endpoint boundary，最终答案写“gate.Run 若存在”并遗漏已证
`gate.Run -> RunWith` 包装边。优先顺序为安全 pre-normalize parity → durable DAG closure receipt → 异构回放后再裁 endpoint capsule；
三案均禁止 request/model/final prose 关键词 hard gate，禁止系统代写结论，Trace 权限面保持隔离。

B95-A 已交付：completion 只在 request-bound executable complete-lens roster 精确闭合时，才于 aggregate Normalize 前校准与该 roster 有
typed overlap 的 principal inventory member_set 机械 count；随后 exact row projection 仍独占 principal universe。无路径权限、inexact lens、
relation、非 principal、disjoint 与非法 value 均不修。生产同形/负矩阵/LOC ratchet 及完整 `internal/types`（22.105s）、`internal/tool`
（170.076s）全绿。状态：`EVAL-B95-PRENORMPARITY1=implemented/full-pass/replay-after-DAGCLOSURE1`。

B95-B 冷读修正：跨 sibling completion/waiver/lens carrier 并未丢失，无需新建平行 receipt。sequence 的 accepted completion 后，ParseOutput
基于 chain subject ranker 的 `best<0.4` 嘈声分数重新铸造 `RepairRebindSubject`，旧 accepted-closure gate 错把它当 hard blocker，才继续
dispatch evidence/validate。现保留其探索期 principal guidance，仅在 accepted-closure 专用边界禁止它重开 DAG；精确 read/origin/handoff/
view 合同不变。生命周期 e2e、边界正负 pin 与完整 `internal/types`（24.285s）、`internal/orchestrator`（13.182s）全绿。状态：
`EVAL-B95-DAGCLOSURE1=implemented/full-pass/replay-next`；不涉及 Trace 或系统答案代写。

### §10.33 B96 production：closure 修复获证；source-class 完整性与 pre-emit 重算成为 P0

`main@80d936fec` 严格并行两个 read case 后，sequence runner PASS/human FAIL（239s），inventory runner/human FAIL（953s）。

- B95-B 已获 production close：sequence 从 3 dispatch/27 Explorer/16 reads 降至 1/13/4；inventory 同样仅一次 Explorer dispatch；
- 新 P0 `EVAL-B96-CLASSPARTITION1`：完整 function lens 把 5 个 production 与 51 个 test 行合成 count=56，而默认主席 typed rows 为
  production 5 行；exact parity 因 56!=5 fail-closed，模型最终错误发布全部 56 个 function；
- 新 P0 `EVAL-B96-ANSWERPREEMITPERF1`：89-item 文档的 item/citation 校验反复重建 surface plan、stable facts、typed exclusion candidate
  census 与 principal refs；sample 证明全 graph 扫描位于内层热路，单例 953s、物理内存约 1.1GiB；
- P1 `EVAL-B95-ENDPOINTFOCUS1` 在去掉 sibling 重探后仍复现：typed wrapper 边已在 evidence，但 final context 没有请求端点边界胶囊，答案仍称
  `gate.Run -> RunWith` 关系待确认并把 sibling fan-out 当主链。

高 ROI 顺序冻结为：先由完整 observation 铸 `role × source_class` complete-lens 分区；再把 immutable typed surface/exclusion/aggregate 派生缓存在
单次 pre-emit context；最后提供 request-bound endpoint typed capsule。均不扫描用户/模型/答案 prose 作 hard gate、不系统代写结论、不改变
RootCauseTrace、显式时间窗、因果投影或自动补齐。

B96-A 已交付：一次 complete role observation 现在保留 combined lens，同时在全部行 source class 可由 typed 字段/规范路径确定时铸造
`role × source_class` 子 lens；production/test/all 请求分别消费自身 5/51/56 宇宙。unknown-class、partial/page/merged-union 不获分区权限，
scope 路径也不能把 test 子 lens 污染成 production。完整 `internal/types`（22.813s）、`internal/tool`（166.054s）及 49-LOC 新文件收敛
ratchet 均绿。状态：`EVAL-B96-CLASSPARTITION1=implemented/full-pass/replay-after-B96-B-C`。

B96-B 已交付：单次 `preEmitCheckContext` 惰性复用 immutable surface plan、stable aggregate facts、principal refs 与 source-inventory authority；
sample 命中的 item×citation 热路不再重复全图 exclusion census。缓存不跨 patch/turn/tool call，文档 mutation 与所有 hard checks 仍逐项执行。
128-item 基准约 9.3ms→1.45ms、6.64MB→0.73MB、136k→23.2k alloc；完整 `internal/tool`（168.932s）通过。状态：
`EVAL-B96-ANSWERPREEMITPERF1=implemented/full-pass/replay-after-B96-C`。

B96-C 已交付：completion 私有调用图判定上移为共享 `types.AnalyzeCallChainEvidenceGraph`，完成门继续消费完全相同的 citable current-source
`ClaimCallEdge` reachability；finalizer 在 typed `no_directed_path` 边界下额外收到 bounded evidence capsule，能看到正向、反向、并行汇合或分离
frontier 的真实边、EID 与 source:line。该胶囊只提供证据，不代写/替换模型结论，不增加成文 hard gate，不读用户或模型 prose；definition、recovered、
runtime artifact 与歧义短名均不能越权铸边。Java/C/C++/Rust/ArkTS/Cangjie 等共享 `.`/`::`/`#` 语义，`QFRootCauseTrace` 负 pin 隔离。
完整 `internal/types`、`internal/agent`、`internal/tool` 通过；状态：`EVAL-B95-ENDPOINTFOCUS1=implemented/full-pass/replay-next`。

### §10.34 B97 production：B96 authority 生效，暴露 no-path admission 与请求坐标断线

`main@f4f0751fd` 严格并行两个 read case，runner 均 PASS、人工均 FAIL。sequence 236s/11 Explorer/4 Finalizer reject；inventory
558s/24 Explorer/15 lens/8 prune。详细工件与裁定见 eval campaign §101。

- B96-C endpoint capsule 已按设计输出 `endpoint_unresolved` 和 `buildAnalysisIR -> gate.RunWith` 的精确 typed frontier；但 completion 在未读取
  `gate.go`、exact sink 无任何存在性证据时仍接受 `no_directed_path`。新 P0 `EVAL-B97-CALLENDPOINTPROOF1` 要求 waiver admission 先由
  citable current-source call/definition typed evidence证明两端精确存在；缺证据必须定向补读，不能把“没查”铸成“无路”。
- B96-A 分区 lens 已正确选出 production 5/test 51，B96-B 也消除了成文热路；但 analyzer selected-subgraph prescan 丢失 repo-root
  `QueryPathScopes`，未铸 request-bound path。Explorer 被迫先做全仓 lens，旧 root debt 污染后续 89-row complete scope。新 P0
  `EVAL-B97-REQUESTBOUNDARY1` 在 tool parameter/query 坐标层统一 `path=<scope>` 与冗余同值 `scope=<scope>`，保留 analyzer-stage 精确 provenance。
- P1 `EVAL-B97-DIAGRAMALIAS1` 暂列观察：短 operation alias 与 qualified endpoint 造成成文重试，但本轮可自修复，需跨语言异构证据后再动共享 resolver。

修复顺序冻结为 endpoint existence admission → analyzer request-bound query coordinate；均只消费 typed evidence/provenance，不扫描模型/答案原文，
不系统代写结论，不影响 RootCauseTrace、显式时间窗因果投影与自动补齐。

B97-A 已交付：`no_directed_path` 不再仅凭 enum+rationale 越过 exact sink 未查缺口。新的 typed existence analysis 要求两个请求端点各自由 citable
current-source call node 或精确 definition 证明；definition 不进入 reachability，prefix sibling、歧义短名、recovered/runtime 均无权。
缺端点时发布定向定位/读取/发证据 repair，两端已证后仍由模型拥有 no-path 结论。15 种语言 identity 矩阵、生产同形与负臂通过，完整
`internal/types`（23.964s）、`internal/tool`（168.462s）全绿。状态：`implemented/full-pass/replay-after-B97-B`。

B97-B 已交付：selected graph 的执行 `scope` 与 repo-root `QueryPathScopes` 已分轴。`path=<scope>` 并冗余重复同一 repo-root scope 时，工具只用
精确路径边界把执行坐标转为 `.`/suffix，再由既有 rebase 把 typed provenance 恢复到 workspace 根；root、已相对值、前缀碰撞和未获 analyzer
provenance 的范围均不变。生产同形证明 observation 与 complete lens 都保留 exact requested path，prescan 可继续铸 request-bound authority。
完整 repomap/types/agent/orchestrator/tool 全绿（2.198s/19.794s/3.019s/14.221s/171.294s）。状态：`implemented/full-pass/replay-next`。

### §10.35 B98 production：B97 修复生效，确认两个新 P0 与一个 P1

`main@7fd42889d` 严格并行两个 read case 后，sequence runner PASS/human FAIL（231s），inventory runner/human FAIL（572s，4 次成文拒绝并降级）。
详细工件、日志证据和方案见 eval campaign §102。

- B97-B requested path coordinate 已闭环：bounded inventory 不再扩散为 repo-wide debt，15 次 lens 降为 6 次，并正确获得 3/5/30 production roster；
- B97-A endpoint existence admission 已生效：未读 `gate.Run` 时 completion 被拒，读取并发射 exact definition 后才接受 no-path；但 wrapper
  `gate.Run -> RunWith` 未作为 call edge 发射，最终答案仍写反方向，故新增 P1 `EVAL-B98-ENDPOINTTOPOLOGY1`；
- 新 P0 `EVAL-B98-SCOPEAGGREGATE1`：test-only model aggregate 与 production canonical row keys 不相交，现投影把它当独立 principal family 保留，
  导致 typed hard gate 强迫生产公开 API 答案加入 51 个测试函数；
- 新 P0 `EVAL-B98-REPAIRSHAPE1`：成员覆盖硬门需要 principal surface + enumeration facet + member identity + row-local citation，但修复提示只披露
  label/cell，且 roster 的集合 `label` 容易被误读为 item label；模型四轮无法构造满足合同的载体。

优先顺序为范围外 aggregate typed 降级 → 让硬门完整披露自身 schema recipe → endpoint-local topology typed debt。所有判定仅消费 request profile、
source class、support refs、call edges/read coverage 等 typed carrier，不扫描 request/think/final prose，不让系统代写结论，不触碰 RootCauseTrace、
显式时间窗、双轴根因、因果投影或自动补齐。

B98-A 已交付：模型发射的 disjoint principal member set 只有在 active canonical set 确有 principal 行、全部成员都由 row-local location 唯一映射到请求角色 observation、source class
全部已知且全部位于 typed `PrincipalScope` 外时，才降为 supporting coverage。混合、缺位置、歧义、unknown 与 all scope 均 fail-closed；不读取
label/request/answer prose，不按语言或符号名特判。Go test 与 Cangjie fixture 生产同形、负矩阵、独立 140-LOC ceiling 及完整 `internal/types`
（23.721s）、`internal/tool`（171.359s）全绿；旧 projection ceiling 未抬高。状态：
`EVAL-B98-SCOPEAGGREGATE1=implemented/full-pass/replay-next`。

B98-B 已交付：source-inventory member coverage 的 hard 判定未变，拒绝提示在 bounded roster 前完整列出 principal carrying block、enumeration
facet/claim-use、item label/cell identity、`set_label` 仅作集合键、row-local citation_ref 五项 schema。生产同形证明 section 只漏
`surface_role=principal` 时，真实 pre-emit dispatcher 携带该 recipe，按提示补一个 typed 字段即可通过；系统不自动改写文档或模型结论，提示
prose 也不参与 gate。完整 `internal/tool`（169.629s）通过。状态：
`EVAL-B98-REPAIRSHAPE1=implemented/full-pass/replay-next`。

B98-C 已交付 P1 soft-debt 形：endpoint existence proof 分为 unproven/ambiguous/definition-only/call-edge/definition+edge，finalizer typed capsule
明确 `definition_only` 只证存在、incident call evidence 尚未发射，不能据此声称 leaf/无调用或翻转方向；QFCallChain Explorer 软指南要求把 bounded
read 中真实 endpoint-local reverse/parallel/disjoint edge 一并发射。无新 hard gate、无 definition/prose 推边、无系统答案代写；15 种语言、
definition+edge、歧义与 runtime/recovered 负臂及 RootCauseTrace 隔离通过。完整 types/agent/orchestrator/tool 全绿
（18.809s/2.689s/11.330s/170.313s）。状态：`EVAL-B98-ENDPOINTTOPOLOGY1=implemented-soft-guidance/full-pass/replay-next`。

### §10.36 B99 production：B98 三项获得正证，新增上下文污染与 patch 引用漂移

`main@d6a3cae04` 严格并行两个 read case 均 runner PASS、人工 FAIL；详细证据见 eval campaign §103 与 B99 manual audit。

- B98-A 关闭：production 3/5/30 roster 未再混入 test-only aggregate；
- B98-B 关闭：完整五步 repair recipe 将旧四次失败/降级收敛为一次 reject、一次 patch 成功；
- B98-C 获 production 正证：Explorer 发射 `gate.Run -> RunWith`，模型最终正确说明它与 `buildAnalysisIR -> gate.RunWith` 是并行汇合，系统未代写结论；
- 新 `EVAL-B99-INVENTORYCONTEXTHYGIENE1`：source-inventory subtopic 已清洗，但自由 analyzer keywords 仍携未验证 `iota` 猜测，后续被重复写入 evidence/aggregate/final；
- 新 `EVAL-B99-PATCHCITATIONDRIFT1`：patch 插入 endpoint row 后把 inherited citation pool index 当行序平移，14 条旧 edge item 错引仍通过。

B99 两项已交付。机械 source-inventory keywords 现在只由 validated source quotes + typed roles/fields 构造；不扫描答案、不参与 hard gate。
patch 引用修复只接受稳定 block/item ID、除引用外完全相同、inherited pool、ref 差值等于真实 row delta 的精确合取形；新条目、显式同位改引和
replace-citation 均不动。两项都只修上下文/证据元数据，不修改模型结论，不进入 RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。

完整 `internal/tool`（166.474s）通过；eval oracle 已补范围机制与代表 edge citation 绑定，防止同类 runner 假绿。状态：
`EVAL-B99-INVENTORYCONTEXTHYGIENE1=implemented/full-tool-pass/replay-next`；
`EVAL-B99-PATCHCITATIONDRIFT1=implemented/full-tool-pass/replay-next`。

### §10.37 B100 production：B99 context hygiene 关闭；确认 visible-carrier 与 endpoint-admission 双 P0

`main@fc2d9faa5` 严格并行两个 read case，runner/human 均 FAIL；详细证据见 eval campaign §104 与 B100 manual audit。

- B99 inventory context hygiene 获 production close：Analyzer、Explorer、evidence、aggregate 与 final answer 均无未验证 `iota`；
- B99 patch citation drift 本轮因上游端点 authority 缺失，没有形成同类插行形，保持 replay-pending；
- 新 `EVAL-B100-VISIBLECARRIER1`：authored Markdown table 使 renderer 隐藏 `items[]`，coverage 却把 30 个 citation sidecar 当作可见常量行，最终只报数量、不列名称；
- 新 `EVAL-B100-ENDPOINTADMISSION1`：schema-required `call_chain_endpoints` 缺失仍被 Go 零值解码接受，后续所有有序端点合同正确 stand-down，Finalizer 在 prose 中反向猜 wrapper，图门四次修补仍不能纠正模型结论。

两项已交付：renderer/validator 共享 concrete block visibility，隐藏 sidecar 仅能给 Markdown 第一列已显示成员补引用；源码非 scalar call-chain 在 Analyzer 边界必须有 request-validated ordered profile，无序 entity/exact-target 永不提供方向。category/code-token/support-ref/multi-target 旁路同步收口；runtime artifact 与 scalar/role-locate 兼容车道隔离。

实现不扫描 request/model/final prose 作 hard gate，不系统代写成员或结论，不改变 RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。完整 `internal/tool` 170.868s 通过；状态均为 `implemented/full-tool-pass/replay-next`。
