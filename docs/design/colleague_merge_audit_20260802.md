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
