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

### §10.38 B101 production：B100 双项关闭；finalizer evidence handoff 权威断层已修

`main@bc4101f61` 严格并行两个 read case，inventory runner/human PASS，sequence runner/human FAIL；详细工件与裁定见 eval campaign §105。

- B100 visible-carrier 已关闭：authored Markdown 表中缺失的成员不能再由隐藏 sidecar 冒充可见，模型一次 repair 后完整显示 3/5/30 roster；
- B100 endpoint admission 已关闭：Analyzer 首次接受 emission 已携 ordered request-validated endpoints；
- 新 P0 `EVAL-B101-BOUNDARYHANDOFF1`：completion 读取 Explorer accepted evidence 后允许 typed no-path，Finalizer semantic boundary 却只读 mutable
  临时 buffer；交接/compaction 后 exact sink definition 消失，导致同轮 `endpoint proven` 与 `endpoint_unresolved` 自相矛盾，模型把 wrapper 方向写反。

已统一 AgentContext/BusContext 的 endpoint evidence pool：handoff `EvidenceItems` 与 mutable evidence lanes 共同进入同一 shared analyzer，保留 existing
grounding、current-source、identity/edge 去重及 definition/call-edge 权限分离。该修复只恢复 typed 上下文连续性，不扫描答案、不作 prose hard gate、不代写模型结论，
也不进入 RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。

完整 types/agent/tool 通过（22.439s/4.072s/169.392s），AgentContext、BusContext 与 Finalizer prompt 三面 pin 已落地。状态：
`EVAL-B101-BOUNDARYHANDOFF1=implemented/full-pass/replay-next`；B99 patch citation drift 仍因缺少同形生产 witness 保持 replay-pending。

### §10.39 B102 production：handoff 权威关闭；repair 目标与 graph 状态词面分轴

`main@30967b332` 严格并行两个 read case 均 runner/human FAIL；详细工件、人工审计与模型波动裁定见 eval campaign §106。

- B101 endpoint handoff 已关闭：Finalizer 收到 `definition_only`，不再把已证 endpoint 回退成 `unproven`；
- sequence 剩余错误是模型在同一答案开头写对 `Run -> RunWith`、末尾写反，diagram gate 删除无证边但不改写正文；不据单次波动新增 prose hard gate；
- inventory 的 38 条 production row/citation authority 正确，但两次 patch 把被拒绝 Markdown 表与新 ordered list 累积成重复 roster；模型另有一次未复现的 `iota` 幻觉和分类计数漏显。

已交付两个 P1 通用修复：`EVAL-B102-GRAPHSTATUS1` 将 prompt 的 `evidence_status` 收窄为 `call_graph_status`，明确它只描述 call-edge graph，
与 endpoint existence proof 分轴；`EVAL-B102-PATCHTARGET1` 在 authored Markdown 表已可见地承载 obligation identity 时，使用同一结构判定点名既有 block ID，要求
`replace_blocks` 原位补 citation sidecar，禁止 `add_blocks` 复制 roster。

两项都只改善模型上下文/repair 指引，不修改模型答案、事实或结论，不扫描 request/final prose 作 hard gate，不触碰 RootCauseTrace、显式时间窗、双轴根因、
因果投影或自动补齐。完整 agent/tool 通过（2.480s/161.239s）。状态均为 `implemented/full-pass/replay-next`；`iota`、per-role count 与 B99 citation drift
按 §106 留项继续异构回放。

---

## §11 MERGE-AUDIT-6 增量审计(2026-08-09):466 笔(08-04→08-09)= §10 修复响应 + evaluator/图表/data-task/trace 权威四大波

**范围**:b4d38cdb3..HEAD 非本席合入 466 笔。**方法**:8 主题读者+逐条 2 席否证(22 agent,wf_052e4d58-a52,多处 go test overlay 可执行判别)+全仓基线实测;只审计不修码。产出:**3 确认(1 高)+6 低+4 项指控被否证席驳回;39 项核验通过**——本审计系列开账以来同事成色最高的一轮。

### §11.1 基线与修复合规总评(正面)

- **main 绿**(本席实测);红线结构 pin 全绿。
- **§10 全部高危验证正确关闭**:S6-1 内部架构文本整段退役(repo-wide grep 零残留);M5-S3-1 定界符门新增 `skipRustLifetimeOrLabel` 按语言族路由(.rs 撇号);M5-S3-2 过针冻结改绑 post-apply 探测报告(附 8ffa88f71 自动盖章/5749e5b30 套件抢占两件加固);M5-S1-1 锚门与 ExpectedShape 教学共享单一别名解析权威;M5-S5-1 方向改走 typed `CallChainEndpointProfile{source,sink}`(实体序退役);S2 族逐语言关闭。
- **S4-3 §10.6 并置臂施工整体合规**:评判臂维持退役、嘈声扫描纯选择器、[E#] 强制、中性框架词、所有权 tripwire 未动——唯 pin ③ 缺口见 §11.2(M6-U1-1)。
- **我方全部裁定面完好**(U5 专项 sweep):crown 前缀+限定注/CROWNCAL 臂/TWODIM 未计价占用账/closed-matrix 教学/causal_token_registry 零动;73783a7a8 on-chain 收窄与 ISPGAP-1 既裁一致;69829402d 离链推断收束保留了披露出口(维度一出口未伤)。
- **同事账本抽检 6 条声称全实**(连续第三轮零虚账);当日 revert 对(8091d497e→f5d30b00b)取证=analyzer 铸图 participant 硬门自知误伤即回滚,END-STATE 零残留;47 笔 eval 提交仅 1 条降杆(见 U7-1 低危,有账可查)。

### §11.2 确认(3 件)

| # | 位置 | 问题 | 复现 |
|---|---|---|---|
| **M6-U3-1(高)** | `answer_document_diagram_evidence.go:377` | **T3-2 类第四例**:8304122c0 把 sequence 箭头扩到非调用关系族(register/type_relation 等)并写进教学,但这两个证据门要求**双端全限定段序相等**(`AnswerCodeIdentitySurfacesEquivalent`),没带调用车道同批刚修好的 short→qualified 唯一投影桥(2d5810692 的 B423 根修)——教学祝福的短名 participant 展示形在非调用臂上硬拒;B423 账本已证产线模型就用短形(7 连拒 513s 级联的前科姿态) | 静态(与 B423 前科逐点对照) |
| M6-U3-2(中) | 同文件:469 | callback/value/guard 族匹配器反向病:`CallChainEndpointCompatible` 双向接受短尾冒充限定名**且无唯一性要求**——与 M6-U3-1 一收一放,同批同臂不同纪律(第三次出现"同批不同车道不同纪律"形) | 静态 |
| M6-U1-1(中) | `orchestrator/prose_typed_reconciliation.go:69` | **S4-3 裁定 pin ③ 缺失且交付声称不实**:唯一 e2e 用的是"正确算式且值恰好 3 位全匹配"的 fixture——算式形状选择器臂在任何测试中都不是判决信号(否证席 overlay 双向实证:删臂全绿/错算式行为确实只靠该臂发射);§10.11 声称的「错误算式 e2e」不存在。行为今日完好,缺的是裁定命令的红绿 pin+交付账实 | 否证席 **overlay 双向可执行复现** |

### §11.3 低危顾问(6 件)

U2-3 evaluator 阶段车道权威 pin 绑 stage_binding.go 压缩源码子串(重构静默失配);U2-4 perf 帧候选改信模型 Janky 位(确定性长帧地板被移,回退了当初设防的理由);U5-1 thread_role 逃逸车道退役但教学仍承诺该凭证(不可铸词面);U5-2 有任何确定性 trace 观测即丢弃**全部** analyzer 阶段报告 prose(粗粒度抑制,含与 trace 无关的分析上下文);U6-1 新 member_notes/support_refs 拒绝车道逐行首错报告(EMITBURN 分歧);U7-1 唯一降杆 eval(contributions=4→任意正数,有文档记录,属既裁 jitter 类,建议回收紧)。

### §11.4 处置建议(未动码)

1. **M6-U3-1/U3-2 同批修**(高优先):非调用关系族证据门接入与调用车道同一的 side-aware exact-or-unique-short 投影助手(单源),callback/value 族补唯一性——第四例后建议把「新关系臂必须复用 `diagramCallEndpointHasExactOrUniqueShortProjection` 单点」写成结构 tripwire(census:凡 `diagram*EdgeHasTypedEvidence` 家族函数必须调用该助手或登记豁免理由)。
2. **M6-U1-1**:补裁定命令的错算式红绿 e2e(fixture=算式内洽但操作数无 3 位匹配,如 82.1+9.1=91.2),并更正 §10.11 交付记录。
3. 低危随批;U2-4 建议恢复确定性地板为 AND 臂;U7-1 回收紧。

**方法学**:①同事修复合规连续第三轮 100%(39/39 核验通过),新缺陷数从 12→12→3 递减且首次零新域高危(唯一高危是既裁类第四例);②T3-2 类四连发定谳其为**结构性复发类**——单点助手+census tripwire(§11.4-1)是终局解,单靠逐例修复不收敛;③否证席 overlay 双向实证(删臂绿+行为依赖臂)首次同时证明"pin 缺失"与"行为完好",是 test-gap 类 finding 的黄金判别形。

### §11.5 处置进展（2026-08-09）

代码复核确认 §11 三项判定均准确，按两批收口：

1. **M6-U3-1/U3-2 批 1 已施工**：call、callback、type relation、registration、assignment/return 与 guard/logical
   全部复用 `diagramRelationEdgeHasExactOrUniqueShortProjection` 单点。证据行继续拥有关系类型与方向；qualified diagram endpoint
   必须由 exact typed qualified identity 支撑，short endpoint 只允许投影一个 typed identity family；同尾不同 owner 失败关闭；source/target
   必须在同一 evidence row 同向成立，禁止两行拼边；
2. 删除 callback/value/guard 原来的双向 `CallChainEndpointCompatible` 宽匹配，也删除 type/register 的 exact-only 孤岛。修复不读取
   request、Mermaid 文案、模型 thinking/final prose、语言名或路径，不放松缺少 typed relation 的硬门；
3. 功能 census pin 覆盖五个非 call 严格关系族：每族验证 unique qualified→short 通过、same-tail 多 owner 拒绝、short evidence→
   model-qualified owner 拒绝；既有 call lane 的 mixed/ambiguity/direction 测试继续共用同一 helper；
4. **M6-U1-1 留给批 2**：补“算式内洽但操作数不匹配 typed 三位值”的 e2e，并纠正 §10.11 交付账，不改现有中性并置行为。

状态：`M6-U3-1=batch1-implemented/full-suite-pass`；
`M6-U3-2=batch1-implemented/full-suite-pass`；`M6-U1-1=batch2-next`；
`raw-prose-new-hard-gate=none`；`model-answer-rewrite=none`；Trace/read/write/data=`unmodified`。

#### 批 2：M6-U1-1 错算式选择器独立 e2e 收账

§11 的否证结论准确：§10.11 虽声称已有“错误算式 e2e”，原测试的
`20.000+30.000+64.940=114.940ms` 每个操作数都命中 typed 三位值，删掉 equation-shape 臂仍会由
exact-value 臂选中，因而不能证明裁定的错算式车道存在。

本批新增 production-root e2e：模型块只含内部自洽但与 typed 账完全不同的
`82.1 + 9.1 = 91.2ms`。fixture 先断言不含账户的任何三位值，再经真实 Trace 投影物化与
`collectSystemCrossCheckFindings` 发布路径，要求 appendix 出现带 `[E#]` 的 typed 全窗状态账户；同时断言
模型/系统块字节不变，发射行不复制 `82.1/9.1/91.2`，也不出现“模型/错误/不符”等评判词。这样 equation
只决定“是否并置”，subject、数值、关系与 locator 仍全部来自已发布 typed projection。

对 §10.11 末尾“错误算式 e2e 已交付”的历史表述作此追加更正：它在本批之前是误记，本批测试才构成该项
可删除性证明。未新增 violation/retry/hard gate，也未修改模型正文。

验证：定向 `go test ./internal/orchestrator -run 'TestTypedReconciliation' -count=1` 与
`go test ./... -count=1` 全绿；其中 `internal/orchestrator` 20.794s、`internal/tool` 196.133s、
`internal/tracequery` 88.706s。

状态：`M6-U1-1=closed/full-suite-pass`；`MERGE-AUDIT-6=closed`。

### §11.6 审计后 production eval 补充（r258）

§11 三项已按 §11.5 两批关闭；随后严格并行 production eval 又发现两个独立通用问题，避免把“审计项关闭”误记成全域无债：

1. `EVAL-B445-DATALEDGERGENERATION1=P0`：source-backed rules 晚于 contribution/reconcile/answer 生成时，旧实现只看各 ledger
   count/present，错误发布 complete/无可用动作；终验则按真实 `rule_refs` 依赖拒绝，形成 state/validator split-brain。当前批用共享 typed
   linkage judgment、replacement generation 和 deterministic typed recompute 修复；`go test ./... -count=1` 全绿，待 production 回放收账；
2. `EVAL-B446-DIAGRAMMETADATAESCAPE1=P1`：QF type-relation 首稿方向错误时，anchor 门正确拒绝，但 patch 删除
   `edge_anchors` 后可保留原可见关系边出厂。它不否定 M6-U3 的 endpoint identity 修复；暴露的是其更上层的 visible edge owner
   完备性 GAP，应以全部 source relation 族共用的 typed carrier coverage 修复，不能按某语言、implements 标签或当前模型文案特判；
3. 两项均不授权系统改写模型结论。B445 只修 typed workflow state/repair；B446 只约束结构化 diagram payload 与 typed evidence。
   Runtime Trace 的显式时间窗、系统补采、链上根因、双轴占时、可消除量及因果投影继续使用独立 authority。

状态：`MERGE-AUDIT-6=closed`；`B445=implemented/full-suite-pass/pending-production-replay`；`B446=next-batch`；
`raw-request/model-prose-hard-gate=none`；`model-conclusion-ownership=preserved`。

#### B446 批次进展：visible-edge owner 按全部 typed relation axis 收口

已新增 `PredicateAxisRequiresDiagramEdgeOwnership` 单点，call/register/return/configure/condition/implement/flow 七个 source relation
axis 共用；`AxisDefine` 保留 presentation-only 出口，`QFRootCauseTrace` 继续走独立 runtime authority。现在删除 `edge_anchors` 而保留同一
关系边会稳定触发 `missing_relation_anchor`，不能再从严格关系车道降成无元数据展示图；有 owner 的边仍必须通过各自的 same-row、same-direction、
exact-or-unique-short typed evidence 门。

新增七轴 census 和 r258 implementer 三臂 pin；`go test ./... -count=1` 全绿。实现不扫描 request/diagram label/model prose，
不由系统改写模型图或结论。

状态：`B446=implemented/full-suite-pass/pending-production-replay`。

### §11.7 r259 独立回放与后续批次

§11 原三项已经关闭；r259 exact-two 又暴露两个不属于原审计项的新断层：

1. `B448/P0`：data material guard 正确拒绝漏读 instructions 的脚本，但 normal+compact repair 连续输出 schema-invalid
   `emit_data_task_plan` 参数后直接终止。已实现 exact typed param-error→workflow candidate fallback；候选仍走全部执行/校验面，系统不代答；
2. `B447/P1`：B446 已阻止删除 relation metadata 逃逸，但 relation roster 的 exact graph authority 没有驱动缺失源码行的 pre-complete read，
   使 Finalizer 只能证明 5/12 条 implements 边。下一批补 typed relation authority handoff，不把未读 graph 数据直接当引用；
3. B445 本轮未被执行到，保持 production replay pending。详细过程与人工审计见 eval campaign §123.468–§123.469。

状态：`MERGE-AUDIT-6=closed`；`B448=implemented/repl-suite-pass/pending-full-suite`；
`B447=next-batch`；`raw-prose-hard-gate=none`；`model-answer-rewrite=none`。

#### B447 批次进展：exact roster 驱动源码行补读，不把 graph 直接当 citation

已增加显式 typed implementer diagram authority provider。required implement diagram 中，只有 principal member_set 实际纳入的 exact graph
candidate 才会产生精确声明行补读；同文件 direct definition 不等于 implementation edge。读后沿用 Explorer 的跨语言 RepoMap relation
producer 铸 `repomap_implementer_relation`，Finalizer 继续只信可引用 typed edge。

该实现不含语言/扩展名/类型名字面量分支，复用所有 supported-language graph carrier；无 required diagram、AxisDefine、excluded member、
辅助 scope、缺失文件/行号均不触发。定向与全 `internal/tool` 通过。

全仓 `go test ./... -count=1` 通过。

状态：`B447=implemented/full-suite-pass/pending-production-replay`；
`B448=implemented/full-suite-pass/pending-production-replay`；`model-answer-rewrite=none`。

#### r260 复核更正：B447 第一段有效，第二段证据导出未闭环

r260 production replay 证明 B447 的 exact declaration read demand 能把 12 个 principal implementer 全部读齐，但原“read-but-unemitted”
分支要求模型发射一个只能在 Explorer completion 后由 deterministic RepoMap producer 生成的证据行，构成时序上不可满足的循环。四次
pre-complete downgrade 后，12 条 typed implementer relation 虽已出现在 Finalizer prompt note，却因成文校验只读 bounded Turn-A 与
model-emitted evidence、未读 lossless `BusContext.EvidenceItems`，导致五次正确关系图均被拒，最后以零边图假绿。

追加批次按 shared evidence boundary 根修：exact declaration line 已读即由 pre-complete 交棒给 post-loop producer，不再要求模型冒充
producer；成文校验索引纳入 StageOutput/BusContext typed evidence，并按 StableEvidenceID 去重运输副本。定向 `internal/tool`（175.574s）和
`internal/agent`（10.265s）通过。该变更不代写模型答案/图，不扫描原文作硬门，不改 Trace/read/write/data 语义。

状态：`B447=production-partial/second-stage-fixed`；`EVAL-B449-TYPERELATIONEVIDENCEEXPORT1=implemented/pending-production-replay`；
`B445/B448=pending-production-replay`；`model-answer-rewrite=none`。

#### r261 production 收账：B447/B449 关闭

同一 QF implementer case 从 r260 的 488s、4 次 pre-complete 不可满足降级、5 次 Finalizer 拒绝、最终 0 边，收敛为 158s、
0 次 pre-complete relation materialization 降级、1 次正确方向拒绝、最终 12 条完整
`implementer -> LoopController` typed edges。唯一 patch 由模型自行把首稿反向 body/anchors 同步翻转，系统只提供 typed direction failure，
没有代画、补边或替换结论。

表格 12 个 production implementer/文件与 3 个 test caveat 均正确；人工与 runner 均通过。由此关闭 B447/B449 production。
并行 data 仍走 planner-distilled 普通成功路径，没有触发 B445/B448，二者继续 pending。

状态：`B447=production-closed`；`EVAL-B449-TYPERELATIONEVIDENCEEXPORT1=production-closed`；
`B445/B448=pending-production-replay`；`MERGE-AUDIT-6=closed`；`model-answer-rewrite=none`。

### §11.8 r262 后续：普通 continuation fallback 与 scaffold 预算断层

严格并行 data+Trace 回放发现审计项关闭后的独立 P0 `B450`：data typed state 正确要求补 contributions，但普通
continuation plan 两次 schema-invalid 后，fallback 一方面未携带 repoRoot，另一方面只收到 first-family-wins 截断后的前 10 条
scaffold；合法的贡献 family 被早期 action 变体饿死，最终以 planner 参数错误终止。

已在共享边界修复：continuation/resume fallback 统一 repo-aware；不增加 10 条 transport 预算，按 action family 公平保留并优先
可执行 carrier，使状态的 allowed actions 与候选构造可达性一致。r262 checkpoint 原样复算已由无候选变为非 relation typed
`value_distribution` 候选，后续仍需模型/validator/evaluator完成业务筛选与贡献计算，系统不代答。定向与全仓测试全绿，待 production replay。

并行 H7 的 runner FAIL 经人工判为旧 oracle，不是 Trace 生产退化：当前链上根因、显式窗、系统补齐、双维根因、因果投影均完整；
旧 `0.018/49.638/0.105` 展示钉与现行 typed `0.033/49.623 + incomplete enumeration` 不一致，独立更新 eval case，禁止倒改生产值。

状态：`B450=implemented/full-suite-pass/pending-production-replay`；`H7-oracle=stale`；
`B445/B448=pending-production-replay`；`model-answer-rewrite=none`。

H7 oracle 已在独立卫生批更新：硬值改为现行 typed `0.033/49.623`，压缩面改钉非零未入榜行与
`enumeration_status=incomplete`，并保留 `未计价占用` 双轴出口；r262 既有 production artifact 全部命中。
状态：`H7-oracle=updated/r262-artifact-pass`。

### §11.9 六项低危顾问复核与批次裁定

在 §11 三项确认件和 r262 收口后，逐项对照当前生产代码复核 §11.3 的六个顾问项。结论不是“六项全部照单施工”：

| 项目 | 复核结论 | 处置 |
|---|---|---|
| U2-3 stage-lane checkout pin | **确认**。`answerDocumentCheckoutMatchesReadModeStageLaneAuthority` 读取 checkout 后把整个函数压缩成字符串，并依赖一种源码拼写；注释/换行可让语义不变的实现静默失配 | 批 1 改为 Go AST 结构验证，只接受 `ReadModeConditionalPreStageBindings` 函数内有序的 `[]PipelineStage{StageLogTriage, StagePerfTriage}`；多项、错序、语法错误仍 fail-closed |
| U2-4 perf 长帧地板 | **指控成立于“实现发生过变化”，但恢复建议不成立**。旧 `16.67ms@60Hz` 是无设备 refresh/deadline 权威的固定假设；当前 `Janky` 明确只作 model-extracted navigation candidate，真实 duration 仍在 `PerfBundle.Frames`、artifact profile 与 claim binding 中完整保留 | 不恢复固定 verdict 地板，也不把 duration 变成硬门。保持“时长可报告、是否丢帧未证”；只有 typed device deadline/frame authority 才能定谳 |
| U5-1 `thread_role` 教学 | **部分准确、非行为 GAP**。span/name producer 已退役 `thread_role`，但类型仍为独立 producer-owned carrier 预留；当前教学明确写了 carrier 缺失时只能说 candidate，并未要求模型铸造该字段 | 批 1 仅补一句“当前 span-name rows 不提供该 carrier”，降低模型心智；保留未来独立 typed producer 兼容，不改 gate |
| U5-2 Analyzer prose 抑制 | **实现属实，GAP 指控不成立**。抑制仅在 Finalizer 且已有 deterministic trace_query 时按 stage provenance 生效；Analyzer 的 typed `AnalysisIR`、请求维度和 runtime ledger 仍独立到达。保留自由 prose 会让查询前猜测与查询后事实竞争；按 prose 主题拆分反而需要噪声扫描 | 保持现状；不解析 Analyzer prose 猜“相关/无关”，不恢复第三语义通道 |
| U6-1 member row 首错 | **确认**。normalize 层已有 EMITBURN 全错汇总，但后置 `member_notes/support_refs` evidence-resolution gate 仍在第一个 fact/row 返回，多个错位行会逐轮烧 emit | 批 2 增加同一 typed gate 内的全 payload 违规汇总；首错语义和拒绝 verdict 不变，消息一次列出所有需修行 |
| U7-1 contributions 4→正数 | **降杆指控不成立**。production 已证明先 target join 后贡献可产生 3 条，先全量贡献后 projection 可产生 4 条；两者最终 `17,0,5`、完整 ledger、reconcile 都正确。固定 4 是内部 DAG 拟合 | 保留“正整数贡献 + 精确最终值 + reconcile=pass + terminal artifact”合同，不回退假红 oracle |

批 1 已完成 U2-3，并同步澄清 U5-1：新增格式/注释重构正臂，并保留额外 conditional stage 的负臂；线程角色软教学明确当前 span/name-derived rows 不提供 `thread_role`，但保留独立 typed producer 的兼容入口。定向 `internal/agent` 通过。该实现只解析 checkout Go AST 和调整软教学，不读取用户请求、模型思考、答案 prose，不改答案或 Trace 权威。

状态：`U2-3/U5-1=batch1-implemented/agent-pass`；`U6-1=batch2-next`；
`U2-4/U5-2/U7-1=reviewed-no-production-change`；
`raw-prose-hard-gate=none`；`model-answer-rewrite=none`；Trace explicit-window/causal projection=`unchanged`。

#### 批 2：U6-1 member row 拒绝一次报告完整修复集

`validateAggregateMemberSetSupportRefs` 保留原首错文字和同一拒绝 verdict，但不再在第一个 fact/row 立即返回。它现在在同一个 typed
payload 内收集所有 `member_notes/support_refs` 对齐、空值、same-member evidence resolution 和 decorated-member grounding 违规，去重后以
`[1]..[N]` 一次发给 Explorer；同一 narrative/per-member 合同重叠时只保留更精确的一条，避免把同一行换词重复报错。

新增三错 witness：第 0 行空 note + 两行 ref 均不解析时，单次拒绝同时给出 3 个修点；既有单错字符串、repair origin、拒绝/降级语义和
EMITBURN normalize 汇总均保持。系统不自动生成 note/ref，不删除 member，也不接受未落地证据。

状态：`U6-1=batch2-implemented/targeted-tool-pass`；`member-row-verdict=unchanged`；
`model-authored-facts=preserved`；`raw-prose-hard-gate=none`。

### §11.10 r263 production 复核：§11 顾问批关闭，B450 后继控制权 GAP 新立案

§11.3 六项已经按 §11.9 的裁定完成施工/否证；定向 agent/tool 与全相关包测试通过。随后 exact-two production 回放没有发现
§11 修复回归，但证明 B450 只修复了 scaffold transport 可见性，没有关闭执行控制权：首个缺失账本明确为 contributions、唯一
producer 明确为 `compute_contributions` 时，post-result deterministic fallback 仍可连续执行不生产该账本的 auxiliary actions，
并靠新 artifact alias 逃过 action-key 去重直至耗尽 18 轮。

新立 `EVAL-B451-DATAFIRSTPRODUCER1/P0`：自动快车道必须由 exact `ledger_graph.first_missing + produces_actions` 授权；不匹配的
可执行 scaffold 只能作为模型的 typed 候选，不能由系统自行接管。QF 并行件另立两个 P1：最终 Mermaid 删除用户要求的
BusContext/Mutable 数据流仍被 runner 签绿，以及 Analyzer 的 model/deterministic provenance 上下文混写。详细证据见 eval campaign
§123.475 与 r263 manual audit。

状态：`MERGE-AUDIT-6/§11=closed`；`B451=P0-next-batch`；`B452/B453=P1-queued`；
`B450=production-replay-failed`；`model-answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.1 B451 施工结果

post-result 自动权现已绑定 exact first-incomplete ledger 的 `produces_actions`：全部 action 均为该 producer 才可自动派发；
auxiliary-only 或 producer/auxiliary 混批回到 evaluator/continuation planner。该修复不把通用 compute scaffold 强制变成可执行，
因此系统不会猜 value/group/filter，也不删除模型可见的 typed 候选。dataworkflow/repl 全包通过，等待同 case production 回放。

状态：`B451=implemented/pending-production-replay`；`B452/B453=queued`；`§11=closed`。

#### §11.10.2 r264 收账：§11 保持关闭，B451 production 关闭并转出独立 repair-carrier GAP

r264 exact-two 没有发现 §11 回归。B451 的 producer-bound 派发权在 production 中生效：同一 data case 不再进入 18 轮 auxiliary
自循环，evaluator/planner 能推进到 contribution producer。随后失败来自独立的 typed error transport 缺口：精确
field/value/locator 被拼为一个展示字符串，模型误把 locator 纳入 filter value，并同时受到 filter alias 与 canonical inner schema 不一致影响。
该项以 `EVAL-B454-DATAREPAIRFACTCARRIER1/P0` 转入 eval campaign，不能回归记到 §11 或 B451。

并行 QF 继续确认 B452/B453：runner 的任意边计数无法识别用户要求的数据流被删，以及 Analyzer model/deterministic provenance 混写。
后续按 B454→B452/B453 排序施工；§11 审计结论及已关闭状态不重开。

状态：`MERGE-AUDIT-6/§11=closed`；`B451=production-closed`；`B454=P0-next`；`B452/B453=P1-queued`；
`model-answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.3 B454 实施结果

独立 repair-carrier GAP 已按 typed 全链修复：generated status 的 field/value/source locator 不再只存在于拼接错误文本，
`repair_params` 直接给出 parser-compatible `qualify_records` 参数片段；scaffold 统一 canonical JSON filter keys。所有新增复合字段均在
workflow snapshot/journal/guard 边界深拷贝，避免 live state 被后续修改污染。系统只提供精确事实与结构化修复输入，不自动过滤记录、
不决定业务归属、不写最终答案。定向三包及全仓测试全绿，等待同 case production replay。

状态：`MERGE-AUDIT-6/§11=closed`；`B454=implemented/pending-production-replay`；`B452/B453=queued`；
`model-answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.4 r265：§11 继续关闭；独立 deferred-prefix 终态误判立案

r265 exact-two 没有重开 §11 的任何已关闭项。data 得到正确 `17,0,5` 和完整 rule/decision/contribution/reconcile/final projection，
但 B454 的 generated-status carrier 本轮未触发，继续等待目标分支 witness。

新发现 `EVAL-B455-DATADEFERREDPREFIXTERMINAL1/P1` 属于 §11 之外的 workflow 控制流：typed dependency split 已保存 deferred
suffix，执行 prefix 却保留 terminal `ContinueAfter=false`，于是中间 join/filter 成功结果被完整终态 ledger validator 判为 contributions
缺失并送修。该 gap 解释 12 batch、5 repair、502s 的主要 churn；后续只修 typed intermediate/terminal authority，不放宽最终 ledger、
reconcile 或 answer gate。

并行 QF 再次确认 B452/B453，runner 任意边 oracle 仍无法识别主数据流被删和 Analyzer provenance 混写。处置顺序维持
B452/B453 后 B455；§11 审计本身保持关闭。

状态：`MERGE-AUDIT-6/§11=closed`；`B454=pending-targeted-production-witness`；
`B452/B453=next-batch`；`B455=queued`；`model-answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.5 B452 独立批完成：辅助 flow edge 不再提前关闭 required participant 调查

B452 已按 typed soft-repair lane 落地：required source-flow diagram 中，已有任意 operation 不再自动等同于已覆盖所有
`incident_required` 参与者。未覆盖名单来自 schema participant roles 与 citable operation endpoints，不读 request/Mermaid/model prose；
Explorer 获得一次聚焦补证机会，再次无进展以 typed boundary 收敛。系统不铸边、不代画、不改写结论，Finalizer 关系门保持原样。

`context_only`、Trace/RootCauseTrace 与外部 runtime-only flow 均有负 pin；types/tool 全包通过。§11 继续关闭，B452 等待 production
回放；下一独立批处理 B453 provenance/系统 supplement 权属。

状态：`MERGE-AUDIT-6/§11=closed`；`B452=implemented/pending-production-replay`；`B453=next`；`B455=queued`；
`model-answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.6 B453 独立批完成：阶段 provenance 前移，退役系统成文补表

B453 冷读确认两处同根问题：`StageAnalyze` 的共享权威描述把模型分类与 post-emit 确定性编译合写成一个主体动作；Finalizer 虽已有
read-mode 主阶段顺序上下文，最后一公里仍会在模型答案后追加“系统补充：阶段绑定核对”。后者会让缺失的模型推理在 runner 上表面变完整，
也越过“系统提供精确事实、模型负责成文”的权属边界。

本批把共享 `StageAnalyze` responsibility 改成显式 producer split：模型只提交请求分类，之后由确定性代码规范化并编译分析合同、任务、
证据、假设、质量与答案合同。Finalizer prompt 的 current-run stage authority 现在从 checkout 校验后的 typed stage-binding rows 动态携带
四阶段 agent、skill、responsibility、primary artifacts 与 source line；源码中不手抄内部类型教学，也不把阶段顺序冒充函数调用边。

最后一公里阶段绑定 supplement 及其只服务于代写面的 gating/dedup helper 已删除；无论 citation 是否完整、是否请求 workflow 维度，系统均
不再给最终答案追加该表。模型原 AnswerDocument 字节面保留，其它 runtime/read-audit/degradation supplements 未改。新增/更新 pin 覆盖
workflow 请求、grounded membership、read/write lane 隔离、lookalike checkout fail-closed、格式重构兼容、provenance 内容与中英文
supplement 负断言；`go test ./internal/types ./internal/agent -count=1` 全绿。

全仓复验 `go test ./... -count=1` 亦全绿，包含 orchestrator、tool、tracequery、tracediag、hitraceconv、dataworkflow 与 writeflow。

状态：`MERGE-AUDIT-6/§11=closed`；`B452=implemented/pending-production-replay`；
`B453=implemented/full-suite-pass/pending-production-replay`；`B455=next`；
`system-stage-answer-supplement=retired`；`model-answer-authority=preserved`；`raw-prose-hard-gate=none`；
Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.7 B455 独立批完成：deferred prefix 与原计划终态分权

B455 从 r265 的 initial/intra-batch witness 扩展审计到全部 deferred split，确认四个同根传播点：initial rank 与 intra-batch prefix
沿用原 terminal flag；stage prefix 把 remainder 也无条件标为继续；deferred queue 二次按 rank 拆分时把“当前尚有 rest”写进 rest 自身；
多个 admission remainder 合并时又无条件改成继续。这既会让中间结果误入完整终验，也可能让最终 suffix 永久失去终态。

现统一不变量：非空 suffix 使本次 executable prefix 必为 intermediate；remainder 始终继承原计划 terminal intent；deferred 再拆时只有
本次 dispatched rank 根据剩余动作标 intermediate；多段 remainder 的最后一段拥有最终 terminal intent。CLI/REPL 的结果终验入口同时
强制接收当前 typed deferred plan：即使恢复了旧快照且 prefix flag 陈腐，只要队列非空也不运行完整 ledger/reconcile/answer gate；队列排空
且最终 rank 为 terminal 后，原 gate 字节路径恢复。

该批不修改 contribution、reconcile、reference projection、业务值或最终答案，不把缺失 ledger 视为成功。新增 initial/intra/stage 三类
split flag、deferred 再拆、remainder merge、legacy snapshot defense-in-depth pin；`go test ./internal/dataworkflow ./internal/repl -count=1`
全绿。

全仓复验 `go test ./... -count=1` 全绿。

状态：`MERGE-AUDIT-6/§11=closed`；`B455=implemented/full-suite-pass/pending-production-replay`；
`B452/B453=pending-production-replay`；`terminal-ledger-gates=unchanged`；`model-business-decision=none`；
`raw-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.8 r266 production 收账：B455 关闭，B452/B453 转入两个独立到达性 GAP

r266 exact-two 没有重开 §11 已关闭项。data 的 terminal answer、contribution/reconcile/reference projection 全绿，并且 r265 的
prefix 完整终验错误在日志中归零，B455 production 关闭。普通计划仍有 11 batch/4 repair/5 failed action，属于模型计划与 action schema
效率债，不回退 deferred terminal 修复。

QF 证明 B452 的 participant coverage 还不能关闭：Explorer 已读到真实 `dispatchStage -> BuildAgentContext` 调用并尝试发射，但
relationship item 因缺 `line_start` 被 `emit_evidence` 跳过；该工具没有返回 typed repair，反而报告无 actionable target。随后
flow-operation no-progress 收敛使 participant lane 的前置条件始终不成立。该跨工具修复账本断层另立
`EVAL-B456-EVIDENCESKIPREPAIR1/P0`，优先于继续扩 relation 教学。

B453 的系统阶段补表已确认不再出现，但 current-run stage authority 仍因触发条件过窄未进入本次 Finalizer prompt，producer provenance
继续含混。另立 `EVAL-B457-STAGEAUTHORITYREACH1/P1`，只允许 checkout-verified Codrax stage binding 与 typed flow/diagram signal
合取触发 prompt authority；不扫原文，不做客户仓污染，不代写答案。

状态：`MERGE-AUDIT-6/§11=closed`；`B455=production-closed`；
`B452=production-replay-failed/blocked-by-B456`；`B453=partial/blocked-by-B457`；
`B456=P0-next`；`B457=P1-after-B456`；`model-answer-rewrite=none`；`raw-prose-hard-gate=none`；
Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.9 B456 独立批完成：skipped evidence item 不再从 repair ledger 消失

对 r266 witness 冷读确认：`emit_evidence` 的合法 sibling 已正常入库，唯一断层是 schema-invalid relationship item 在 per-item
validation 后消失，summary 又宣称无修复目标；因此本批没有放宽关系证据合同，而是在原拒绝点生成局部 typed repair。

新增 `evidence_item_validation` repair code，精确携带 `items[N].field`、拒绝数量、stage/scope 与 action-required 状态；字段路径来自
producer-owned validation branch，未知新分支退守 item path。required typed relation diagram 才附加 completion debt，optional/普通证据
不会阻断。completion 读取最新 `emit_evidence` repair，保留修复机会且不把它记为 no-progress；成功重发立即清账。系统不猜字段值、
不从 participant 铸边、不扫描 request/model prose、不补写最终答案。

新增 partial/all-invalid/optional/completion-clear 五类断言；tool/types 包与全仓测试全绿。B456 等待同 case production replay，B452 的
participant 补证需在该 replay 中一并复核；B457 仍是下一独立批。

状态：`MERGE-AUDIT-6/§11=closed`；`B456=implemented/full-suite-pass/pending-production-replay`；
`B452=pending-replay-after-B456`；`B457=next`；`model-answer-rewrite=none`；`raw-prose-hard-gate=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.10 B457 独立批完成：stage authority 以 typed participant slate 到达 prompt

B453 的 provider 内容与系统补表退役均正确，r266 未触发的原因是到达条件只认 requested-dimension / membership-definition 两种窄形。
现增加精确第三臂：`AxisFlow + required DiagramHint + 四个动态 read-main binding 全覆盖 + citable pipeline source`，并继续合取
checkout-side stage binding/conditional pre-stage AST 校验。binding 校验同时覆盖 responsibility、primary artifacts、terminal，不再只凭
stage/agent/skill 同名放行；身份匹配复用跨语言 code-identity surface 规则，不扫用户或模型原文。

该臂允许 BusContext/Mutable 等额外 participant，但不把它们铸成 stage/edge；缺阶段、context-only、非 flow、Trace、optional、
uncitable source 全部有负 pin；同名职责漂移拒绝与纯格式重构接受也有 pin。真实 `BuildInitialInstruction` 接线已有正 pin，避免 provider 存在但生产 prompt 不消费。系统仍不补写阶段表，
不修改答案，不放宽 relation gate。

全仓 `go test ./... -count=1` 全绿。

状态：`MERGE-AUDIT-6/§11=closed`；`B457=implemented/full-suite-pass/pending-production-replay`；
`B453=prompt-reach-code-complete`；`B456/B452=pending-production-replay`；`model-answer-rewrite=none`；
`raw-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.11 r267：§11 保持关闭；B456 收账，另立布尔域与 typed participant alias 两件

r267 未重开 MERGE-AUDIT-6/§11 的已关闭项。QF 的 skipped evidence 局部修复在真实流水线生效：合法 sibling 保留，被拒关系项按精确
字段重发，因此 B456 production 关闭。B452/B457 仍未闭环，但新 witness 将共同阻断点收窄为 schema participant 的展示身份：
`Analyzer agent`、`Mutable (in BusContext)` 等不是合法 code identity，现有 comparator 必然拒绝。另立 B459，只允许 typed participant
经 typed entities 做软 alias 解析；禁止把 alias 当源码边或成文关系 authority。

data 的 `20,0,5` 则是独立 `EVAL-B458-BOOLFILTERDOMAIN1/P0`：typed filter `active != inactive` 面对真实字段值 `false` 时，比较器未把
active/inactive 归为 bool-like 值，导致 inactive r3 进入候选集；后续账本忠实地对错误输入自洽。修复位置应在通用 filter value
normalization，不从材料 prose 推断业务规则，也不改 contribution/reconcile 的终态门。

本轮 QF “系统保留内容”属于已裁定的未校验模型输出保全，未进入结构化主体或 evidence authority，不按系统代写/strict gate 绕过立案。
runner 任意 Mermaid 边计数继续仅作机械 signal，人工判定仍为失败。

状态：`MERGE-AUDIT-6/§11=closed`；`B456=production-closed`；`B458=P0-next`；`B459=P1-queued`；
`B452/B457=partial`；`model-answer-rewrite=none`；`raw-request/model-answer-prose-hard-gate=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.12 B458 完成：标准布尔状态词不再裂域

通用 filter comparator 已把 active/inactive、enabled/disabled 纳入既有 bool-like 等价层；因此 `active != inactive` 面对 `false` 时
正确排除，而不会让错误候选集进入 contribution/reconcile。该修复覆盖 eq/ne/in/not_in 的共享比较入口，不读取 purpose/prose，
不代选业务 filter。四值域 + decision ledger 回归及 dataquery/全仓测试全绿。

状态：`MERGE-AUDIT-6/§11=closed`；`B458=implemented/full-suite-pass/pending-production-replay`；`B459=next`；
`terminal-ledger-gates=unchanged`；`model-business-decision=none`；`raw-prose-hard-gate=none`。

#### §11.10.13 B459 完成：展示 participant 只在软层解析为 typed entity

共享 typed helper 已覆盖 B457 prompt trigger 与 B452 Explorer checklist：展示标签只有在括注前主段唯一命中 AnalyzerHints.Entities 的
code identity 时才形成 planning alias；原生 identity 保持原样，歧义/未命中不解析。该载体不进入 evidence/grounding/edge validator，
因此不会把 analyzer planning 名字升级为源码关系。

装饰四阶段 participant 的真实 Finalizer prompt pin、装饰 Analyzer participant 的真实 operation coverage pin及歧义负臂均通过；三包和
全仓测试全绿。§11 继续关闭，B452/B457 等待同 case production 收账。

状态：`MERGE-AUDIT-6/§11=closed`；`B459=implemented/full-suite-pass/pending-production-replay`；
`B452/B457=code-complete/pending-replay`；`model-edge/answer-rewrite=none`；`raw-prose-hard-gate=none`。

#### §11.10.14 r268：§11 保持关闭；typed 图参与者声明与数据资格 lineage 另立两案

r268 未重开 MERGE-AUDIT-6/§11。B457 的 current-run stage authority 已真实进入 Finalizer prompt，producer split 正确，故生产关闭。
QF 仍缺主关系的直接原因变为 Analyzer 发射 required flow diagram 时省略整个 `participants` 字段；B452/B459 没有输入可消费。
用户点名的两份缺图结果并非完全同根：02:41 的旧结果属于 B459 前的展示 alias 阻断，04:10 的新结果属于 participant 字段可省略。
新立 B460，以 diagram_hint 内字段 presence 做 JSON 合同闭环，不扫描原文、不铸边、不放宽关系门。

data 中 B458 的 bool comparator 已生产生效，r3 被首个 filter 正确排除；错误终值来自派生支路把同一物理行重铸为新 item id，导致已有
exclude decision 无法约束 contribution。新立 B461，要求 typed record 变换保留精确 origin identity，使现有 decision↔contribution 门可复用；
不得针对 active/inactive 或本 case 字段做特判。

状态：`MERGE-AUDIT-6/§11=closed`；`B457=production-closed`；`B460=P1-next`；`B461=P0-independent`；
`B459/B452=pending-production-replay-after-B460`；`model-edge/answer-rewrite=none`；
`raw-request/model-answer-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.15 B460 独立批完成：required diagram 不再静默省略 participant 决定

`diagram_hint` 保持可选；对象在场时 `participants` 与 kind/required 同为 schema-required，未点名身份使用显式空数组。Analyzer 教学与 schema
共享同一合同，missing/empty/named 三臂均有 pin。系统不从 request/entities 推断列表，不把 participant 变成关系证据；B452/B459 仍须由
后续生产回放验证实际 operation 补证。全仓测试通过。

状态：`MERGE-AUDIT-6/§11=closed`；`B460=implemented/full-suite-pass/pending-production-replay`；
`B459/B452=pending-replay`；`B461=next`；`model-edge/answer-rewrite=none`；
`raw-request/model-answer-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.16 B461 独立批完成：decision/contribution 复用不可变 source-row identity

r268 的 eligibility 决策与 contribution 无法关联，不是既有一致性门缺失，而是同一物理行在 materialized 派生工件中被重新编号。
现将 `_source_path/_source_index/_source_line/_source_locator` 作为一对一 record action 的不可变 typed origin carrier；显式
`item_id_field` 保持最高优先级，缺精确 carrier 时仍 fail-open，禁止按字段值、说明文字或最终数值模糊连接。

filter/qualify 的 RowDecision 与 compute 的 ContributionRecord 现从同一 helper 取 item/source/locator；expand、apply-resolution、enrich
同步携带 origin。已有 cross-ledger gate 因此能直接拒绝“原始行已 exclude、另一派生支路却重新 contribution”的整类分支旁路。
反例与合法 filtered-sum 正例均已转绿；dataquery/dataworkflow 包套件通过。B460 遗留的旧 schema required-set pin 也已独立重钉为
kind/required/participants，不混入生产逻辑。

状态：`MERGE-AUDIT-6/§11=closed`；`B461=implemented/full-suite-pass/pending-production-replay`；
`B460=schema-contract-and-pin-closed/pending-replay`；`fuzzy-row-identity=forbidden`；`model-business-decision=none`；
`raw-request/model-answer-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.17 r269 收账：B460/B461 生产关闭，关系丢失收敛为证据铸造冲突

r269 exact-two runner 为 2/2，人工为 1/2。data 的 `17,0,5` 与 terminal contribution/reconcile/reference projection 一致，证明 B461
稳定 source-row identity 已堵住派生支路换号旁路；`B461=production-closed`。QF 的 Analyzer 明确发射六个 incident participant，证明
B460 schema presence 与教学在生产生效；`B460=production-closed`。

QF 仍缺关系的确定性根因是新 `EVAL-B462-TYPEDASSIGNMENTSHAPE1/P0`：纯字段声明
`mutable *types.MutableState` 被 Tier 1 token 命中误铸为 assignment，Finalizer 依 typed recipe 画出的同一边又被 answer validator 拒绝，
最终删边才可签绿。应在 grounder 源头要求 assignment/initializer 的结构形状，禁止放宽 downstream evidence gate。

同时冻结 `EVAL-B463-DIAGRAMPARTICIPANTCOVERAGE1/P1`：incident participant 缺关系且无 typed unproven boundary 时仍可绿签，runner 的任意
边计数是假阳性；修复需模型填写结构化 coverage/boundary，不扫 prose、不由系统造边或代写结论。复合 identity 的 Mermaid alias 表达单列
`EVAL-B464-DIAGRAMENDPOINTALIAS1/P1`，不得按当前字符串过拟合。

状态：`B460/B461=production-closed`；`B462=P0-next`；`B463/B464=P1-queued`；
`raw-prose-hard-gate=none`；`system-edge/answer-rewrite=none`；Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.18 B462 完成：assignment/initializer 不再由 token 单独铸权

Tier 1 与 recovery 现共享 assignment/initializer 行形校验；字段名精确命中、精确 snippet 或恢复定位都不能把纯字段/类型声明提升为
value-flow。真实赋值与成员初始化继续由 repomap feature 或语言无关结构谓词通过。Go/Java/ArkTS/Cangjie 正负矩阵、tool 包与全仓测试全绿，
downstream relation validator 未放宽。

状态：`B462=implemented/full-suite-pass/pending-production-replay`；`B463/B464=queued`；
`raw-prose-hard-gate=none`；`system-edge/answer-rewrite=none`；Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.19 B463 完成：participant 完整性以“已证 incident / 模型未证边界”二选一闭环

本批关闭 r269 的 runner 假阳性：required source-flow 图不能再由任意两条旁支边替代全部 `incident_required` participant 的关系覆盖。
合同只消费 typed participant slate、结构化 Mermaid AST、`edge_anchors` 与 grounded EvidenceItem；不读取 request、thinking、答案 prose。

新 `participant_boundaries[]` 由模型填写并由系统校验/确定性渲染。已证 relation 原样保留；无证 participant 必须留作同 block 的断开节点并
显式标为 `unproven`。系统不补边、不推断 relation、不重写模型结论；重复、stale、unknown/context-only、不可见或跨图借节点均 fail loud。
该字段仅在非 Trace required `AxisFlow` typed participant contract 上投影，Trace 因果投影、自动补齐、时间窗和链上主因权威继续走原独立车道。

semantic-view/cache、schema、wire/normalizer/quarantine、pre/post chokepoint、registry/hint、双语 renderer 与 Trace 负臂均有结构 pin；相关五包
联合测试全绿，全仓复验在提交前执行。B464 的 Mermaid endpoint alias 仍保持独立，禁止把本批 boundary 当作 identity/alias 修复。

状态：`B463=implemented/full-suite-pass/pending-production-replay`；`B462=pending-production-replay`；`B464=next`；
`participant-as-relation-authority=forbidden`；`system-edge/answer-conclusion-rewrite=none`；
`raw-request/model-answer-prose-hard-gate=none`；Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.20 r270：B462 生产关闭，B463 被合法 bare-node 解析缺口阻断

r270 exact-two runner/人工均为 0/2。QF 不再出现字段声明铸造的 assignment 伪边，故 `B462=production-closed`。B463 的 typed
participant boundary 已进入真实 schema、prompt 与 pre-emit gate，但模型按合同提交 `analyzer` 等合法 standalone flowchart 节点时，
visibility helper 因只识别 shaped node declaration 而反复误报 `boundary_participant_not_visible`，形成系统合同自冲突并耗尽四次同类修补。
新立 `B466/P0` 在 Mermaid AST 层补 bare-node statement，不改 relation/evidence 权威。

独立 data witness 新立 `B465/P0`：terminal 同时知道 reference key 为 3、answer item 为 2 却签 satisfied；reconcile 只覆盖非零贡献组，
完整 reference 的零值成员无终态闭包。修复必须来自 typed reference universe 与聚合单位元，不可读 prose/expected answer 代做业务决定。

B464 仍按独立 P1 追踪 source-repair alias provenance；本轮不以放宽 endpoint comparator 解决。§11 已关闭主项不重开，新增事项归入 eval
战役续表。

状态：`MERGE-AUDIT-6/§11=closed`；`B462=production-closed`；`B463=blocked-by-B466`；
`B465/B466=P0-next`；`B464=P1-queued`；`raw-prose-hard-gate=none`；`system-edge/answer-rewrite=none`；
Trace explicit-window/auto-supplement/on-chain causality=`unchanged`。

#### §11.10.21 B465 完成：reference scope 由模型显式二选一，不再接受 false+pair

r270 的 3-vs-2 satisfied 根因已收窄为 planner typed 状态冲突：`complete_reference=false` 与非空 reference path/key 同时在场。
现由 schema 与兼容解码后的精确检查共同要求 true+pair / false+no-pair 两种合法状态；冲突进入一次 bounded JSON repair，模型自行重选，系统
不把 structural candidate 加冕为完整 reference。repl/dataworkflow/dataquery 联合回归通过，全仓复验在提交前执行。

状态：`B465=implemented/pending-production-replay`；`B466=next`；`B464=P1-queued`；
`soft-candidate-hardening=none`；`raw-prose-hard-gate=none`；Trace 车道不变。

#### §11.10.22 B466 完成：participant boundary 与 Mermaid 合法裸节点语法重新一致

共享 Mermaid AST 现识别独立 safe ID 为可见 node，同时拒绝 header/control/directive、箭头、路径与复合语句；该能力不产生 edge/relation。
r270 的四个断开 stage participant 因此可由模型按 B463 合同诚实保留并标 `unproven`，不再陷入
`boundary_participant_not_visible` 重试循环。五包联合回归通过，全仓复验在提交前执行。

状态：`B466=implemented/pending-production-replay`；`B463=pending-production-replay`；`B464=P1-next`；
`participant-as-relation-authority=forbidden`；`raw-prose-hard-gate=none`；Trace 车道不变。

#### §11.10.23 B464 完成：兄弟载体 endpoint alias 与唯一可见有向边对齐

Mermaid source repair 后，diagram-local anchor 已能同步 alias，但 sibling prose/list block 的 typed anchor 仍保留 raw 复合 identity。
现仅当 exact visible label pair 在一张且仅一张图中映射成功、并对应 AST 中真实同向 edge 时机械改写两个 endpoint；复用标签歧义、
断开节点、反向或缺失 pair 均不猜测。该车道不铸 edge/relation/evidence，不扫描 request 或模型 prose。tool 定向、三包联合及全仓测试全绿。

状态：`B464=implemented/pending-production-replay`；`B463/B465/B466=pending-production-replay`；
`fuzzy-endpoint-match=none`；`system-edge/answer-rewrite=none`；Trace 车道不变。

#### §11.10.24 r271：boundary clone 漏字段与 coverage recovery 死路

r271 runner 1/2、人工 0/2。QF 最后一稿的 7 条 model-authored `participant_boundaries` 已过 pre-emit，却被
`cloneAnswerDocumentV2` 漏拷，post-emit 对同稿反报缺失并带 caveat 发货，立 `B467/P0`。data 已由 B465 从错误绿签转为诚实失败，
但 exact `required_material_not_consumed` 到达终态后只允许 final actions，且 previous-first merge 不能替换同 material usage mode，
导致 6 次同错，立 `B468/P0`。两项均为跨场景结构合同问题，不按当前文件名/答案文本拟合。

状态：`B467=P0-next`；`B468=P0-after-B467`；`B463=blocked-by-B467`；`B464/B465=pending-replay`；
`raw-prose-hard-gate=none`；`system-edge/answer-rewrite=none`；Trace 车道不变。

#### §11.10.25 B467 完成：pre/post participant coverage 共用同一持久化事实

`cloneAnswerDocumentV2` 已补 `ParticipantBoundaries` 深拷贝，生产形测试固定
`model emit -> Mutable persistence -> defensive clone -> post-emit oracle` 全链。该批不创建 boundary/edge，也不放宽 coverage；
定向 types/orchestrator/tool 联合测试全绿，全仓复验在提交前执行。

状态：`B467=implemented/full-suite-pass/pending-replay`；`B463=pending-replay`；`B468=P0-next`；
`system-edge/answer-rewrite=none`；Trace 车道不变。

#### §11.10.26 B468 完成：discovery-only inventory 不再冒充 required material consumption

根因不是 runner 过严，而是 workflow coverage 递归把 `material_inventory` 候选 child 的 `source_paths` 当作已消费，导致阶段提前推进；终局
runner 依据真实 `ConsumedPaths` 又正确拒绝，repair 却只得到 final action 域。现 inventory 只保留根工件可寻址，不再贡献候选材料 coverage，
exact missing material 会回开 `cover_required_materials`。

执行错误同时携带 exact typed `InputAlias`；仅当模型为该同一路径显式提交完整的 `planner_distilled` 或
`text_evidence_consumed` 证据时，previous-first merge 才允许替换消费机制。required floor、材料 identity/purpose 与其他材料均不可被改变；
系统不选 mode、不代读/代蒸馏、不扫描 request/answer prose。三包定向与全仓测试全绿。

状态：`B468=implemented/full-suite-pass/pending-replay`；`B463/B465/B466/B467=pending-replay`；
`inventory-as-consumption-authority=forbidden`；`system-mode-selection/material-drop=none`；Trace 车道不变。

#### §11.10.27 r272/B469：data 生产关闭；boundary nested 自愈漏掉自身 allowlist 字段

r272 runner/人工均为 1/2。data 首批真实消费四份材料，终态 `17,0,5`、reference 3/3 与全部 ledger 一致，故 B468 生产关闭。
QF 的 participant boundary 已穿透 B467 clone，但模型把 6 条 exact typed rows 放进 `diagram{}`；通用 sibling hoister 的 allowlist 包含
`participant_boundaries`，shape switch 却遗漏它，导致 quarantine 后再次报 missing 并耗尽重试。该确定性自冲突立 B469/P0 并已修复：
full/patch 均无损上移，schema/retry/precheck 共用明确 sibling JSON 形，系统不造 boundary/edge/relation。联合回归通过，全仓复验在提交前执行。

关系内容仍薄：最终仅保留三条低层 call，六个请求 participant 的主数据流未证。该项单列 B470/P1，待 B469 回放后审计 typed operation
evidence 与 participant identity 是否汇合；禁止以 roster 造边或把 request/answer prose 作为 hard authority。

状态：`B468=production-closed`；`B469=implemented/full-suite-pass/pending-replay`；`B470=P1-after-replay`；
`system-boundary/edge/relation-synthesis=none`；`raw-prose-hard-gate=none`；Trace 车道不变。

#### §11.10.28 r273：boundary 载体关闭，关系表达与赋值端点权威成为下一根因

r273 runner 2/2、人工 0/2。QF 不再出现 nested boundary quarantine 或 degraded 发货，模型提交的 block-level
`participant_boundaries` 穿透全链，故 B469 生产关闭；但六个请求主体中的五个仍断开，最终图仅余底层赋值与 scheduler call，B470 确认为
source-flow typed 表达不足，而非 B469 残留或一次模型波动。

现有 `assignment` 图边方向定义为 assigned receiver→value/type，不能表达 data/state 从 producer 经 merge 到 consumer；模型若用 call/
assignment 代替就会被正确拒绝或得到语义反向图。新增 data-flow relation 之前必须先修 B471：赋值 grounder 目前只证实“该行像赋值”，不校验
model-authored subject/object 是否真是 LHS/RHS，已存在假 typed authority。施工顺序冻结为 B471 exact endpoint grounding → B470 typed
data-flow relation/recipe → 异构关系回放。任何一步都不得从 participant roster、request 或答案 prose 造边。

同批跨语言用例的 endpoint 链虽完整，摘要却把 PyO3 桥写成“绕过 FFI”，并把 try/except 二态写成“硬编码 True”，立 B472 追踪 typed
bridge role 与 branch scope 交接。该项不按 Python 专名做硬门。

状态：`B469=production-closed`；`B471=P0-next`；`B470=confirmed/P1-after-B471`；`B472=P1-queued`；
`system-edge/relation/conclusion-synthesis=none`；`raw-prose-hard-gate=none`；Trace 车道不变。

#### §11.10.29 B471 完成：赋值语法存在不再等于任意端点关系成立

新增共享 exact-line assignment endpoint extractor，把简单赋值/初始化的 receiver/value 与 model-authored `subject/object` 精确对齐；无法唯一
归约的解构、复合、二元/三元表达式不铸有向权威。流程 operation、discover-sink selection、diagram assignment 三面复用同一谓词。
可解析但端点错误的简单 assignment 在 `emit_evidence` 即降为普通 text reference，并向模型披露真实 tuple；复杂 initializer/registration
本地事实保持兼容，但不得绕过下游 exact matcher。该方案覆盖 Go/Java/ArkTS/Cangjie/Rust/Python/C++/TypeScript，不扫描 request/answer
prose，不由系统补边或改结论。正例覆盖全部可表达赋值的 14 种支持语言，Proto 字段编号另有负 pin。

五包联合回归与 `go test ./... -count=1` 全绿。状态：`B471=implemented/full-suite-pass/pending-production-replay`；`B470=P1-next`；
`B472=P1-after-B470`；`assignment-shape-as-endpoint-authority=forbidden`；`system-edge/relation/conclusion-synthesis=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.30 B470 完成：绑定视图与执行方向数据流分型

新增 typed-only `data_flow`，仅由 B471 exact assignment tuple 反向投影为 RHS source/value → LHS receiver；旧 `assignment` 保持
LHS receiver → RHS bound value/type，`return` 保持 function → returned value/type。enum/schema/JSON 教学/validator/repair/机制 capsule 单源接通，
AxisFlow 得到 source-derived advisory recipe；其它 axis 不被改向。反向、伪端点、歧义语法均 fail closed，label/request/prose/participant roster
不铸权，系统不生成答案边或结论。五包联合回归与 `go test ./... -count=1` 全绿。

状态：`B470=implemented/full-suite-pass/pending-production-replay`；`B471=code-closed/pending-joint-replay`；`B472=P1-next`；
`conceptual-flow-as-hard-edge=forbidden`；`system-edge/relation/conclusion-synthesis=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.31 B472 完成：跨语言 bridge role 与 branch state 精确交接

按 r273 证据复核，PyO3 用例不是 endpoint 缺失或纯模型波动，而是 typed handoff 仍把 registration/assignment/return 折成宽泛角色，且没有把
caller target 与 registered export 的精确相等关系显式交给 finalizer。现按 ClaimForm 拆分 invocation/registration/guard/assignment/return/
literal；仅在 current-code-path lane 内做 exact registered-export join。owner-qualified registered callable 可与其独立 call subject 精确接续；
unqualified callable 维持 unresolved，禁止短名碰撞闭桥。

同批新增 fact-only scalar assignment 复算，只有同文件 guard anchor 与真实 LHS 相等时才携带状态写入；多状态明确是 alternatives/updates，
branch-call mapping 在缺少 typed ownership 时保持 unproven。该 scalar helper 不授权任何有向 edge。Explorer JSON 教学要求 registration、call、
guard、assignment、return 各自单条原子 item，并读取完整初始化/异常/fallback 块；不按语言名分支，不扫描答案，不由系统改写结论。

专项 types/agent 与 `go test ./... -count=1` 回归通过；exact-two 回放按提交记录继续。状态：
`B472=implemented/full-suite-pass/pending-production-replay`；`B470/B471=pending-joint-production-replay`；
`short-name-wrapper-guess=forbidden`；`line-order-as-branch-ownership=forbidden`；
`system-edge/relation/conclusion-synthesis=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.32 r274：B472 正证与 accepted-closure/hard-backtrack 冲突

exact-two runner 1/2、人工 0/2。B472 的多状态 guard handoff 已真实进入 finalizer，原“硬编码 True/绕过 FFI”两错消失；但装饰形 registration
subject 阻止 exact export join，最终 ordered member 的 wrapper/core/registration 引用又错配，新增 B474/B475 两个通用精度项。

QF 的 B470 `data_flow` hard validator 正确拒绝 conceptual 边，没有回退为 call 或放宽。冷读纠正：首轮 six-node + six typed boundary + 零边已经
通过 pre-emit；外层 `required_diagram_edge_absent` 精确要求 Explore 回探后，旧 accepted closure 却把 hard pendingViolation 当 stale carry-over
清空，七个 Explore 节点零 dispatch 自动完成。第二稿删掉 requested 节点后才出现 not-visible/unknown。故撤销 alias 双 matcher 归因，B473
重裁为 closed typed RetryState 的 Explore owner 必须压过旧 closure；不读 prose、不造边。施工冻结为 B473 closure/backtrack → B476 operation
evidence supply → B474 registration 双端 exact authority → B475 typed member/citation 同源 → exact-two 回放。

实现只消费 closed typed `RetryState.LastPrimaryOwner=explore + ActiveViolations>0`，不扫描 rendered violation prose；普通 advisory closure
收敛与 finalizer-owned retry 不变。专项 pin、热文件 ratchet 与 `go test ./... -count=1` 全绿。

状态：`B472=partial-production-positive/original-errors-closed`；`B473=implemented/full-suite-pass/pending-production-replay`；
`B476=P0-after-B473`；`B474=P0-after-B476`；`B475=P1-after-B474`；`system-edge/relation/conclusion-synthesis=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.33 r275：operation repair 真实执行仍无主关系，双 PASS 人工判失败

`main@476129c6d` exact-two runner 2/2，人工 0/2。QF 出现两次真实 Explore dispatch，但第二次是首次 finalizer 前的
flow-operation completion repair，并非 r274 的 finalizer hard-backtrack 生产形；B473 只收专项与全量测试，不虚报生产关闭。

第二次 Explore 已拿到六个 typed participant 与 producer/transfer/consumer 指引，也读到 `BuildAgentContext`、`applyStageOutput`、
`recordTaskFinalize`，最终图却只剩两条内部辅助 call，六个用户要求主体全部断开并标 unproven。`mermaid_edge_count` 因无关边签绿，证明 B476
是通用证据供给/repair 定向 gap：应把已读生产文件与 typed operation-site targets 交给 Explorer 做 bounded body 复核；targets 不铸权，只有新发射的
exact call/callback/assignment/initializer/return/precedence rows 才能授权关系。

多态用例的 B472 二态语义保持正确，但 registration 桥、wrapper/core 逐项引用与 fallback 定义引用仍错，B474/B475 继续排队。两例各两次成文拒绝，
根因是上游关系载体不足后模型画概念边再删图；禁止放宽 hard validator、以 participant roster 造边、或扫描 request/thinking/final prose。

状态：`B473=implemented/full-suite-pass/pending-direct-production-witness`；`B476=confirmed/P0-next`；
`B474=P0-after-B476`；`B475=P1-after-B474`；`runner=2/2,human=0/2`；
`system-edge/answer-conclusion-rewrite=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.34 B476 完成：typed operation-site 补采与语法端点单源

r275 的 operation repair 已执行但携带空 Files/Keywords，模型虽读到操作函数，仍用组件/结果角色概括 source/object，导致 exact grounder
拒绝、请求 participant 无 incident relation。现把 repair 目标限定为 analyzer typed participant、citable evidence 与 read closure，最多 8 个
symbol、6 个 production-scope file；规划层可做有界软匹配以优先相关文件，但该结果永不进入 relation authority。

普通与 compact 两个 repair renderer 均发布非空 typed stems 和已读源文件；跨语言 operation guide 明确 call、assignment、return 三类精确
语法端点，禁止用 carrier/component/final-answer 语义标签代替端点。只有 Explorer 复核源码后发射且通过既有 exact grounder 的 row 才能授权边；
系统不补边、不改答案、不放松 required-edge/participant-coverage gate，也不扫描用户或模型 prose。

专项三包与 `go test ./... -count=1` 全绿。状态：
`B476=implemented/full-suite-pass/pending-production-replay`；`B473=pending-direct-production-witness`；
`B474=P0-next`；`B475=P1-after-B474`；`semantic-role-as-syntax-endpoint=forbidden`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.35 B474 完成：注册 owner/reference 唯一合取补齐跨边界 handoff

针对 r275 的 exact registration shape，新增 typed-only owner/reference join：外部 call target、registration 的 system-stamped enclosing owner、
同文件 callable call owner 与 registration expression 的完整 identity token 四者一致且候选唯一时，发布
`binding_endpoint_status=exact_owner_reference_join`。qualified identity 必须同 namespace；缺 producer/owner、错 namespace、歧义或 substring
均不连接。显式 registration subject/object 路径不变。

这是 finalizer advisory evidence，不增删/改写模型答案，不制造 call edge；非执行 binding 在 sequence diagram 继续用 Note，真实执行箭头继续要求
独立 call evidence。Explorer 对所有支持语言统一教学最小跨行 registration range，不含语言/框架专名 hard gate。专项正反 pin、发布接线 e2e pin
与 `go test ./... -count=1` 全绿。

状态：`B474=implemented/full-suite-pass/pending-production-replay`；`B476=pending-joint-production-replay`；
`B475=P1-next`；`registration-binding-is-not-call=preserved`；`system-edge/answer-conclusion-rewrite=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.36 B475 完成：callsite/definition 证据角色不再由模型猜测

r275 的 wrapper/core/fallback 引用错位源于同一 evidence list 同时承载 call 与 definition，却未提供逐 callable 的坐标权限。新增
QFCallChain typed advisory 矩阵：callsite 只证明 exact caller→target，definition 才证明声明/函数体；无定义证据的 target 只能表述为链路
到达边界。定义 join 要求同 source 与 exact identity，或可信 Explorer producer 盖章的 compatible OwnerSymbol；同尾名歧义、未盖章 owner、
跨文件均 fail-closed。

该改动只降低模型引用心智，不扫描 request/answer prose、不新增 hard gate、不补关系、不改模型结论。共享 ClaimForm/identity 逻辑覆盖所有语言，
测试含 Go/ArkTS/Cangjie 同名歧义及 finalizer 生产接线；`go test ./... -count=1` 全绿。

状态：`B475=implemented/full-suite-pass/pending-production-replay`；`B473/B476/B474=pending-joint-production-replay`；
`callsite-definition-role=typed-separated`；`system-edge/answer-conclusion-rewrite=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.37 r276：relation repair 正证与 typed authority 双源红线

`main@c5f94cf49` exact-two runner 1/2、人工 0/2。B473 已获直接生产见证：required zero-edge violation 真实回到 Explore 并 dispatch；
B476 也把补采定向到 topology/stage-binding/Run/context 权威面。QF 仍在 864 秒后输出零边图，因为 finalizer 教学把 checkout-verified
canonical stage sequence 声明为权威，hard diagram gate 却只认 EvidenceItem precedence，不认同一 stage-lane carrier。立
`B477-STAGEAUTHDUAL1/P0`，以共享 typed relation provider 根修 prompt/validator 双源；禁止系统代画或降级证据门。

多态 runner PASS 是假绿：B475 matrix 已发布并正确标记 definition unproven，但 ordered-list 的 native/slow 两项 citation_ref 互换仍通过。
立 `B478-ITEMCITEID1/P1`，对 structured item code identity 与 cited typed/source surface 做精确对齐；不读 free-form item text 或任何原始 prose。
B474 因本轮未发射 wrapper call，仍无生产正证。

状态：`B473=closed/direct-production-witness`；`B476=production-positive/insufficient`；`B477=P0-next`；
`B478=P1-after-B477`；`B474=pending`；`B475=production-consumed/soft-only-insufficient`；
`system-edge/answer-conclusion-rewrite=none`；Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.38 B477 完成：消除 current read stage authority 的 prompt/validator 双源

复核确认 §11.10.37 的 P0 判断准确。旧 finalizer prompt 使用 checkout-verified stage binding/sequence，diagram gate 却只读
EvidenceItem precedence；这会把系统明确教学的真实 `Analyzer→Explorer→Extractor→Finalizer` 顺序当成 unproven，触发重复 Explore 与删边。

新增共享 `stageauthority.LoadReadMode`，结构化校验 checkout 的 binding、conditional pre-stage 与 `AllMainStages`，一次生成教学行和三条相邻
precedence。prompt 与 pre/post diagram validator 现消费同一 provider。仅阶段/agent 的声明别名可映射；逆序、跳级、BusContext/Mutable、write
lane 继续拒绝。该 authority 不证明内部函数调用、载体传输或运行时因果，系统不代画、不代答；Trace source gate 旁路未动。

专项正反 pin 与全仓测试通过。状态：`B477=implemented/full-suite-pass/pending-production-replay`；`B478=P1-next`；
`prompt-validator-stage-authority=single-source`；`system-edge/conclusion-synthesis=none`；
Trace explicit-window/auto-supplement/on-chain root-cause families=`unchanged`。

#### §11.10.39 r277：B477 provider 生效，但缺少同源 precedence recipe

`main@5f3bc07fd` exact-two runner 2/2、人工 0/2。QF 生产日志确认 checkout-verified stage authority 已进入 finalizer prompt，且 validator
消费同一 provider；初始 B477 根修方向正确。残余在消费协议：prompt 只给 canonical facts，没有给模型与 diagram contract 同构的三条
`from/to/relation_kind=precedence` 行。模型多次铸成 call，正确被拒后删掉主图关系。立 `B477b-STAGEAUTHRECIPE1/P0`，由同一 provider 发布
逐边写法；权限仍只覆盖相邻 stage order，禁止把顺序扩成 call/data_flow，禁止系统代画/代答。

70 轮 Explorer 还暴露 `B479-STAGECLOSEBOUND1/P1`：completion gate 不认识共享 stage authority 已满足四个 stage participant，且缺少
BusContext/Mutable 的 typed unproven close lane，造成高成本补读。B477b 回放后再裁，不能用伪造边止血。

多态 case 中 line 40 wrapper 与 line 10 core 被一个 structured item 合并并失去 citation，line 42 exact call 未发射；因此 B478/B474
继续开放。修复只能读 structured item identity/citation 与 typed evidence，不读自由文本，不对语言或绑定框架硬拟合。

状态：`B477=partial/production-provider-positive`；`B477b=P0-next`；`B479=P1-after-replay`；
`B478=P1`；`B474=pending`；`runner=2/2,human=0/2`；`system-answer/graph-authoring=none`；
Trace explicit-window/auto-supplement/causal projection/on-chain root-cause families=`unchanged`。

#### §11.10.40 B477b 完成：stage facts 与 diagram recipe 同源

r277 的 provider 正证后，残余不是证据门过严，而是模型没有拿到与该门同构的关系写法。现由同一 checkout-verified
`ReadModeAuthority.Precedence` 发布三条相邻 `from/to/relation_kind=precedence` recipe，stage/agent identity、source range 与 validator 所消费
的权限完全同源。模型自行决定是否作图及 node ID；系统不代画、不补关系、不改结论。

recipe 只证明相邻 stage order，不能扩为 call/data_flow/artifact transfer/shared-state connectivity/runtime causality。write/Trace 隔离、
checkout drift fail-closed 与既有 hard gate 不变。专项与全仓测试通过。

状态：`B477b=implemented/full-suite-pass/pending-production-replay`；`B479=P1-after-replay`；
`B478=P1`；`system-answer/graph-authoring=none`；`raw-request/model-prose-hard-gate=none`；
Trace explicit-window/auto-supplement/causal projection/on-chain root-cause families=`unchanged`。

#### §11.10.41 r278：B477b 关闭；多仓调用 caller 未按 source owner 校准

`main@da46f5219` exact-two runner 2/2、人工 0/2。QF 图准确消费三条同源 stage precedence recipe，B477b 获直接生产正证并关闭；
BusContext/Mutable 等未证 participant 没有被系统补边，但模型正文仍越过图证据宣称共享状态数据流。15 次 Explore 相比上一轮 70 次明显收敛，
B479 仍需 typed bounded-unproven completion lane，禁止以伪关系换取 completion。

多仓用例确认 `B480-MRCALLER1/P0`：`emit_evidence` 在 primary compatibility graph 找不到另一子仓 source 的 FileInfo 时，caller
normalization 静默失效；line-text grounder 只验证被调用 callee 即保留模型填写的错误 subject。错误 row 随后成为 hard relation authority，造成 final
answer wrapper/core 合并、引用错位与删图。修复必须按 source owner 路由到子仓 graph，由 enclosing callable+exact call relation 规范化方向；owning graph
存在却无法校准时 fail-closed。规则对全部支持语言统一，不读取自由文本，不由系统代画或代答。

状态：`B477b=closed`；`B480=confirmed/P0-next`；`B479=P1`；`B478/B474=pending`；
`runner=2/2,human=0/2`；`typed-but-wrong-directed-call=forbidden`；
Trace explicit-window/auto-supplement/causal projection/on-chain root-cause families=`unchanged`。

#### §11.10.42 B480 完成：source-owner graph 成为 directed call 的 caller 权威

新增 MultiGraph `SourceGraphFile` 窄接口，通过 ground Context callback 把 parent-relative cited source 路由到 active owning sub-repo graph/FileInfo。
emit_evidence 的 call direction、realignment、OwnerSymbol 及 statement owner 校准均改用 owner file；ground 的 definition/call/import Tier-2 match 同源，
不再让 primary compatibility graph 代表其它子仓。

directed call 现在要求 owning FileInfo 的 enclosing callable 与 exact parser/read-line callee 合取。owning graph 可用但 caller 无法证明时降为普通文本证据；
无 MultiGraph/单仓仍走旧 Graph fallback。该规则跨语言、无框架关键词、无原始 prose hard gate，且不由系统生成边或答案。专项 tool/ground/multigraph、
agent/orchestrator 联合套件及 `go test ./... -count=1` 全绿。

状态：`B480=implemented/full-suite-pass/pending-production-replay`；`B479=P1-next-after-replay`；
`B478/B474=pending`；`typed-but-wrong-directed-call=fail-closed`；
Trace explicit-window/auto-supplement/causal projection/on-chain root-cause families=`unchanged`。

#### §11.10.43 r279：owner-routed caller 正证与 boundary repair 风暴

exact-two runner 2/2、人工 0/2。多仓生产日志把 Python native/fallback 与 Rust wrapper→core 三条 caller 全部校准正确，B480 关闭；残余是 fallback
上游 state-producer 证据缺席和 structured item 引用错位，分别记 `B482/P1` 与既有 B478/B475，不回滚 caller 根修。

QF 发生 12 次成文拒绝。validator 每次都在 precise typed diagram/boundary surface 上正确拒绝伪边或不可见 boundary，没有合同自相矛盾；问题是教学
只给总规则，没按 typed participant 发布同构 recipe，模型在大小写/显示节点/边界数组之间试错到第 13 稿。立 `B481/P0`：同一 participant slate 逐项
给 `identity + visible node first-line label + unproven status + no-edge` 写法；保持 hard gate 与模型图所有权不动。

状态：`B480=closed`；`B481=P0-next`；`B479/B482=P1`；`B478/B475=open`；
`runner=2/2,human=0/2`；`system-answer/graph-authoring=none`；
Trace explicit-window/auto-supplement/causal projection/on-chain root-cause families=`unchanged`。

#### §11.10.44 B481 完成：boundary hard contract 与模型写法同源

非 Trace flow diagram prompt 现从 typed participant slate 逐项输出 uncovered recipe：精确 participant identity、可见独立节点首行 identity、可直接复制的
合法 JSON boundary row，以及 `edge_action=none`。该 recipe 只在模型判断没有 grounded incident relation 时使用，不自动创建节点，不授权关系；
context-only 与 Trace/root-cause lane 均不注入。

定向、agent 全包及 `go test ./... -count=1` 回归绿。状态：`B481=implemented/pending-production-replay`；`B479/B482=P1`；
`hard-gate=unchanged`；`raw-prose-scan=none`；`system-answer/graph-authoring=none`；Trace families=`unchanged`。

#### §11.10.45 r280：B481 生产正证与 B483 typed enumeration row identity gap

exact-two runner 1/2、人工 0/2。QF 的 boundary shape reject 从 12 降到 3，最终稿正确消费四条 B481 JSON/node recipe，证明该批减少模型心智且未
放松关系门；但最终仍丢失两条已证 stage precedence，B479 继续开放，禁止以系统补边或伪共享状态 data-flow 闭合。

Cangjie typed lens 与 Principal Enumeration Rows 已完整提供 12 条正确 member/family/location/package，故 runner 的三条 missing row 不是提取器
漏识别。模型在结构化 payload 里交换两条 extend 的行内容与 citation，并用 `(bridge)/(ffi)` 装饰同名 member，绕过只对“完全相同 visible label”
要求 row_id 的规则；后续弱 aggregate citation binder 又把 ffi 行移到 bridge 文件。确认 `B483/P0`：所有 accepted source-inventory principal
item 统一携带 exact typed row_id，row_id 单源选择 family/location/citation；hard check 只比较 structured label/row_id 与 typed registry，不扫描
自由文本，不由系统改写可见行或结论。适用于全部语言和构造族。

状态：`B481=production-positive`；`B483=P0-next`；`B479/B482=P1`；`parser/repomap-loss=false`；
`system-answer-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.46 B483 完成：typed row identity 覆盖 unique/decorated/duplicate 清单行

每个 principal source-inventory structured item 现统一携带 `source_inventory_row_id`；正确的 structured label+exact citation 可确定性补齐该不可见 metadata，
否则 precise hard hint 要求复制 prompt-visible row id。row id 单源绑定 typed family/location/citation，较弱 aggregate binder 不再能把同名行移动到兄弟文件。
括号 display discriminator 只通过既有 structured decorated-label parser 还原 base，不做 substring/prose 匹配；同坐标多 family 在无显式 family 时保持歧义。

系统不读 item text/request/thinking/final prose，不修改可见条目或模型结论。prompt、共享 schema、field schema、normalizer 与 hard gate 有直接接线 pin；
专项与 `go test ./... -count=1` 全绿，待 r281 exact-two 回放。

状态：`B483=implemented/full-suite-pass/pending-production-replay`；`system-visible-answer-authoring=none`；
`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.47 r281：B483 关闭，Trace 链上/背景/双口径隔离通过

exact-two runner 2/2、人工 0/2。Cangjie 首稿携带全部 12 个 exact row_id，逐行 citation/package 正确且零 reject，B483 获直接生产正证并关闭。
一句“public class 不含 sealed/abstract”与同页 typed roster 自相矛盾；上下文已足，记 B484/P2-watch，禁止新增 final-prose hard scan。

Trace 显式窗、自动补齐、因果投影、on-chain semantic class verification、4.600ms effective 与 5.000ms actual 双轴、调度供给次席、背景降级和
frame-causality caveat 全部保持。模型把 4.600ms overlap 误写成 5.000ms 完全重叠，但 finalizer prompt 已精确并置 overlap/actual/interval，记
B485/P2-watch 模型波动；系统不得替换该结论。后续异构复现前不做 type/fixture 拟合。

状态：`B483=closed`；`B484/B485=P2-watch`；`B479/B482=P1-open`；`trace-isolation=pass`；
`system-answer-authoring=none`；`raw-prose-hard-gate=none`。

#### §11.10.48 B479 完成：stage relation 的允许、完成、repair 与 participant coverage 同源

对 r277-r280 冷读确认，B479 是确定性兄弟合同分叉：`ReadModeAuthority.Precedence` 已能让 relation gate 接受三条相邻 stage edge，但 completion、
mechanism repair capsule 和 participant boundary 仍只消费普通 EvidenceItem，导致已证 stage 一边通过、一边被当作未证并在重试中删除。

现由 `stageauthority` 统一判断完整 required read-stage slate，并把同一 checkout-verified precedence 只读投影到四个消费面：Explorer completion 不再重复
补采 stage operation；participant completion/diagram coverage 承认 stage incident；finalizer typed relation capsule/repair 保留三边单组件；boundary recipe
仅面向真正未证的额外 carrier。partial/ambiguous/context-only/write/Trace/checkout-drift 均 fail-closed，BusContext/Mutable 未证关系不会被制造。

该批不扫描 request/thinking/final prose，不生成图或边，不接管模型结论；stage order 不能升级为 call/data_flow/runtime causality。定向、四大包和
`go test ./... -count=1` 全绿，待 exact-two production replay。

状态：`B479=implemented/full-suite-pass/pending-production-replay`；`B482=P1-next`；
`system-answer/graph-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.49 r282 与 B486：B482 关闭；stage 多别名误拒使 B479 保持 partial

exact-two runner 2/2、人工 1/2。Python→Rust 用例完整覆盖 native/fallback state、guard、两个 sink、wrapper/core call 与 module registration，最终答案
准确说明 `_fastlex` 和 ImportError fallback，B482 关闭。

QF 首稿包含三条正确 stage precedence；relation hard gate 将同一节点的 `extractor` 与 `StageExtract` 两个 declaration-backed alias 当成冲突身份，修复
指令随后迫使模型删掉两条边。另有相同 `[Mutable BusContext]` participant blocker 因无关 closure 变化连续触发三次。前者立刻按 provider row 合一：
同一 stage 的多 exact label line 只算一个 endpoint，跨 stage 混写保持拒绝；provider 入口同步要求完整 typed read-stage slate，避免对普通 flow 图扩权。
后者记 `B487/P1-next`，将按稳定 missing-participant typed set 收敛。

状态：`B482=closed`；`B486=implemented/package-suite-pass/pending-production-replay`；`B479=partial`；`B487=P1-next`；
`system-answer/graph-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.50 B487：flow participant 的真实 blocker 与共享 closure 解耦

新增 namespaced typed identifier-set key，flow participant completion 只比较仍缺 incident relation 的 exact participant set。同一 set 在无关 read/
finding/repair churn 后第二次即 caveated convergence；set 缩小/变化才重置一次 focused pass。该 key 不读任何自由文本，也不放宽 relation validator；
系统仍不补图、不造边、不改结论。

生产调用点 key pin、旁路 closure churn、own-set shrink reset、types/tool 全包均绿。状态：`B487=implemented/pending-production-replay`；
`B486=implemented/pending-production-replay`；`B479=partial-await-r283`；Trace families=`unchanged`。

#### §11.10.51 r283 与 B489：stage authority 收口，Trace wakeup CPU 拓扑补齐

exact-two runner 2/2、人工 0/2。QF 三条 stage precedence 全保留且 completion/reject 明显收敛，B486/B487 获生产正证；载体 data-flow 仍因
Explorer 错把字段声明当 initializer、未读取真实 producer/consumer operation 而留空，立 `B488-CARRIERFLOW1/P1`，禁止放宽 relation gate 或系统补边。

Trace 的窗、补采、投影、链上席位与双口径均保持，但模型将 CPU2→CPU1 的唤醒依赖错写成同核竞争。确认 parser 有精确信号而 WakeupEdge 丢 transport。
B489 现把 `waker_cpu`、`wakee_target_cpu` 与 closed `cpu_relation` 贯通 JSON/text/observation/handoff，并用 soft guidance 区分依赖与同核竞争；不读取
自由 prose，不进入链构造、排名或可消除量。

状态：`B486/B487=closed`；`B489=implemented/pending-production-replay`；`B488=P1-next`；
`system-edge/answer-authoring=none`；Trace causal/value machinery=`unchanged`。

#### §11.10.52 B488：载体 data-flow 在角色规划与 operation 证据层根修

冷读确认 r283 的 `Mutable/BusContext` 空边由两层上游软合同共同造成：请求要求连接的 carrier 被 Analyzer 标成 `context_only`，而 Explorer 将无值字段声明
误作 initializer；严格 grounder/relation gate 的拒绝行为正确，不能靠放宽门或系统补边修复。

现由 participant SSOT 教学命名 carrier 在被请求连接时必须为 `incident_required`，并由 flow-operation SSOT 教学读取 exact writer/reader member call、
assignment、return 等操作；declaration-only member 明确是 definition，initializer 必须含 value-bearing binding。completion repair 复用后者全文，消除兄弟提示
漂移。无法证明仍显式留白，participant identity 仍仅供导航。

无 request/thinking/final-prose hard scan，无系统 relation/diagram/结论代写。Trace runtime source-flow 旁路与因果/value machinery 未动。types/agent/tool
受影响包全绿；B489 registry golden 漏钉另由 `e046ae3c1` 收口。

状态：`B488=implemented/pending-production-replay`；`B489=implemented/pending-production-replay`；
`system-edge/answer-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.53 r284：wakeup CPU 正证与 carrier operation repair 新残差

exact-two runner 2/2、人工 0/2。Trace 已贯通 CPU2→CPU1 跨 CPU topology 且未再称同核，原有窗/补采/链上排序/双口径/投影/背景隔离通过；finalizer
无视 explorer 的 `priority_inversion_candidate=false` 自行写反转候选，记 B494/P2-watch 模型波动，不新增 prose hard gate。

QF 的 participant role 与 declaration/initializer 教学均生效，Explorer 已转向真实 orchestrator operation；但其 call row 用了 semantic `writes→Mutable`，
安全降权后没有得到 parser exact caller/callee 修复 tuple，completion 收敛，finalizer 5 次 patch 后 carrier 仍断开。确认 B490（exact call repair recipe）与
B493（repair targets 携 grounded source aliases/operations）为下一高 ROI 小批；B491（Mutable/MutableState visible alias churn）后置。

所有方案只增加 typed/source-derived navigation 与修复信息，不自动创建 relation、diagram 或结论，不扫描 request/thinking/final prose；Trace families 不动。

状态：`B489=production-positive`；`B488=partial`；`B490/B493=P1-next`；`B491=P2`；`B494=P2-watch`。

#### §11.10.54 B490+B493：调用降权给 exact repair，carrier 搜索携源码别名

当模型把 exact call 行写成 semantic writes/binds endpoint 时，工具继续 fail-closed 降为 text reference，但若 parser/已读行能唯一确定 caller/callee，现同轮
给出完整可复制 call tuple；不自动改 evidence，宽语义仍须独立 operation 证明。participant repair 同时补入只与 missing identity 相关的 grounded source
type/method aliases，修复 `Mutable` 搜不到 `MutableState` 的导航断层；上限与 requested source scope 过滤保持。

两项均不读取 request/thinking/final prose，不铸 relation/diagram/answer。`internal/tool` 全包绿，待 r285 exact-two production replay。

状态：`B490/B493=implemented/pending-production-replay`；`B488=partial`；`B491=P2`；Trace families=`unchanged`。

#### §11.10.55 r285：ungrounded evidence 被兄弟 enrichment 重新赋予值事实外观

exact-two runner 2/2、人工 1/2。严格 JSON 数据面一次输出正确结果，零 JSON 修复或成文重试。QF 的 stage authority 继续正确，但 carrier operation 仍未形成；
relation validator 正确删除伪边，最终图诚实断开而正文仍概念性宣称共享流。

本轮确认一个比继续补 prompt 更根本的权限分叉：Explorer 把 `Mutable *MutableState` 声明误发为 initializer，grounder 明确标
`GroundingUngrounded`；finalizer 的 typed enrichment 选择器却不检查该状态，并按 `AnchorInitializer` 将同一拒绝行发布成
`lane=value_fact/assignment_fact/authority=illustrative`。即“拒绝聚合”与“事实回放”同时发生，直接污染模型上下文。立
`B495-UNGROUNDEDLANE1/P1-high`，在 handoff SSOT 上禁止 ungrounded 行进入 factual value/flow/chain lanes；另立 `B496-DECLREPAIR1/P1`，把 grounder
已有的精确 source-shape 失败转成可执行 typed repair，但不自动铸关系或改答案。

B490/B493 新路径因模型未发 semantic call row 而未获 production witness，保持 pending；额外 `Orchestrator` participant 仅单例，记 B497/P2-watch，
不建立 raw-request/prose 硬门。Trace 所有窗、补齐、投影、链上根因分类、业务线索与可消除量链路未触碰。

状态：`B495=P1-high/next`；`B496=P1`；`B490/B493=pending-production-witness`；`B497=P2-watch`；
`system-relation/diagram/conclusion-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.56 B495：修复 ungrounded audit row 的 finalizer 权限泄漏

typed enrichment 现于单一选择入口排除明确 `GroundingUngrounded` 行，阻断“grounder 已拒绝、finalizer 又按 initializer 发成 value_fact”的确定性分叉。
recovered evidence 保持既有可用语义，原始 audit buffer 也继续保留供 Explorer 修复。系统不重写 evidence/答案、不补关系；agent 全包回归绿。

状态：`B495=implemented/pending-production-replay`；`B496=P1-next`；Trace families=`unchanged`。

#### §11.10.57 B496：声明误标不再收到泛化重读提示

当 exact 已读行持有目标标识符、但没有 assignment/member-initializer source shape 时，grounder 现直接说明该行不可证明值流：声明事实可按 definition 重发，
数据流必须另引 writer/reader operation。line 与 line_range 同源；Go、Java、ArkTS、Cangjie 声明负臂及真实值流正臂均已钉住，tool 全包绿。

该修复不自动重分类 evidence、不创建关系或修改答案。状态：`B496=implemented/pending-production-replay`；Trace families=`unchanged`。

#### §11.10.58 r286：B495 关闭，兄弟 advisory 未服从 support scope

QF 生产回放确认 ungrounded initializer 已不再进入 factual enrichment，B495 关闭；图的无证 carrier 边继续 fail-closed。残余 82KB finalizer context 却含
`explorerSearchCache`/预算代码的 same-owner lexical groups，以及仅因共享 `context.go` 被选中的 trace-admission FlowFinding。它们各自为真，但不属于当前
principal support，模型随后继续写出无证共享流。

立 `B498-SUPPORTSCOPECTX1/P1-high`：lexical capsule 复用 support-plan SSOT；FlowFinding 在 typed anchor scope 存在时禁止 file-only coincidence。
该批只降 advisory 噪声，不改 relation/diagram/正文。write 一行修复正确且 source-static-only 诚实降级，无新 gap。

状态：`B495=production-closed`；`B498=P1-high/next`；`B497=P2-model-variance`；Trace families=`unchanged`。

#### §11.10.59 B498：support-scope advisory 收窄批

已把 local lexical capsule 接到 `AnswerSupportPlan` 的既有 typed scope，并修正 FlowFinding 的 file-only coincidence：scope 有 endpoint anchors 时，只有
evidence ID/endpoint anchor 命中可进入可选 finalizer context；scope 无 anchor 时保留 file-only 兼容。相关组与稀疏旧载体正臂、兄弟组与同文件异端点负臂均已钉住，
`internal/agent` 全包绿。

这是 prompt precision 修复，不改变证据权威、关系门、图、正文或 Trace 投影。状态：`B498=implemented/pending-production-replay`；Trace families=`unchanged`。

#### §11.10.60 r287：第一版 support scope 被生产否证，提升为 owner/principal-floor 根修

QF 生产 prompt 仍有 cache/write-policy 兄弟事实：whole-plan scope 的同文件 ±24 行与宽 endpoint 不是 principal connection。已改为 lexical 同 owner，且 required
diagram 的两个 flow 发射面与 lexical capsule 共用实际展示的 bounded `Principal Support Path`；全 agent 绿，待生产重放。此批仍只减 advisory，不改关系/正文。

东湖 trace 人工通过主链与五类修向边界，但系统末尾复发“建议结合源码”。立 `B499-RUNTIMEGENCAVEAT1/P1`，对应旧 G13/G33/G153 未收债；应按 typed
runtime-only authority 抑制 generic accepted-path supplement，不扫描用户原文。状态：`B498=v2-pending-replay`；`B499=next`。

#### §11.10.61 B499：runtime-only Trace 的 bucket 元数据残差不再伪装成源码缺口

亲验 r287 的唯一 user-visible caveat 根为 `ViolFacetUncovered(facet=bucket_label)`；统一 `answer_coverage` 模板把这个结构化标签残差错误翻译成“结合源码”。
现由 shared runtime/source authority 与同 ledger 的 publication-grade Trace projection 合取确权：仅在 current-source 精确 excluded、deterministic
trace query 可独立承载且确定性投影会物化分桶时，`bucket_label` 残差留作 telemetry。source allow/mixed、无投影，以及 must-include/root-cause/
endpoint/uncertainty/具体矛盾等真实披露均保留。

该批不扫描 request/model/final prose，不修改模型答案、根因结论或 Trace 值通道；orchestrator 全包绿，待 r288 生产回放。

状态：`B499=implemented/pending-production-replay`；`B498=v2-pending-production-replay`；Trace families=`unchanged`。

#### §11.10.62 r288：runtime caveat 收债；发现 StageReport 旧载体旁路

exact-two runner 2/2、人工 1/2。东湖 Trace 主链与五类修向、业务线索、双口径和背景隔离均通过，且不再出现“结合源码”，B499 生产关闭。

QF 的 bounded Principal Support Path 已干净，但旧 `Prior Stage Findings` 仍把 128 条全局 FlowFinding 与无关 Primary Evidence 带给 finalizer；最终图无 carrier
边而正文继续过度主张。立 `B500-STAGEREPORTSCOPE1/P1-high`：finalizer-facing StageReport digest 复用 typed principal scope，底层完整 StageOutput/TurnA
不裁剪；required diagram 的 flow 只接受 exact evidence id 或有序双端点，不再以单端点碰巧命中放行。B491 同时获得 exact participant 身份提示反复五次的生产 witness，
只优化 typed repair wording，不放宽 primary identity authority。

状态：`B499=closed`；`B498=v2-partial`；`B500=next`；`B491=P2`；`system-answer/edge-authoring=none`；Trace families=`unchanged`。

#### §11.10.63 B500+B491：同一 support floor 覆盖旧摘要和三类关系上下文

required-diagram scope 现只允许 exact support evidence id 或有序双端点均命中的 FlowFinding；单端点碰巧共享 principal identity 不再进入 diagram seed、
typed enrichment 或 explorer StageReport。StageReport 只过滤 finalizer-facing 展示副本，完整 StageOutput/TurnA evidence/findings 保留，且 compaction total/complete
继续诚实披露。由此关闭 r288 的旧 `Prior Stage Findings` 权限旁路，不生成关系或替换正文。

B491 未放宽 identity gate，仅把失败提示补成可执行的精确规则：typed identity 必须是 Mermaid node id 或首个 visible label，不能只放在短 id 的次级括注/多行标签。
agent/tool 全包绿，待 exact-two 生产回放。

状态：`B500=implemented`；`B498=v3-implemented`；`B491=typed-repair-implemented`；
`system-answer/edge-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.64 r289：单文件近邻仍绕 scope；组件抽象吞掉 exact relation

exact-two runner 2/2、人工 1/2。QF 已找到并在首稿画出三条 Mutable call，但用 A/B/C/M 抽象节点承载 exact callable relation；严格门拒绝后模型删边，终图只剩
阶段顺序。B491 primary identity 提示使拒绝从 5 降到 3 且最终被消费，方向正确但尚未闭环。

B500 StageReport 从 128 降到 57，证明接线生效但不充分：typed anchor scope 仍接受同文件双端点，evidence 仍接受 ±24 行近邻。下一批禁止 anchored required-diagram
的 file fallback，并要求 concrete source evidence exact id/line。另立 B501：软教学用 component subgraph 包含 exact endpoint 节点，关系只画在 exact endpoints；
不系统补图。write plan 正确，但 top-level/change-local 重复同一 probe，立 B502 补“ID 全局唯一、逻辑 probe 只选一个 carrier”的 JSON SSOT。

状态：`B500=v4-next`；`B501=next`；`B491=partial-positive`；`B502=next`；`system-answer/edge-authoring=none`；Trace families=`unchanged`。

#### §11.10.65 B500 v4+B501+B502：principal digest 不再靠 file/近邻扩权

required-diagram 的展示 evidence 现只认 exact support id/line，typed FlowFinding 两端只认 exact endpoint anchor；`file:symbol` 可剥 path 后匹配 symbol，但 file
本身不再放行兄弟路径。无 anchor 旧兼容车道不变。组件逻辑图新增通用 soft recipe：组件 subgraph 内放 exact endpoint 节点，copied relation 只连 exact nodes，
不抽象改挂、不删已证边。ChangePlan JSON SSOT 同步明确 probe id 全 plan 唯一且一个逻辑 probe 只选一个 carrier。

agent/skill/types 全包绿。状态：`B500=v4-implemented`；`B501/B502=implemented`；`system-answer/edge-authoring=none`；Trace families=`unchanged`。

#### §11.10.66 r290：B500/B502 生产正证与 participant role 波动边界

exact-two runner 1/2、人工 1/2。write 计划首次即只在 change-local carrier 放置一个全局唯一 probe，正确应用一行补丁，B502 关闭。

QF 的 StageReport `flow_findings=0,total=14260,complete=false`，对比 r289 的 57 条，证明 B500 v4 已关闭 file/proximity 旁路且没有篡改完整上游账本。
终图仍漏 `Mutable`，但不是 scope 修复误删：Analyzer 将精确 `Mutable` 装饰成 `Mutable (pipeline state)` 并标 `context_only`，与已经存在且到达 prompt 的
State/Context carrier SSOT 直接相违。finalizer 随后只按该 typed role 行事；B501 分层 recipe 在场但未被模型采用。

没有独立 precise typed 信号可安全反转 analyzer role；禁止用 request/entities/final prose 关键词做硬 promotion，也不由系统补图。记
`B503-PARTROLEVAR1/P2-high-impact-watch` 为单次模型合同波动，异构复发后再评估 schema 级交叉约束。Explorer 63 轮/31 mid-loop/11 completion
另入 lifecycle 审计候选，不与关系权限混修。

状态：`B500=v4-production-closed`；`B502=production-closed`；`B501=pending-production-witness`；
`B503=P2-watch`；`system-relation/diagram/conclusion-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.67 r291：关系缺失的下一根因是修复 owner 错配

exact-two runner 1/2、人工 1/2。strict JSON data 一轮正确发布，`planner_distilled` 规则材料与实际 JSON 输入的 typed 消费账一致，无新 gap。

read 关系案首个 finalizer prompt 已有两条 exact typed data-flow recipe；模型将 source endpoint 抽象改挂为组件边，validator 正确 fail-closed，最终删到零边。
真正的系统 gap 出现在 post-finalize：`required_diagram_edge_absent` 无条件归 Explore，忽略 recipe 已到达这一精确信号，导致清草稿、重跑 46 reads 和第二轮
finalizer，累计 21 次拒绝后 1200 秒无答案。立 B504：有 recipe 是 finalizer authoring debt，无 recipe 才是 explorer evidence debt。

状态：`B504=P1-high/next`；`B501=negative-production-witness`；`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.68 B504：typed prompt-delivery receipt 收窄关系修复半径

新增 dispatch-scoped 私有 receipt，由 mechanism relation authority 的同一 producer 在 exact recipe 真正进入 finalizer prompt 时置真；每个新 dispatch 先清零。
零边 violation 在 receipt=true 时只回 Finalizer，false 时仍回 Explore。它不读取用户/模型/最终正文，不生成关系、不改图、不降低 strict edge authority。

agent/orchestrator/types 套件全绿；回归覆盖 receipt 正负、旧 Explore 臂和跨 dispatch 清零。待 r292 生产回放确认 explorer 重启消失，并继续观察模型是否消费
B501 的 exact-endpoint 分层表达。

状态：`B504=implemented/pending-production-replay`；`raw-prose-hard-gate=none`；`system-relation-authoring=none`；Trace families=`unchanged`。

#### §11.10.69 r292：participant boundary 产生“必带且必删”互斥合同

exact-two runner/人工 1/2。东湖 Trace 完整通过显式窗、补采、因果投影、链上五类修向、业务 span、actual/effective、可消除量及背景隔离，证明 B504 未污染 Trace。

read 单 finalizer 20 次拒绝后降级。同一 structured rejection 对 `Analyzer Agent`/`Finalizer Agent` 同时报 `missing_unproven_boundary` 与
`unknown_or_context_only_boundary`。根因为 participant typed display identity 含空格时不属于 code surface，coverage matcher 无法接受它与自己 exact 相等；
边界先被判未知，未置 bounded 的 obligation 再被判缺失。这是确定性不可满足合同，立 B505/P0，不归模型波动。

状态：`B505=P0/next`；`B504=pending-direct-witness`；`Trace=pass`；`system-edge-authoring=none`。

#### §11.10.70 B505：exact typed participant 优先于 alias 兼容

coverage state 恒带原 typed identity，code resolution 只追加 alias；boundary 先 exact 匹配 obligation，零命中才走现有 short/qualified compatibility。
带空格显示身份现可合法声明 unproven，`Foo`/`pkg.Foo` 也不会因 tail alias 抢占 exact boundary。调用/数据流 relation gate、participant role 与图内容均未改。

定向 coverage 与 tool/orchestrator 全包均绿（174.024s / 11.478s），待 r293 生产回放。状态：`B505=implemented/package-suites-pass`；
`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.71 r293：B505 生产关闭；两个新确定性权限错位

exact-two runner 2/2、人工 0/2。read 中带空格 typed identity 已可与自己的 boundary 精确匹配，旧 unknown+missing 互斥未复发，B505 关闭；但 Analyzer
推断的七个组件被 `incident_required` hard presence contract 强制塞成断开节点，五次拒绝后图仍仅一条已证边。立 B507：planning participant slate 不能在
无独立关系见证时等同最终关系权限，需把 explorer coverage guidance 与 final emit authority 分层。

Java 模型原始容量检查引用为正确的 `VisitService.java:18`，确定性 normalizer 却以 aggregate 成员同名为由改成 `VisitController.java:18`。这是系统主动
降质证据的 P0 红线 B506。`AuditLog.record` body 仅 stdout，最终“持久化”另记 B508 terminal-definition evidence gap。

状态：`B505=closed`；`B506=P0/next`；`B507=P1-high`；`B508=P1`；`system-answer/edge-authoring=none`；Trace families=`unchanged`。

#### §11.10.72 B506：更具体的 grounded citation 不被 label-only 行覆盖

principal aggregate binder 增加单调 keep：当前 citation 同时对齐 item exact identity 且证明第二可见 typed attribute 时保持；否则才允许 exact aggregate
support row 修正。旧的 wrong/missing citation 修复、歧义 fail-closed 与接线顺序不变。新增 `VisitService.schedule` 容量检查生产同形 pin和完整 pre-emit pin，
tool/orchestrator 全包绿。

该批不扫描 request/model/final prose，不改正文、结论、关系或图。状态：`B506=implemented/pending-replay`；`B507=next`；`B508=queued`；
Trace windows/causal projection/auto-supplement/on-chain cause families/business clues=`unchanged`。

#### §11.10.73 B507：stage precedence 已教即必须可被所有 validator 消费

r293 精确确认同一 checkout stage provider 的消费分叉：Finalizer prompt 在 typed required workflow + grounded pipeline source 下发三条 recipe，但 completion 与
pre/post validator 仍要求 Analyzer 列全 canonical participant slate。现用 shared relevance admission 统一四面：完整 slate，或 required stage/workflow dimension
+ grounded citable pipeline authority source，二者任一再与 checkout verification 合取。

该 authority 仅含相邻 stage precedence；call、data-flow、carrier connectivity、runtime Trace 均不扩权。partial slate 无 source、Trace、optional/non-flow 与
checkout drift 负臂保留；三消费面接线测试及 stageauthority/types/tool/agent/orchestrator 全包绿。

状态：`B507=implemented/pending-replay`；`B506=pending-replay`；`B508=queued`；`participant-inflation=separate-open`；
`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.74 r294：B506/B507 正证与 participant authority 反例

exact-two runner 2/2、人工 0/2。Java 的容量检查引用稳定在 `VisitService.java:18`，B506 关闭；终点 body 虽已读取但未形成精确 body evidence，
`AuditLog.record` 仍为 definition-unproven 且答案误把 stdout 称为落库，B508 保持开放。read prompt 已出现三条 checkout-verified stage precedence
recipes，证明 B507 四面接线生效；但 Analyzer 猜测的 `Orchestrator`/`Agent` 被无来源升级为 hard participant obligations，迫使 27 轮探索和 7 次成文
修复，最终模型没有消费阶段 recipe。

立 `B509-PARTPROV1/P1-high`：participant planning row 必须逐席携带包含 exact identity 的当前请求 verbatim `source_quote`，否则不得进入硬 obligation。
这是 typed provenance 校验，不扫描关键词、不读取 final prose、不生成关系。显式窗 Trace、因果投影、自动补齐、链上根因/修向/业务线索均不触及。

状态：`B506=closed`；`B507=production-authority-closed`；`B509=in-progress`；`B508=open`；`system-relation-authoring=none`。

#### §11.10.75 B509：Analyzer 猜测不再升级为 participant 硬合同

每个 `diagram_hint.participants[]` 现在必须携带 `source_quote`，且 emit 端在剥离历史会话前缀后的当前请求上验证 quote verbatim 存在、quote 含 exact
identity。只有具备该 typed provenance 的参与者可进入 semantic view 的 completion/finalizer obligations；无来源的旧缓存 IR fail-soft 为非硬义务。

这修的是 hard-authority 来源，不是关系内容：系统不补节点/边、不扫描用户关键词或模型/答案正文、不改变 relation evidence gate。schema、parser、IR、
semantic-view 接线与 legacy 缺来源负臂均有 pin；types/tool/agent/orchestrator/tracediag/stageauthority 包套件全绿。

状态：`B509=implemented/pending-r295-production-witness`；`B508=open`；`system-edge/answer-authoring=none`；
Trace windows/causal projection/auto-supplement/on-chain root causes/business clues=`unchanged`。

#### §11.10.76 r295：B509 生产关闭，B508 由终点函数体证据缺席确认

exact-two runner 2/2、人工 1/2。read 首次 invented participant 被逐席 current-request provenance 精确拒绝，最终四阶段 skeleton、输入输出表与独立
implementation call 均正确，B509 关闭；两次跨组件成文修复另记 B510 soft-context salience，不放宽 validator。

Java 的完整五边调用链与容量 guard 在场，摘要也识别 `AuditLog.record` 为 stdout；但步骤行又称“实现审计落库”。已读 `AuditLog.java:6` 没有对应 typed
body-call item，Finalizer 只有 incoming callsite 且 definition-unproven。故 B508 是 producer/context gap，不是单纯模型波动。

状态：`B509=production-closed`；`B508=confirmed/next`；`B510=P1-watch`；`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.77 B508：typed-selected terminal 的精确函数体调用通道

新 producer 只消费 typed endpoint selection、已证调用图、parser provenance 与 exact read coverage，提升选中终点函数体里的 direct call。Tree-sitter 语言与
Cangjie 共用判据；regex、未读行、sibling terminal fail-closed。Finalizer 将 exact body-call fact 与 declaration/body general authority 分离，允许模型引用
`A -> B @ file:line`，但不由系统解释其业务语义或改写答案。

Java、ArkTS、Cangjie 三面 pin 及 agent/types/tool/orchestrator/tracediag/stageauthority 套件全绿。没有 request/final prose 扫描、系统补边或结论代写；
Trace 显式窗、因果投影、自动补齐与链上根因通道未改。

状态：`B508=implemented/pending-r296-production-replay`；`B510=P1-watch`；`system-answer/relation-authoring=none`；Trace families=`unchanged`。

#### §11.10.78 r296：B508 v1 生产反例与 stage sequence 关系孤岛

exact-two runner 2/2、人工 0/2。Java 中 v1 terminal producer 错发 sibling
`VisitRepository.countOpenVisits -> String.startsWith`，真实 `AuditLog.record -> System.out.println` 缺席。根因是 receiver 已被 parser
规范化为 `AuditLog`，selection 却只比较 initializer 左值 `audit`；选择失败后 discover-sink 又错误回退任意 leaf。该反例确认 B508 不是模型波动。

read 的正文/表正确，但两次成文拒绝后 sequence 只剩六条互不连通的关系。validator 拒绝无证边正确；缺口是完整 stage skeleton 所需 typed recipe
没有以单一可消费结构到达。B510 升为 confirmed，并扩为 relation kind × diagram kind × endpoint identity × producer/teaching/validator/repair 的统一合同审计，
禁止按单 case 放宽或系统补边。

状态：`B508=v1-negative/v2-next`；`B510=P1-confirmed`；`system-edge/diagram-authoring=none`；Trace families=`unchanged`。

#### §11.10.79 B508 v2：规范化 receiver 选择与终点实现读取同闭环

initializer/assignment selection 对 subject/object 两端做 typed endpoint join，使局部 receiver 与规范化 concrete receiver 同源；无 selection 的
discover-sink 不再落到任意 leaf。唯一可定位的本地 selected terminal 若未被一个连续 read range 覆盖，Explorer 发 exact bounded read 软导航；事实仍须
parser direct call + exact read，系统不解释业务语义、不改模型答案。

新增 normalized receiver、无选择 leaf 负臂、连续 body coverage、读后清债与三语言 terminal authority pin；agent/types/tool/orchestrator/tracediag/
stageauthority 全绿。该批不扫描 request/model/final prose，不生成关系或结论；Trace 显式窗、补采、因果投影、链上根因/修向/业务线索未改。

状态：`B508=v2-implemented/pending-r297`；`B510=unified-audit-next`；`system-answer/relation-authoring=none`；Trace families=`unchanged`。

#### §11.10.80 r297：live evidence 与 repair context 两处断链

exact-two runner 2/2、人工 0/2。B508 v2 禁止任意 leaf 回退已获生产正证，且 `AuditLog.record` body 已被模型精确读取；但 active-loop evaluator
看不到同轮 `emit_evidence` 已接受 items，未及时发 selection hint。completion downgrade 排入的 `RepairEmitEvidence` 又只渲染文件、丢 producer rationale，
模型误重试 completion 后低增量收口，终点 body fact 仍缺。两处均为 typed carrier 接线问题，不是模型波动。

stage sequence 用例 634s、7 reject，最终仅三条实现 call；stage 关系和状态传递大量缺席。B510 确认为跨图类/关系类合同问题，后续先做统一矩阵审计，
不按单语言或单样例补图。

状态：`B508=v3-next`；`B510=P1-confirmed`；`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.81 B508 v3：同轮已接受 evidence 与 repair 形状进入单一提示面

Phase promotion、discover selection 和 terminal owner 现合并读取 evaluator snapshot 与 Mutable 的 typed emitted-evidence buffer，消除 active dispatch 的
一轮可见性延迟。`RepairEmitEvidence` compact renderer 同时发布 producer rationale，不再只给文件名；内部 Subject 不发射。

二者都只影响软导航/修复上下文，不从 read 文本造事实、不代写关系或业务结论、不改变 validator。六包全绿，待 r298 生产回放。

状态：`B508=v3-implemented/pending-r298`；`B510=unified-matrix-audit-next`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.82 r298：B508 producer 关闭；B510 mixed repair 缺口导致 1200s 无答案

exact-two 为 Java PASS / read TIMEOUT，人工 0/2。Java 已稳定发布
`AuditLog.record -> System.out.println @ AuditLog.java:6`，错误 sibling leaf 消失，B508 关闭；正文仍把 stdout 称“审计落库”，另立 B511 typed terminal
capability boundary，禁止系统代改模型结论。

read 首轮已收到三条 stage precedence recipe，却在 call/value/participant 混合 reject 后只获负面列表；随后 alias 重映射、无证边重复与一次畸形
`replace_blocks` string carrier，5 reject 后超时。`Mermaid`/`sequenceDiagram` 被模型列作 incident participant 另列 Analyzer 软教学项，不据字符串硬门。

状态：`B508=production-closed`；`B510=P1-confirmed/in-progress`；`B511=P1-open`；`B512=json-carrier-watch`；Trace families=`unchanged`。

#### §11.10.83 B510-A1：关系表达统一矩阵与 mixed typed repair 同源

完成四语义图类（flow/architecture/sequence/call_dag）× 十三 relation kind 的统一审计。closed copy-ready registry 取代 default-allow：未知图类/关系 fail-closed；
contain 永不成为箭头，只能 subgraph；sequence 系统 copy-ready 仅 call/callback，call_dag 仅 call，flow/architecture 可承载除 contain 外的 exact typed relation。
这是系统 authoring aid 的保守面，不限制模型在已有 typed evidence/顺序语境下自行作图。

required diagram 的 repair selector 由“sole call violation”扩为“纯 diagram locus 的 typed relation + required edge + participant coverage”闭集。mixed reject 现在同轮重复：
关系 recipe、exact alias/anchor、以及 `edge_action=none` boundary recipe；citation、table 或未知 violation 混入即拒绝该捷径。系统不绘图、不补边、不删模型关系、不改答案。

当前 class/state/ER/C4 Mermaid families 仍明确 unsupported；仅放开 directive 会绕过 AST/证据门，故列 B510-C 全栈设计，不做语法特判。回归已覆盖 mixed full/patch、
非图/引用负臂、原 sole-call lane 与 4×13/future fail-closed；六包全绿（agent/types/tool/orchestrator/tracediag/stageauthority），待 exact-two 生产回放后关闭 A1。

状态：`B510-A1=implemented/package-suites-pass`；`B510-B=replay-next`；`B510-C=design-open`；
`system-relation/diagram/conclusion-authoring=none`；`raw-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.84 B510-A2：展示语法不再被教学为关系参与者

Analyzer/schema 的 participant 单源教学补充 actor/presentation 分层：只有请求要求连接的 code/runtime/data actor 才进入 roster；图语法、输出格式、列名、渲染
语言/库名只决定展示方式。实现无 token 列表、无 request/final-prose hard scan，state/context carrier 的真实 incident 义务不降级。

状态：`B510-A2=implemented/pending-production-witness`；`system-edge/participant-synthesis=none`；Trace families=`unchanged`。

#### §11.10.85 r299：关系修复通道复绿，值流 evidence 修复通道仍断

exact-two runner 2/2、人工 1/2。Rust 五条 call 在 diagram patch 中完整保留，pipeline mixed reject 也重新收到并保住三条 stage precedence，B510-A1 的
positive repair 已获生产正证。pipeline 仍失败在更上游：模型提交的 assignment endpoints 与 exact source LHS/RHS 不符时，emit 端正确降为
text_reference，grounding note 也精确给出 receiver/value；但结构化 repair 同时标为 `none`，completion 没有等待模型重发，Finalizer 只能把状态载体画成断开节点。

该 gap 不是要求系统从源码替模型画数据流。B510-A3 将只在 typed required-flow 合同下把 parser 已唯一解析的 mismatch 发布为 action-required item repair；原错误
evidence 继续无 relation authority，正确 tuple 仍须模型重发并再次通过 grounding/validator。其它问答、可选图、复杂/歧义 assignment、Trace 因果图均不触发。

状态：`B510-A1=production-closed`；`B510-A2=pending-new-binary-replay`；`B510-A3=P1-high/next`；
`system-edge/answer-authoring=none`；`hard-keyword/prose-scan=none`；Trace explicit windows/causal projection/auto-supplement/on-chain causes=`unchanged`。

#### §11.10.86 B510-A3：关系撤权后的 exact endpoint 修复不再静默丢债

assignment/initializer endpoint mismatch 继续 fail-closed，不自动修 evidence；但在 typed required source-flow 合同下，producer 现在发布 exact
`items[i].subject/object` action-required repair 与 parser-owned LHS/RHS，completion 等待模型局部重发。普通/optional/歧义 syntax 与 Trace/root-cause lane 保持
原非阻塞行为。tool summary、ToolRepair 与 completion 三面同源，不再出现“note 要求重发、结构化 target 却为 none”的自冲突。

多语言与 Trace 隔离回归、核心六包全绿。下一步 r300 用新构建并行回放 pipeline + ArkTS；同时审计“业务分组/短标签 + 组内 exact endpoint”的双层图表达，
只作软教学和模型 authoring 支撑，禁止系统把业务标签直接升级为关系端点。

收口另关一档 mixed repair 覆盖臂：schema-invalid skipped item 与 endpoint mismatch 同批出现时，合并后的 typed repair 同时携带 validation reason、全部 JSON path、
exact LHS/RHS 和 blocking 状态，不再由后构造者覆盖前构造者。新增复合回归及核心包全绿。

状态：`B510-A3=implemented/pending-production-replay`；`B510-A2=pending-production-replay`；
`business-language-diagram-layer=P1-audit`；`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.87 r300：typed assignment repair 正证与两档新 P1

pipeline 回放证明 B510-A3 端到端生效：错误 assignment 不获关系权威，completion 给 exact LHS/RHS，模型局部重发后才允许 closure。其余 call-shaped row 的
caller/callee mismatch 仍只有 note 而无 structured repair，导致 Finalizer 没有足够 call authority；首稿关系门正确拒绝，patch 后图退化。下一批
`B510-A4-RELREPAIR2` 将 call exact tuple 接入同一 producer-owned repair carrier，不改变 evidence/答案所有权。

ArkTS 暴露 `B513-QUALMEMBER1`：`EntryAbility` 明确无 `@Entry`，仍进入 `@Entry` authoritative member_set。错误起于 model aggregate，但系统的 principal 加冕缺少
逐成员 qualifier witness。修复必须基于 typed source-inventory/evidence surface family，不扫描 aggregate label/member note/最终答案。

展示审计另记 `B510-D`（业务角色/动作 + 组内 exact endpoint 双层图）和 `B510-E`（typed requested dimensions 投影表头）；二者均为 soft authoring 支撑，不让系统代写
模型关系或结论。

状态：`B510-A3=production-closed`；`B510-A2=production-positive`；`B510-A4=next`；
`B513/B510-D/B510-E=open`；Trace explicit windows/causal projection/auto-supplement/on-chain causes=`unchanged`。

#### §11.10.88 B510-A4：call 与 assignment 共用精确关系修复闭环

required non-Trace flow/call diagram 下，parser-owned exact call tuple 现在与 assignment 一样成为 action-required、completion-blocking 的局部 item repair；错误 row 继续
text-reference fail-closed，模型需按 exact caller/callee 重发并重新 grounding。repair 还携带 `evidence_kind=relationship`、`predicate=calls` 与 exact
`anchor_symbol`，避免只修 subject/object 后再次被 schema compatibility 降级。

optional/Trace/root-cause 隔离、assignment+call 合并、schema validation 复合臂和核心六包均绿。下一轮以新二进制回放 pipeline + 异构语言关系图，验收 typed capsule 是否
不再因上游 call 欠债而缩水。

状态：`B510-A4=implemented/pending-production-replay`；`system-edge/answer-authoring=none`；
`raw-request/model/final-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.89 r301：call 修复生产关闭；required table 存在零行假完成

TS exact chain 六条边、重试语义与 alias 定义/消费均正确，B510-A4 获异构生产正证。pipeline 则暴露独立载体 GAP：Analyzer 已精确声明
`has_per_member_table=true` 和四个 required dimensions，Finalizer 也创建正确四列，但没有任何 `items`；required-block kind 计数把该空壳当作表已完成，
renderer 将其静默渲染为空。`B510-E-EMPTYTABLE1` 应在 full/patch 共用 block normalizer 上按 JSON 结构拒绝“无 markdown table 且零可见 row”的 table，
不读取 label/request/prose，不替模型写值。

五次图修复另确认 `B510-F-BOUNDARYREPAIR1/P2`：合同要求 disconnected participant 同时具有可见 node 与 `unproven` boundary row，语义正确但当前
提示让模型三轮往返；后续提供逐 participant copy-ready typed recipe，仍由模型 patch。业务展示层继续采用业务分组/动作与组内 exact endpoint 分离，禁止 alias
成为关系权威。

状态：`B510-A4=production-closed`；`B510-E=P1-high/next`；`B510-F=P2-open`；
`system-edge/table/answer-authoring=none`；`raw-request/model/final-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.90 B510-E：table kind、可见 payload 与 renderer 完成态对齐

共用 block normalizer 已拒绝无有效 Markdown table 且无任一可见 row 的 `kind=table`，full/patch 两车道同源；合法 Markdown、fallback row 和 structured
cells 保持通过。修复路径只包含原 block/items JSON path，系统不补表、不改列、不接管模型答案。核心六包全绿，下一轮 exact-two 验证 Finalizer 能否保住用户要求的
stage/input/output/carrier 表，同时观察成文重试是否下降。

状态：`B510-E=implemented/pending-r302`；`B510-F=P2-open`；`system-table/answer-authoring=none`；
`raw-request/model/final-prose-hard-gate=none`；Trace families=`unchanged`。

#### §11.10.91 r302：关系怪异不是波动；stage/workflow typed 载体仍未闭环

pipeline 新二进制已补出四行完整表，`B510-E` 生产关闭；但最终 sequence 只剩实现调用/字段赋值，四阶段 precedence 与三类状态交接缺失。Analyzer 多次 retry 后的
最终 typed participant/dimension carrier 发生退化，display alias 又无法匹配 checkout-verified stage identity；四个断开 participant 连续四轮 boundary repair 后仍
留在图上。故 `20260811-010746.240-88900.md` 的异常是 `B510-G-STAGEFLOW1/P1-high`，不是单次模型波动，也不是 A4 已覆盖范围。

最优方案只在 typed workflow/stage dimension 下保住 stage id/order/carrier，把业务显示名与 exact endpoint 分层，再由模型消费 recipe 作图；系统不补边。普通关系图与
Trace 因果投影保持隔离。`B510-F` 继续以逐 participant 单一 node+boundary recipe 降低修补心智。

Cangjie 同批确认 `B514-SICELL1`：cells-only 表按公开 schema 合法、row_id 也正确，但 exact identity gate 只读 label，导致六次确定性拒绝。成员真值和 package 全部已在
恢复稿，故 parser 无丢失。

状态：`B510-E=production-closed`；`B510-G=P1-high`；`B510-F=P2-open`；`B514=P1-high/next`；
`system-answer/relation-authoring=none`；`hard-prose-scan=none`；Trace families=`unchanged`。

#### §11.10.92 B514：label-first 与 cells-only 的 typed 行身份合同统一

exact row identity 现遵循 renderer 同一结构规则：优先 `item.label`，为空时取且仅取 `cells[0]`；后续 cells/text 不参与成员确权。这样 Cangjie、ArkTS、Java 等所有
source-inventory 表均共享同一载体语义，同时保留 row-id/family/location 的严格门。缺 row-id 的 exact citation metadata 绑定也复用该 helper，不扫描请求或 prose，
不生成成员/表/结论。

完整 `internal/tool` 套件 180.528s 全绿。

状态：`B514=implemented/tool-suite-pass`；`B510-G=next`；`system-member-authoring=none`；Trace families=`unchanged`。

#### §11.10.93 B515：HTML Mermaid 在 participant display alias 上补同源安全自愈

`013600` 的 sequence source 含未引号 dotted aliases，统一 source normalizer 未覆盖，生产 repair counter 为 0。新增的 sequence participant label pass 只 quote `as`
右侧 display；左侧 id、edge、方向与消息保持不变，并从 block normalizer/Markdown dump/HTML preview/terminal renderer 同源消费。无法安全解释的声明不猜修，沿既有 raw
source + render-failure fallback 出厂。

该批只修语法兼容，不补 `B510-G` 缺失的 stage/data relations；二者必须分批验收，避免“图能渲染”误记成“关系已闭环”。

相关四包全绿：mermaidcompat 0.471s、render 1.984s、preview 1.042s、tool 177.508s。

状态：`B515=implemented/package-suites-pass`；`B510-G=next`；`system-relation-authoring=none`；Trace families=`unchanged`。

#### §11.10.94 B510-G：stage/workflow authority 从“全席”扩为 typed 端点连续区间

旧门只认四阶段 participant 全席或 optional requested-dimension，故 r302 的 `analyze/finalizer` typed 端点与 grounded pipeline source 无法激活任何 precedence。新增 shared
selection：在 required non-Trace flow 图下，两个以上各自唯一匹配的 typed stage 端点选择 checkout canonical lane 的最小连续区间；无证据、单端点、歧义、optional、
Trace 均 fail-closed。未匹配 display/presentation participant 仍走 boundary coverage，不影响 stage 区间，也不获得边。

completion、Finalizer prompt、diagram validator 同源消费选中区间；系统只提供 exact precedence recipe，绝不代模型作图。call/data/value/runtime authority 不扩域。
全量回归抓到并修复无 AnalysisIR 的既有 grounded-membership 顾问入口兼容臂，核心六包全绿（stageauthority/types/tool/agent/orchestrator/tracediag）。

状态：`B510-G=implemented/core-suites-pass/pending-r303`；`B510-F/D=open`；`system-edge/answer-authoring=none`；Trace families=`unchanged`。

#### §11.10.95 r303：012817 草稿恢复 GAP 生产关闭，限定词/roster 冲突接棒

新二进制 exact-two 确认 B514 的 label-first/cells-first 统一有效：Cangjie 合法 cells-only 行不再触发 row-id mismatch，最终也未进入
`answer_document_retry_state_recovered`。因此 012817 的用户可见降级属于前置合同自冲突造成的系统 GAP，恢复旧稿本身只是正确的 fail-soft 末端保护。

pipeline runner PASS：四阶段 precedence 已进入最终 sequence，participant display alias 经统一 normalizer 修复，B510-G/B515 均有生产正证；但图仍以内部
`precedence/call` 表述，数据载体断开，且两次 relation repair 与高探索成本尚未消失，故只记 positive 不提前关账。

新 `B516-SISETQUAL1/P1-high`：model-owned set label 明示“不含 abstract/sealed”，同一 typed roster 却强制 `Animal/Service`，导致最终成员、计数、限定词互相矛盾。
后续以 typed requested-family/predicate + row-local family witness 做 handoff，展示 label 不再具有资格确权能力；不扫描自然语言、不系统代写结论。

状态：`B514=production-closed`；`B510-G/B515=production-positive`；`B516=next`；`B510-D/F=open`；
`system-answer/relation-authoring=none`；Trace explicit-window causal projection=`unchanged`。

#### §11.10.96 B516：成员资格不再由模型展示 label 确权

完成 `B516-SISETQUAL1`：每个 principal set 仅在全部 row-local typed `surface_family` 一致时携带 `selection_family`；模型 `Label` 降为
`display_group`，不得排除 typed roster 中的行或改变行数。缺失/混合 family fail-closed，旧无 typed family 场景维持原合同。

Principal Enumeration Rows、Required Principal Member Set、pre-emit obligation roster 与 same-cause fingerprint 统一消费该 typed family；模型仍负责
最终展示措辞和结论，系统不修改 AnswerDocument。实现不扫描请求原文、模型推理或最终正文来判资格，也不触碰 Trace 因果投影、自动补齐或链上根因。

全量 `internal/types`、`internal/agent`、`internal/tool` 回归通过；状态：
`B516=implemented/package-suites-pass/pending-production-replay`。

#### §11.10.97 B517：活跃 SSE 的 4 分钟无正文降级臂退役

确认并修复 `B517-STREAMLIVE1/P0`：OpenAI SSE 原先在 hidden reasoning 持续到达、连接活跃时，仍以 1× `requestTimeout` 的 visible-idle 时钟取消请求，
随后 Finalizer fallback 发布系统降级摘要。该信号不能证明模型不会产出正文，且会越过模型答案所有权。

现退役 OpenAI SSE 的该铸错臂；首字节、真实 byte stall、2× request timeout 绝对上限继续独立生效。回归证明 reasoning 可越过 1× timeout 后正常返回正文，
永久 heartbeat/reasoning 仍受 2× cap 约束。兼容 error/fallback 保留给其他 adapter 的显式 typed failure，不再由活跃 OpenAI-compatible 流产生。

状态：`B517=implemented/package-suites-pass/pending-production-replay`；`system-answer-takeover-on-active-stream=removed`；Trace=`unchanged`。

#### §11.10.98 B510-D/F：exact relation metadata 与业务可见图分层

`B510-D` 已实现为全 DiagramKind 共用的 soft guidance：exact node id/edge anchor 保留证据身份，visible label/message 由模型用业务动作表达；
`relation_kind`、`claim_form`、recipe/validator 词不再作为推荐可见文案。read stage 额外发布 verified responsibility 作为措辞依据，但不生成图或边。

冷审同时确认 `B510-F` 的单行 copy-ready boundary recipe 已由既有提交落地：standalone node identity、同值 unproven boundary row、`edge_action=none` 三项同席，
且 context-only、已证 stage participant、Trace 均隔离。账面状态从 open 纠正为 implementation-confirmed/pending replay。

全 `AllDiagramKinds()`、read-stage 双层表达、prompt hygiene 回归通过。状态：`B510-D=implemented`；`B510-F=implementation-confirmed`；
`model-diagram-authorship=preserved`；Trace=`unchanged`。

#### §11.10.99 r304：业务图层未闭环，局部 relation repair 教学自冲突获实证

exact-two runner 2/2、人工 1/2。Cangjie 的 typed family/roster/display 三面已一致，B516 记生产关闭。pipeline 的 typed 关系与不连通边界没有丢，但最终图仍以
`calls`、`precedence`、Go 函数和源码行作主文案。初始 prompt 已有业务显示指导，失败根因不是模型没有上下文，而是局部修补提示随后反向要求 exact identity
作为第一可见标签并重复 relation/location，属于系统自身合同冲突。

并行发现独立观察项 `B519-CITATIONDETACH1`：错误 item 引用安全剥离后 claim 文本仍保留，可能形成无锚断言；后续需以 structured claim-support 状态引导模型局部
重引/收窄/删除，不能扫描或改写最终正文。

状态：`B516=production-closed`；`B510-F=production-positive`；`B510-D=production-negative`；`B518=next`；`B519=P1-observe`；Trace=`unchanged`。

#### §11.10.100 B518：统一所有 diagram repair 的证据层与展示层

初始 relation capsule、optional 修补、required exact-capsule 修补、required relation-boundary 修补现统一要求：node ID、方向/拓扑、anchor 精确保留；visible
label/message/Note 由模型用业务语言成文。relation enum、claim form、recipe/validator 名和源码位置只作 metadata，不得作为推荐主文案。旧“exact identity 第一可见”与
“整个 Mermaid body 字节不动”的相反教学已退役；只冻结证据拓扑，不冻结展示文案。

实现只调整 prompt guidance 与测试，不扫描 request/model/final prose，不增硬门，不生成或替换业务节点、边、标签或结论。`internal/agent` 全量回归通过；状态：
`B518=implemented/agent-suite-pass/pending-r305`；`model-authorship=preserved`；Trace=`unchanged`。

#### §11.10.101 r305：两项确定性 GAP 与两项效果观察

Trace typed 主链、#1 IO wait、三个 runnable 席和 background 权限均正确，但模型把 11+1+1+1=14ms 写成完整解释 20ms；立 B521，用 exact additive
carrier 边界做 soft synthesis guidance。投影中同一 target sleep 与 cookie/network sleep 有跨来源重复，立 B522 冷审，禁止按同名同值盲合并。

pipeline 最终图不再显示源码行，B518 记 partial；仍有内部词、错误 5-stage 总结与通用表头。额外第二次成文拒绝为确定性 B520：participant boundary payload 到达时，
修补提示没有说明嵌套图必须拆成独立 `kind=diagram` block。runner 2/2、人工 0/2。

状态：`B520/B521=next`；`B522=audit-open`；`B518=production-partial`；Trace chain/background authority=`preserved`。

#### §11.10.102 B520：boundary row 与 diagram block 的 JSON carrier 合同闭环

当且仅当 relation repair 同时携带 typed uncovered-participant rows 时，局部提示明确教授两臂：非 diagram block 中的嵌套 Mermaid 必须拆出唯一独立 diagram block；既有
diagram block 则原位替换。`participant_boundaries`、`diagram`、`edge_anchors` 三字段同属该 diagram block；prose sibling 保持模型所有。

实现不检查原始/最终 prose，不由系统拆 block 或补关系；无 participant payload 的负臂不收到多余教学。`internal/agent` 全量回归通过。状态：
`B520=implemented/agent-suite-pass/pending-r306`；`model-authorship=preserved`；Trace=`unchanged`。

#### §11.10.103 B521：关键路径总量不得被无加法权威的 priced 子集“解释完”

Trace Decision Handoff 在 actual occupancy 与 existing-rule eliminable 两轴同时存在时新增 exact additive ceiling：只有 typed carrier 发布同一 subtotal 才能称完整分解；
否则模型需分别呈现总占用、各可消除席与未计价/未解析链上工作。overlap/未知 pair relation 不得靠自行加减构造 residual。

这是 prompt-only soft guidance，不扫描或修改模型答案，不改变 Trace 排名、投影、补齐及链上/背景权限。`internal/agent` 全量回归通过；状态：
`B521=implemented/agent-suite-pass/pending-r306`；`model-conclusion-ownership=preserved`。

#### §11.10.104 r306：图丢失来自 analyzer 子字段连带失权，不是模型随机删图

pipeline 初稿有完整 sequenceDiagram，随后 relation gate 拒绝部分无证据边；但更早的决定性断点发生在 Analyzer：required sequence hint 带入用户未命名的推断 participant，
首条 provenance 校验失败后模型删除整个 diagram_hint。Answer Surface 于是 `has_diagram=false`，optional repair 合法提供 `remove_block_ids=[d1]`，终稿无图。故该现象是
`B523-DIAGRAMAUTHDECOUPLE1`，不是 Mermaid renderer、B520 carrier 或纯模型波动。

Trace 侧 B521 仅 partial：旧 14=20 字面消失，但模型仍写“前面三个节点各自阻塞时间之和”；B522 的同测量跨查询/跨面重复也继续存在。二者保持开放，不用答案关键词门或
subject+value 粗去重拟合。

状态：`B523=confirmed/next`；`B521=production-partial`；`B522=audit-open`；`B517=no-regression-only`。

#### §11.10.105 B523：required visual 不再被无出处 participant 行连带删除

`parseDiagramHint` 现对“identity 未由 CURRENT request 命名且 source_quote 也无当前请求锚”的 participant 做行级 fail-soft：剥离该行、发布 warning、继续保留合法 sibling participant
以及 diagram kind/required。若 identity 确由请求命名但 source_quote 缺失/错误，仍 fail-loud；非法 kind/role、缺 required/slate、重复 identity 等结构错误也不放宽。

SSOT 教学同步要求模型只删除无出处 participant，绝不因此省略显式 required diagram。该修复消费 typed emit_analysis 字段，不从用户/模型/终稿 prose 关键词重建展示要求，
不由系统画图或补边。r306 同形先红回归已落：无出处 Orchestrator 被删除、有出处 BusContext 留存、required sequence 保持。

`internal/skill`、`internal/tool`、`internal/agent` 全量回归通过。状态：`B523=implemented/package-suites-pass/pending-r307`；
`system-diagram-authoring=none`；Trace=`unchanged`。

#### §11.10.106 r307：B523 生产关闭，typed endpoint 缺载体导致正确业务别名被拒

exact-two runner 2/2、人工 0/2。required sequence 已稳定出厂，B523 生产关闭；742s 任务没有触发旧 1× timeout 降级，B517 获正证。但图的完整时序与业务表达仍未闭环。

四轮 reject 亲证 `B524-DIAGRAMIDENTITYCARRIER1`：node ID 与方向已按系统 typed recipe 正确，模型只缩短 visible label，validator 却把 label 当 endpoint identity，拒绝
精确 n14→n15 边并逼模型删除。根因不是 Mermaid 格式或模型随机反向，而是 edge anchor 没有独立 exact identity 字段，先前“display alias 不影响 authority”的教学与实现不一致。

Trace 另确认 `B525-TRACETIMEROLE1`：20.000ms query window、20.020ms attachment extent、0.020ms post-wakeup delay 在上下文中各自真实但 role 不够突出，模型混写并给 S-state
附加无证业务语义。后续以 typed interval role/state semantics handoff 根修，不上答案数字/词面硬门。

状态：`B523=production-closed`；`B524=next`；`B525=planned`；`B521=partial`；`B522=audit-open`；Trace chain authority=`preserved`。

#### §11.10.107 B524：证据端点与业务显示端点完成结构分权

`DiagramEdgeAnchor` 新增 optional `from_identity/to_identity` pair。copy-ready typed recipe 保留 node ID/方向/关系，同时把 producer-supplied exact endpoint 写入该 pair；schema、JSON
repair、quarantine、strict validator 与 participant coverage 同源消费。visible label/message 不再参与已携带 pair 的 evidence identity 选择，可由模型用业务语言表达。

identity pair 不是新关系事实：仍必须命中同方向 citable EvidenceItem；改指无证据端点必拒；同一 visible edge 出现冲突 pair 也 fail-closed。旧无 pair anchors 保持既有 label resolver，
Trace root-cause diagrams继续走独立 runtime relation authority。系统不代画、不补边、不改模型答案。

相关四包全量回归通过。状态：`B524=implemented/package-suites-pass/pending-r308`；
`model-visible-copy-ownership=preserved`；`system-relation-authoring=none`；`hard-prose-scan=none`；Trace=`unchanged`。

#### §11.10.108 B525：最终 Trace 上下文补齐精确时间角色，不代模型作答

`renderAnswerDocTraceFinalDecisionBoundary` 现从 typed projection 在作答前最后一层重复 selected query window/duration、target 五通道状态分区与 total；attachment extent
只作工件导航，窗外 sched-in 只作独立事件，二者不得替代窗口内状态时长。S-state 机制保持未证；post-wakeup runnable/dispatch 必须由自己的 typed interval 支撑。

同时纠正旧 handoff 的内部矛盾：五个 engine lanes 不再叫“closed four-state partition”，统一为 engine-state partition。实现是 prompt-only soft authority，零 request/model/final
prose hard gate、零系统答案改写，投影/排名/可消除量/自动补齐/链上主因均不变。定向与 `internal/agent` 全量测试通过。

状态：`B525=implemented/agent-suite-pass/pending-r308`；`model-conclusion-ownership=preserved`；Trace causal authority=`unchanged`。

#### §11.10.109 r308：endpoint 分权已生效，关系证据规划未覆盖完整必需图

exact-two runner 2/2、人工 0/2。Trace 的 20.000ms selected-window sleep 不再被 20.020ms attachment extent 覆盖，B525 数值 role 生效；模型仍越过精确 final ceiling 给
S-state 添加“等待下游完成”语义并引用窗外 sched-in，先按非确定性服从问题留作异构回放，不用模型/答案关键词做硬门。主链排名、IO/调度/背景权限无回归。

pipeline 中 B524 exact endpoint pair 消除了显示 alias 误拒，但 typed relation recipe 只覆盖 5 条关系。模型首稿依据 prose/member evidence 画完整流程时，缺证桥被正确拒绝；随后只能删成
两个断开子图才通过，仍显示内部 relation enum/源码位置。立 `B526-DIAGRAMRELPLAN1/P1-high`：required relationship visual 的调查计划和完成条件必须按目标关系补齐 typed relation rows，
并显式发布缺边 roster；不得靠宽松校验、最终 prose 扫描或系统代画关系。

286s 活跃流仍获得模型答案，未发生四分钟降级。状态：`B524=production-positive`；`B525=numeric-positive/semantic-negative`；
`B526=confirmed/next`；`B517=production-positive×2`；`B522=audit-open`。

#### §11.10.110 B522：同一物理状态的强 key 与精确 envelope 进入同一展示域

确认 r308 target sleep 双行是同一物理测量：一条 wakeup publication 有 `StateAccountKey`，一条 state-drilldown publication 无 key；旧代码分别落入强键和 envelope 键，无法相遇。
实际占用表现对 exact continuous sleep/runnable/running envelope 先 census distinct producer keys：最多一个 key 时允许 keyed/unkeyed 镜像折叠；多个非空 key、不同值/区间/状态、多段 hull
全部 fail-open。D/io_wait 不使用 envelope fallback，仍只认精确 producer key。

该修复不删除 ledger/projection 证据，`E5(+2)` 等合并来源披露保留，只避免同一墙钟占用展示两行。定向对抗 pin 与 `internal/tool` 全量通过。

状态：`B522=implemented/tool-suite-pass/pending-replay`；`root ranking/eliminable/model answer=unchanged`。

#### §11.10.111 B526：关系组件新增请求主脊角色，纠正“所有 recipe 都要进主图”

r308 冷读修正：四阶段时序本身已有完整三条 typed precedence，真正不完整的是独立 Orchestrator call 片段。此前把它概括为“required visual 整体缺 relation rows”过宽；若据此增加
完成硬门，会迫使 Explorer 为用户未要求的内部实现继续补边，扩大探索并制造新的重试。正确问题是 carrier 未区分“直接完成请求关系轴的组件”和“同样真实但断开的支撑组件”。

本批在既有 typed edge carrier 上增加内部 `requestSpine` 规划位，只由 request-scoped checkout authority 设置；component 输出新增
`answer_role=requested_relation_spine|supporting_grounded_segment`。当两者断开时，Finalizer 获得 soft selection guidance：主图优先展示完整 spine，supporting segment 留正文或独立图；
不把支撑片段当缺失 hop，不因图面整齐补桥。系统不选业务结论、不写可见边/标签、不改模型答案；strict relation validator 和证据端点合同均未放宽。

定向测试覆盖 stage spine + disconnected call support，确认角色、禁止 synthetic bridge 与 model-authorship 文案均进入真实 authority renderer。

状态：`B526=implemented/targeted-pass/pending-r309`；`scope-correction=recorded`；`relationship-authority=unchanged`；
`raw-request/model/final-prose-scan=none`；Trace=`unchanged`。

#### §11.10.112 B517：四分钟活动流不降级，旧系统代答出口彻底失活

代码历史确认：`09ae86be8` 曾把 active hidden-reasoning 的“无可见输出”当四分钟终止信号，`0a42ce85f` 随后增加系统证据摘要代答；`9a8f3a24b` 已从内置 SSE adapter
移除该 watchdog，因此持续 reasoning bytes 的流不会在 `requestTimeout=240s` 被杀，绝对总 cap 独立为 `2×requestTimeout`。但 Orchestrator 仍保留 legacy typed error →
`AnswerDegraded+SkipAnswerChecks` 的系统代写分支。

该残余现已退役：为守 L1，不改 `runReadSchedulerLoop` 两处调用字节；`finalizerNoVisibleOutputFallback` 兼容 seam 恒返回 nil，358 行系统 AnswerSymbols/Aggregates/Evidence/
Hypotheses 拼装 renderer 删除。遗留 provider 若错误终止活动流，不再得到系统代答；既有 scheduler 只能恢复 model-authored draft、再次请求模型或 fail-loud。单测明确要求 result 为空、
LastError 在场且不存在“降级答案”。`internal/llm`、`internal/orchestrator` 全量通过。

状态：`B517=closed`；`active-stream-at-4m=continue`；`legacy-no-visible-system-answer=disabled`；
`L1=byte-preserved`；Trace/diagram/JSON contracts=`unchanged`。

#### §11.10.113 r309：跨展示面 participant scope 串扰导致无关补证和五次成文拒绝

exact-two runner 1/2、人工 0/2。pipeline 有模型答案但漏 `Mutable`；B526 请求 spine/support segment 角色已准确发布。真正阻塞来自 Analyzer 把表格专属
`BusContext` 铸成 sequence `incident_required`，使 Explorer 和 Finalizer 被迫为用户未要求的图关系补证/断开披露。该信号是 analyzer 语义猜测，不应跨展示面驱动硬合同。

Trace 主链和链上排序无回归，但正文继续混写 sleep occupancy、wakeup latency 与 IO 因果传播；投影面同一 target sleep 仍双行，说明 B522 的实际占用表折叠不能代表整个投影面已闭环。

状态：`B526=production-positive`；`B527=confirmed`；`B528=projection-dedup-open`；`B517=no-regression`。

#### §11.10.114 B527：关系展示面以 typed 引文交集收窄 participant 强义务

在 RequestModel 发布前，对 required diagram 与 required `stage_or_workflow` dimension 做精确 source-quote 对账：只有当该维度引文覆盖至少两个 incident participant，才把它认作
有界关系面；位于其他展示面引文中的 incident participant 从 diagram slate 移除，但仍留在 entities/dimensions 供表格和正文使用。无有界关系面则不动作；明确写入关系引文的
State/Context carrier 继续保留。

该修复只消费 schema-validated enum/boolean/verbatim carriers，不扫描 request/model/final prose 关键词，不放宽任何 relation evidence gate，不由系统生成边或结论。SSOT 教学同步说明
diagram+table 多面隔离；正反 pin 与相关六包全量通过。

状态：`B527=implemented/pending-replay`；`participant-hard-authority=cross-surface-reconciled`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.123 r313 / B531：parser handoff 闭环，participant 系统删席退役

r313 TypeScript 最终补齐 `dispatchOnce -> fetch`、`send -> nextDelay/sleep`，B530 获得生产正证并关闭。Go 案则确认 B527/B527b 的跨展示面 reconciler 违反更高层所有权：Analyzer
已按用户“六者之间的数据流”正确发出六个 incident participant，系统却依据另一个 model-authored answer dimension 及零匹配短引文删除 Mutable/BusContext，导致后续取证义务消失、图中
载体孤立。

现退役该生产 reconciler 及其确定性删席实现。合法 participant slate 原样发布；跨展示面范围依赖 Analyzer SSOT 教学和模型 typed role，复合 participant / bare context-only 的既有
结构 fail-loud 仍保留。answer dimension 标签与 source_quote 不再拥有 participant 删除权，系统不补边、不画图、不改结论。新增 r313 六席+责任维度、table-only 误分类保留及零匹配单席三组
反回归。

状态：`B530=production-closed`；`B531=implemented/pending-r314`；`system-participant-rewrite=retired`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.115 r310：两种 participant scope 均表面通过，人工关系审计均失败

runner 2/2、人工 0/2。图+表案只剩一个 table-only BusContext participant，绕过 B527 首版双匹配阈值；终图把 BuildAgentContext 的证据身份显示成 BusContext，又同时披露
BusContext 关系未证。显式数据流案则保留 Mutable/BusContext，但 Explorer 没有把已读赋值/投影源码铸成 assignment/data_flow row，最终只能画成断开节点。两案共同证明 token、
Mermaid edge count 与 boundary 在场都不能代表用户所求关系已经交付。

立项：`B527b` 覆盖 typed 多展示面零/一匹配；`B529-FLOWRELATIONPLAN1` 补请求参与者对的精确关系证据计划和 producer 发射教学。四分钟活动流没有系统代答回归。

#### §11.10.116 B527b：多展示面本身足以阻止 sibling participant 变硬

required stage/workflow dimension 与至少一个 required sibling dimension 的 typed 合取现作为多展示面权限。该形成立后，diagram participant 必须由 workflow 引文覆盖；不再要求先有
两个匹配 participant，因而 Analyzer 只剩一个错误 BusContext 时也会清空 diagram slate。没有 sibling dimension 的单面关系图保持原 participant 权限，显式 State/Context 连接不受影响。

新增 r310 零匹配 pin及既有三参与者反向 pin；不读 request/model/final prose 关键词，不改 relation validator/模型答案。状态：
`B527b=implemented/pending-r311`；`B529=confirmed/next`；Trace causal authority=`unchanged`。

#### §11.10.117 B529a：关系证据的“分类错误”在首次发射时给出 parser-owned 重发形

r310 已读到真实值传递行，却以 definition/mechanism 发射；旧 repair 只修已经声明为 assignment/initializer 的端点，无法把这类更早的分类偏差反馈给模型。
现在 required source-flow diagram 的已落地单行证据若能唯一解析为 assignment/initializer，且解析端点命中 typed incident participant，工具会返回
`value_transfer_classification` action-required，携带精确 `relationship/assigns/LHS/RHS` 重发参数。原证据不被系统升格，关系、图和答案仍由模型发射与撰写。

Go、ArkTS、Cangjie、C++ 正臂和 Trace/optional-diagram 负臂已钉；实现不扫描 request/model/final prose，不放宽 validator，也不进入 Trace 因果路径。

状态：`B529a=implemented/targeted-pass/pending-r311`；`B529=partial`；`model-authorship=preserved`；
`system-edge-synthesis=none`。

#### §11.10.118 r311：表面全绿仍掩盖 participant 豁免与完整链漏边

exact-two runner 2/2、人工 0/2。Go 案的 Analyzer 把两个用户点名的数据载体合成 `Mutable/BusContext` 并以裸身份 provenance 标为 context_only，绕过既有
incident-participant operation 补采；最终只画四阶段 precedence，两个载体孤立。B529a 未触发不是实现失效，而是 Explorer 从未读到值传递 operation site。

TS 案从同一已读文件漏掉 terminal/native 与 retry 辅助调用，仍把定义行作为 sleep 调用位置并宣称完整。关系 authority 诚实地只发布三条边，但完成/答案层没有阻止“完整”越权。

立项：`B529b-FLOWPARTROLEPROV1/P1-high` 约束 context-only provenance 与复合 participant；`B530-CALLCHAININTERIOR1/P1-high` 补已读
parser/source 漏边闭环。两项均禁止系统补边/改答案，也不进入 Trace 因果路径。

#### §11.10.119 B529b：participant 角色逃逸增加精确 provenance 门，不由系统纠正角色

required source-flow lane 现在拒绝两种 Analyzer typed 自冲突：一个 participant 由多个 distinct typed entity 经分隔符拼接而成；context_only 只携带裸身份自身作为
source_quote。前者必须拆席，后者必须提供更宽的当前请求 verbatim 边界片段或改为 incident_required。工具只返回结构化纠正说明，不自动拆分、升格、补边或改答案。

显式 surrounding-boundary 正臂及 Trace/non-flow 负臂已钉；相关六组全套通过。状态：`B529b=implemented/pending-r312`；
`B529=partial`；`B530=next`；Trace causal authority=`unchanged`。

#### §11.10.120 r312：关系角色修复生效，修复指令可见性与销账权暴露新 seam

exact-two runner 2/2、人工 0/2。B529b 在 Go 真实回放中生效：Analyzer 直接发布独立 `Mutable`、`BusContext` incident participants；但 Explorer 读到初始化/读取点后，
精确 endpoint 修复只存在 ToolRepair carrier，同轮模型只看到 Summary。后续无关成功 emit 又按“最后调用”语义清掉旧债，最终只能发布 participant-unproven 边界并保留孤点。
这不是 B529b 失效，而是新确认的 `B529c-RELREPAIRDEBT1`。

TS 继续漏掉已读 principal body 内 `dispatchOnce -> fetch` 与嵌套 `send -> sleep`，证明 B530 稳定。Go 401s 活动流仍返回模型答案，B517 无四分钟降级回归。

#### §11.10.121 B529c：relation repair 以 syntax key 持久化并即时反馈

required source relation 的 assignment/initializer/call endpoint repair 现携带版本化 typed obligation（anchor/source/line/subject/object），completion 仅在对应 citable typed row 真正入池后
销账；任意无关成功 emit 和部分修复均无权清除。action-required exact Hint 同轮进入工具 Summary，减少一次 completion 拒绝和模型记忆负担。历史无 obligation 的普通 schema repair 保持旧
latest-call 兼容语义。

该批不扫描 request/model/final prose，不自动升格 evidence、不补边、不写图或结论；Trace/root-cause 和 optional diagram 保持隔离。定向测试覆盖多义务持久/部分/全量销账和跨语言即时反馈。
状态：`B529c=implemented/targeted-pass`；`B530=next`；`answer/diagram ownership=model`；Trace causal authority=`unchanged`。

#### §11.10.122 B530：完整调用链的 parser handoff 去除自我遮蔽前提

连续生产回放确认：terminal `dispatchOnce -> fetch` 已存在于 AST repo graph 且精确行已读，但旧 handoff 先要求 principal member_set 全成员 usable；缺边导致成员不 usable，成员不 usable 又让
缺边检测不运行。该循环前提现仅从 handoff 检测器移除，最终 aggregate support/引用合同不放宽。

principal roster 两端唯一匹配的已读 AST call 现在必须由模型补发 typed edge；装饰 member 走代码身份归一化。明确 completeness 下，同一 source/line/caller 已发一条 typed call 而 AST 还有
兄弟 call 时也会逐条提示，覆盖 nested call statement 的部分交接。系统不自动铸边、画图或改答案；regex/unread/ambiguous/runtime Trace 均 fail-open，且不扫描 request/model/final
prose。Rust、TypeScript、Cangjie 正臂和边界测试通过。

状态：`B530=implemented/targeted-pass/pending-r313`；`parser-handoff=precise-read-scoped`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.124 r314：B531 生产正证与 B532 锚类型失权

r314 exact-two runner 2/2、人工 0/2。Go 案六个模型 participant 全部保留，B531 已获生产正证；但已读
`Mutable: types.NewMutableState(request)` 的 AST 是 `member_initializer`，模型却发为 assignment。旧 grounding 接受该行作为普通证据，下游精确端点解析失败，且没有任何结构化纠正，于是
Mutable/BusContext 仍只能以 unproven 孤点收敛。立 `B532-FLOWANCHORKIND1/P1-high`：由精确 AST 行形给模型 copy-ready anchor-kind/endpoint 修复，禁止系统自动升格或补边。

TS 案再次由 B530 补出 terminal fetch，但 Analyzer 本轮把完备性义务发 false，漏掉同语句 `send -> sleep`。与前两轮 true/false 波动对照后登记
`B533/P2-watch`，只做软教学和异构复放，不扫描请求/答案词面硬化。Go 279s 活跃流正常返回模型答案，没有四分钟降级。

状态：`B531=production-positive`；`B532=confirmed/next`；`B533=model-variance-watch`；
`B517=production-positive@279s`；Trace causal authority=`unchanged`。

#### §11.10.125 B532：唯一 parser shape 生成 typed 重发债，不生成关系

required source-flow 的 grounded/recovered 单行证据若 anchor kind 与该行唯一 repomap shape 冲突，现返回
`value_transfer_classification` action-required：`assignment XOR member_initializer` 是唯一硬信号，保守行解析器提供 LHS/RHS，typed incident participant 交集限制作用域。repair 同轮可见并沿 B529c syntax key 持久，直到模型重发正确 citable relationship row。

缺/歧义 AST、非唯一端点、optional diagram、Trace/root-cause 全部 fail-open；不读请求/模型/答案 prose，不改证据、不铸边、不画图、不改结论。Go、ArkTS、Cangjie、C++ 双向消费及 missing/ambiguous 负臂已钉。

状态：`B532=implemented/tool-suite-pass/pending-r315`；`model-authorship=preserved`；
`hard-signal=typed-AST-XOR`；Trace causal authority=`unchanged`。

#### §11.10.126 r315：write 通过，read 暴露 flow repair 可见性 seam

r315 exact-two runner 2/2、人工 1/2。C write 精确 patch 与项目验证均通过；模型误配 probe 被 typed verifier 安全旁路到 Makefile suite。Go 数据流案六 participant 保留，但未读取
BusContext/Mutable 初始化区，B532 无生产触发。completion 内部已有 bounded files/keywords，ToolResult Summary 却只发泛化缺口；模型自猜 `dispatchStage` 搜索失败，最终按
unproven 孤点收敛。立 `B534-FLOWREPAIRVIS1/P1-high`：让 producer-owned soft navigation plan 在同轮可见，不放宽证据门、不系统补边。

状态：`write=production-pass`；`B531=production-positive`；`B532=no-production-activation`；
`B534=confirmed/next`；Trace causal authority=`unchanged`。

#### §11.10.127 B534：两条 flow completion 车道共享当轮 soft 导航载体

`flow_operation_carrier` 与 `flow_participant_coverage` 均把同一 bounded stems/files 计划发布到 Summary、ToolRepair Hint 和 durable closure rationale。目标只由 typed participant、citable evidence、read closure 生成，并显式声明 `not relation evidence`；它只能指导 grep/read，不能清 coverage、铸 edge 或改答案。

模型仍须打开精确 operation line 并自行发射 assignment/initializer/call/return 等关系证据；找不到时继续披露 unproven。无 request/model/final prose 扫描，Trace、write、JSON/Mermaid 与答案所有权不变。定向 pin 覆盖无 operation 与 participant 未覆盖两臂的三面同步。

状态：`B534=implemented/tool-suite-pass/pending-r316`；`navigation=soft-only`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.128 r316 / B535：soft target 已到达模型，但单回合不足以完成 locate→read→emit

r316 exact-two runner 2/2、人工 0/2。Go 案已显示 B534 的 typed stems/files，却只读取 BusContext declaration；第二次 completion 即按 participant typed set 收敛，真实 operation
尚未来得及定位/读取/发射，最终关系仍缺。现把导航顺序写成 repo_map/grep 定位、read 有界行、emit relation，并明确 candidate files 非穷尽范围；participant coverage 改为第三次
相同 close 才收敛，提供两个有界 repair turns。系统不执行搜索、不补边、不改答案。

Trace 案系统权威保持正确，模型把链上席与背景混写、sleep 段称 wakeup latency、频率缺口扩写热节流；先记 `B536/P2-watch`，不以最终 prose hard gate 纠正。187/193s 活跃流均正常产出模型答案。

状态：`B535=implemented/targeted-pass/pending-r317`；`B536=model-variance-watch`；
`B517=no-regression`；Trace causal authority=`unchanged`。

#### §11.10.129 r317 / B537：修复回合已够用，下一断点是 participant 与成员 operation 的 owner 对齐

r317 exact-two runner 2/2、人工 0/2。Go 生产日志确认 B535 的两个 repair turns 均被使用：模型先 grep 定位，再读取并发出
`analyzerEvaluator.BuildInitialInstruction -> ctx.Mutable.ResetPrescanSummary/SetPrescanRoundLimit` 两条 citable call。旧 participant coverage 仍只比较完整 endpoint/最终 leaf，无法把
`Mutable` 与 `ctx.Mutable.<operation>` 的 receiver 对齐，于是精确边在场却仍披露 Mutable 未证，Finalizer 5 次修补后留下孤点。

B537 以 segment 化 exact owner suffix 统一 Explorer completion、Finalizer soft checklist 与 diagram participant hard coverage：业务 participant 可由已证成员 operation 触达，edge anchor 仍保留完整技术 endpoint，关系与方向不变。该桥只消费 typed participant 与 typed edge endpoint，不扫描 request/model/final prose，不自动生成图或答案；boundary node visibility 继续要求 participant 自身可见，防止方法名冒充断开节点。

C++ 回放另确认图层表达债：完整正文含工厂 return、type/virtual dispatch 与静态调用，copy-ready call-DAG 却只能发三个断开的静态 call segment。记
`B538-DYNAMICVISANNOT1/P1-open`，后续应给模型 typed annotation/grouping 载体表达非调用边界，禁止放宽 call authority 或由系统拼桥。用户未显式要求图却被 Analyzer 发成 required 的形记
`B539/P2-watch`；在没有 typed presentation-origin 前，不用请求关键词扫描作硬门。

状态：`B535=production-positive`；`B537=implemented/targeted-pass/pending-r318`；
`B538=open`；`B539=watch`；`B517=no-system-degrade@326s`；Trace causal authority=`unchanged`。

#### §11.10.130 r318：B537 生效，但 sibling table identity 被误升为 diagram 硬席

r318 exact-two runner 2/2、人工 0/2。显式六参与者数据流案中，`Mutable` 在精确成员 operation 入池后立即从 uncovered slate 消失，确认 B537 三个消费面接线正确；
`BusContext` 因当前 evidence 只有 broad call/初始化叙述，仍无可画的 typed carrier relation，最终为孤点。该残余记 `B541/P1-open`，不以 call 重标参数传递，不由系统补边。

更高杠杆的是复合 sequence+table 案：Analyzer 已收到 SSOT 的“table-only actor 不进 diagram”教学，仍把表格括号中的
`AnalysisIR/EvidenceItems/AnswerDocument/Mutable/BusContext` 当图参与者；parser 只靠逐席 verbatim provenance，无法证明这些词属于 sibling 表而非 sequence，所以三个精确身份进入硬 coverage，触发第二个
32 轮 explore、10 次 completion downgrade 与四稿成文，904s 才输出一张 stage spine 加大量断开内部 operation 的图。确认
`B540-DIAGRAMSURFACESCOPE1/P1-high`：应新增 diagram-level verbatim 关系面 scope carrier，并要求 participant provenance 位于该 typed scope；禁止恢复 dimension join 删除、禁止关键词扫原文/答案。

两条任务均超过四分钟后继续等待活跃模型并交付模型答案，未触发系统代答；活动流策略继续为生产正证。最终表中另有调度边界/状态所有者事实错误，后续按上下文精度审计处理，不以答案关键词硬改。

状态：`B537=production-positive/partial`；`B540=next`；`B541=open`；`B517=positive@330s,904s`；Trace causal authority=`unchanged`。

#### §11.10.131 B540：用 typed relation scope 取代跨展示面猜测

`diagram_hint.relation_scope_quote` 现为 schema 必填；required 图必须给出当前问题中图/时序关系面的最短连续 verbatim 范围。participant 的逐席 source_quote 只有完整落在该范围内才获硬席权限；范围外行逐条删除并告警，显式图合同保留。

该实现不消费 requested_answer_dimensions、不扫描 request/model/final prose 关键词、不在发布后改 RequestModel，也不生成关系。六参与者数据流正臂与 sequence+独立表 table-only 反臂同时固定；后者的载体仍保留在普通 entity/维度车道服务表格答案。
相关 tool/skill/types/agent/tracediag/context/orchestrator 套件全绿。

状态：`B540=implemented/pending-r319`；`typed-scope=diagram-level-verbatim`；`B541=open`；Trace causal authority=`unchanged`。

#### §11.10.132 r319 / B540b：全量 scope 越界不能静默变成“用户未指定参与者”

r319 exact-two runner 2/2、人工 0/2。复合 sequence+table 案证明 B540 能排除 sibling 表格示例；逻辑视图案则把过窄的 diagram-level scope 与五个 scope 外 participant 同时发出，旧 parser 逐行 warning 后发布空 participant slate，使用户明确要求的组件/载体关系义务静默消失，最终图退化为四阶段顺序。

现收紧为结构化 payload 自洽门：required 图的原始 participant 非空且全部因 scope containment 失败被删时，emit-analysis fail-loud 要求模型扩 scope 或明确重发空数组；部分越界仍逐行删除。该门只读 typed payload 与 verbatim containment，不扫描 request/model/final prose，不替模型选 participant、不补边、不改答案。两案 367s/400s 活跃流均继续等待并交付模型答案，无四分钟系统降级。

状态：`B540=production-positive/partial`；`B540b=implemented/full-relevant-suite-pass`；`B541=open`；
`runner=2/2`；`human=0/2`；`B517=positive@367s,400s`；Trace causal authority=`unchanged`。

#### §11.10.133 r320：B540b 获生产正证，B541 收敛到声明类型身份投影

r320 exact-two runner 2/2、人工 `uncertain + fail`。Analyzer 已能用同一关系面保留六个用户点名 participant，并逐行剔除 scope 外 Orchestrator；
B540/B540b 的正反臂均获生产证据。Mutable 的精确成员 operation 可覆盖业务 participant，BusContext 却不能与
`o.busCtx.EvidenceItems` 这类精确 variable/member endpoint 对齐，三次成文修补后仍是孤点。确认 B541 的施工边界：跨语言 parser 提供声明类型/receiver-type
typed identity，coverage 只消费该别名；关系、方向、源码端点和答案仍由模型发射的 EvidenceItem 持有。禁止命名猜测、prose hard gate 与系统补边。

Trace 对照案保留显式窗、自动补齐、因果投影、链上类校验/调度供给排序与实际占用/可消除双轴。模型对缺失 B/E instrumentation 有一次“未执行任何工作”
的证据口径越界，只登记软教学债，不改答案。450s 活跃流始终等待模型并发布模型答案，未按四分钟降级或系统代答。

状态：`B540b=production-positive`；`B541=confirmed/implement-next`；`trace-authority=unchanged`；
`active-stream-degrade=forbidden/no-regression@450s`。

#### §11.10.134 B541：静态类型身份与模型已证 operation 精确合取

已补齐跨语言声明类型载体和三个 coverage 消费面。静态类型只能在同文件、精确 binding segment、兼容 declared type、同 declaration/callable owner 的
严格合取下，把业务 participant 对齐到模型已经发射且可引用的 operation endpoint；不改变 relation kind、direction、subject/object 或 source。
definition-only、动态/无类型、歧义、错文件、错 owner 和 runtime artifact 全部 fail-closed，系统不铸边、不绘图、不改答案。

受影响 extractor cache epoch 全量 bump；全部支持语言的 member matrix 明确固定 typed/untyped 行为。Explorer completion、Finalizer soft checklist、
AnswerDocument hard participant coverage 共享一个 types-level 谓词，避免三面语义漂移。相关 types/repomap/index/tool/agent/skill/tracediag/context/orchestrator
套件全绿；Trace intent/root-cause 继续由独立 typed causal authority 处理。

状态：`B541=implemented/pending-r321`；`model-edge-authorship=preserved`；`cross-language=explicit-static-type-only`；
`trace-window/projection/autofill=unchanged`；`four-minute-active-stream-degrade=forbidden`。

#### §11.10.135 r321：声明身份桥缺生产 acquisition，C++ 动态关系仍无可画载体

r321 exact-two runner 2/2、人工 0/2。B541 types/consumer 实现存在，但生产模型没有额外发字段声明，completion 也没有给出 declaration+operation 配对教学，
所以严格 join 无输入；23 个 Explorer 回合与 5 次 Finalizer 拒绝后，Mutable/BusContext 仍为孤点。下一批 B541b 将 parser 唯一静态 binding/type/owner
直接附着到模型已选择且接地的 exact operation，只补身份 metadata，不生成 edge、不改模型端点/方向/答案。

C++ 案再次确认 B538：直接 call 门没有错，但缺虚分发、实现和工厂 return/selection 的 typed 图层载体，首稿图只能被删除。另观察到探索证据中的
`fputs`/`fputc`/`stderr` 在成文时漂移为 `std::puts`/标准输出，记 B542 异构复现与结构化上下文审计，不以终稿 prose hard gate 处理。

状态：`B541b=implement-next`；`B538=open`；`B542=watch`；`model-authorship=preserved`；
`active-stream=no-degrade@440s`；Trace causal authority=`unchanged`。

#### §11.10.136 B541b：operation 自带 parser 静态身份，不再消耗声明证据回合

emit-evidence 现从模型已选择且接地的 exact operation 端点提取 parser-owned 静态 binding/type/owner；同文件、同 callable owner、完整 segment、非歧义
四项合取后才发布 system identity metadata。三个 coverage 消费面直接复用，不再要求第二条字段 definition evidence。helper 参数未成为 relation endpoint
时不覆盖 carrier；动态/无类型、错 owner、命名近似、同 binding 冲突全部 fail-closed。

该实现不改变 operation relation/方向/端点/源码，不生成边、不改图和模型答案；soft repair 仅指导模型优先找含 carrier binding 的真实 assignment/member
operation。相关套件全绿，待 r322 exact-two 生产回放。

状态：`B541b=implemented/pending-r322`；`model-operation-authority=preserved`；`trace-authority=unchanged`。

#### §11.10.137 r322：B541b 正证与新 P0——同一成文 prompt 的参与者状态自冲突

r322 exact-two runner 2/2、人工 `pass-with-caveat + fail`。B541b 已把 `o.busCtx.*` operation 对齐到 BusContext；最终 Current-Source Authority
明确称六个点名 participant 全部 incident。可是 Diagram Contract 在同一 prompt 仍携带早期计算的 BusContext/Mutable uncovered boundary recipe，形成
“已覆盖/未覆盖”同时必真的 typed 合同冲突。Finalizer 四次修补后保留 BusContext 孤点并删除载体数据流，图没有完成用户要求。立
`B543-PARTICIPANTCAPSULESSOT1/P0-high`：最终 closure 的 participant state、boundary recipe、relation recipe、pre-emit check 必须同源同代；不补边、不放宽门、
不扫描 prose。

另立 `B545-ASSIGNENDPOINTSSOT1/P1-high`：采集教学把 `strings.TrimSpace(out.FinalAnswer)` 称 exact RHS，完成 repair 却要求 parser 截取的
`strings.TrimSpace`；把 canonical operation tuple 独立附为 parser metadata，让所有硬门共享，模型保留可读 subject/object。Trace 案核心通过，仅把调度供给候选扩写为
同步“阻塞”的机理口径越权，记 `B544/P2-soft`，只优化 typed 上下文，不硬改终稿。

两案中 Trace 显式窗、因果投影、自动补齐、链上根因与实际占用/规则可消双轴未回归。464s 活跃链路持续有进展且正常交付模型答案，未发生四分钟降级。

状态：`B541b=production-positive/partial`；`B543=P0-next`；`B545=P1-next`；`B544=P2-soft`；
`active-stream=fixed-threshold-degrade-forbidden`；Trace causal authority=`unchanged`。

#### §11.10.138 B543：消除 participant incident/uncovered 双合同

Diagram Contract 与 Current-Source Authority 现共享最终 evidence closure、relation edge 投影和 participant coverage 谓词；stage precedence、exact
receiver/member 与 parser-stamped declared binding 不再分车道判断。pre-emit 进一步区分“typed 关系在且已画”“typed 关系在但未画”“typed 关系不存在”：
前者拒绝 stale boundary，中者要求模型自行呈现一条现有证据边，只有后者允许 unproven 孤点。

系统仍不画边、不选择边、不改 relation/endpoint/方向或答案；判据只读 schema-valid typed participant、citable EvidenceItem 与 verified stage authority，
无 request/model/final prose 扫描。全仓 `go test ./...` 通过，Trace 与活动流策略未改。

状态：`B543=implemented/full-suite-pass/pending-r323`；`B545=next`；
`model-answer/edge-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.139 B545：完整 RHS 与 canonical relation endpoint 解耦

assignment/initializer 允许模型逐字复制接地源码的完整 RHS expression，parser 从同一行独立提取唯一 primary source identity 供 completion、call-chain、
participant coverage 和 diagram relation 使用。这样 `strings.TrimSpace(out.FinalAnswer)` 不再被系统先称 exact、后要求缩成 `strings.TrimSpace`；可读事实保留，
硬关系仍为 canonical tuple。

不同表达式、歧义/复合赋值与字面量不获 relation authority。方案无 EvidenceItem/schema/hash/cache 变更，不生成边或答案；Go、Java、Python、ArkTS、
Cangjie、Rust、C++ 与 composite construction 回归及全仓测试通过。

状态：`B545=implemented/full-suite-pass/pending-r323`；`endpoint-contract=single-source`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.140 r323：B543 生产闭环；参与者图仍有 visibility 与 repair-map 两层债

r323 exact-two runner 2/2、人工 `pass-with-caveat + fail`。B543 获生产正证：同一成文 prompt 只发布六席全部 incident，不再出现
BusContext/Mutable uncovered recipe。B545 的完整 RHS 自冲突未复现，但原 witness 未被本轮选择，故只记无回归、待 exact production witness。

新确认 `B548-PARTICIPANTVISIBLE1/P1-high`：最终 Mermaid 没有 BusContext，也没有四 Agent 与 Mutable/BusContext 的业务数据流，却因内部 operation
可经 owner/declared binding 对齐 participant 而通过 coverage。关系权威与展示完整性必须拆开：typed operation 证明 incident；用户点名 participant 还必须以
业务 node/subgraph identity 可见。系统不补边、不选边、不改模型答案。

新确认 `B549-PARTICIPANTRECIPEMAP1/P1`：六席 `available_typed_incident_edge_not_rendered` 连续三次重复，修补提示只给全局 relation capsule，
没有逐席列 bounded exact candidate recipes，模型心智与成文重试过高。应发布候选映射但保持模型选择权，候选不得升级成“全部必画”。

Trace 157s 首稿通过，显式窗、因果投影、自动补齐、链上根因、实际占用/规则可消双轴与确定性语义点均无回归；模型重叠 IO 求和和“完全来自 S 状态”
继续归 B544 软指导，不扫描终稿硬改。读案 508s 活跃流全程未降级，固定四分钟系统代答继续禁止。

状态：`B543=production-positive`；`B545=pending-exact-witness`；`B548=next`；`B549=next`；
`active-stream=no-degrade@508s`；Trace causal authority=`unchanged`。

#### §11.10.141 B548/B549：业务 participant 不再被内部 operation 隐身；修补候选逐席发布

已把 relation incidence 与 visual identity 拆成两个精确条件。owner/declared-binding 对齐继续证明某个 typed operation 与 participant 相关，但不再能单独证明
该 participant 已在用户可见图层出现；业务 identity 必须存在于 node/participant/subgraph/group，或由模型显式 endpoint identity 绑定到可见业务标签。
verified stage alias 与显式 identity 保持兼容，系统不规定固定内部名或布局。

每个 `available_typed_incident_edge_not_rendered` / `required_participant_identity_not_visible` 缺口现在附最多 3 条、按 participant 分组的 typed candidate：
关系类型、canonical 方向、精确 endpoint、source location 原样发布。它只帮助模型从已有证据选择和表达，不把候选变成必画全集，不创建/替换边或答案。
availability、candidate 与 pre-emit coverage 共用 citable operation/verified precedence 权威。

定向与相邻五包全绿；无 raw request/model/final prose hard gate，Trace family 被显式排除。B517 同步复核：活跃 SSE 超过约 4 分钟不会降级或系统代答；
只有首包未到、真实静默、断链或 2× 绝对上限可产生 typed failure，恢复链不得合成模型结论。

状态：`B548/B549=implemented/relevant-suites-pass/pending-r324`；`model-authorship=preserved`；
`active-stream-fixed-4m-degrade=forbidden`；Trace causal authority=`unchanged`。

#### §11.10.142 r324：B548 正证；跨关系图投影与泛化附注仍有权限缺口

LoopController 生产回放在一次 implements 方向修正后完整保留 12 个实现节点、12 条 `implementer -> interface` typed relation、文件与 citations，
B548 获正证且未过硬。与此同时，完整 typed 集合仍被系统追加“部分项证据支持稍弱”，立 `B550/P1`：附注必须读取最终 principal relation/row
coverage，不能把早期 soft advisory 跨题族翻译成全局弱证据。

C++ virtual/factory 回放正文正确，最终图却被 copy-ready skeleton 压成两个 direct-call 断片；typed guard、factory return/construction、subtype/binding 与
virtual dispatch 已在上下文，但 skeleton 的 diagram-family 投影只保留 call。立 `B551/P0-high` 并归并 B538：先形成 family-neutral typed semantic graph，
再映射 call/return/guard/type/binding 到可表达的 Mermaid edge/decision/note/group；不得强转 relation kind、静默删除核心关系或系统补造业务边。

状态：`B548=production-positive`；`B550=next`；`B551/B538=next-highest-ROI`；`runner=2/2`；`human=1/2`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.143 B551-A：修复 runtime-selection 义务随 endpoint demotion 丢失

已新增与 endpoint mode 正交的 typed `runtime_selection_required + runtime_selection_source_quote`。只有 analyzer 明确布尔裁定且 quote 是当前请求连续逐字片段时，
exact/discover_path 才进入选择证据完成门；discover 维持既有语义。`NormalizeCallChainEndpointProfile` 的 exact/discover 候选降级现会完整携带该义务，explorer、
pre-complete、terminal body 与 finalizer context 共用 `RequiresRuntimeSelectionEvidence()` 单源谓词。

这关闭了 C++ 案“问题明确问 sink 如何选择，但 code identity 降级同时把选择关系当可选背景”的采集 GAP；系统只请求模型发射 citable registration/
assignment/initializer/factory-return，不从 deterministic candidate 自铸关系。mixed-relation 图在错误首稿后的 copy-ready 恢复仍需 r325 判断：若采集补齐后仍退化为
call-only 图，再单独修视觉恢复，不把两个权限层混成一次硬扩域。

状态：`B551-A=implemented/relevant-suites-pass/pending-r325`；`B551-B=pending-r325`；`model-authorship=preserved`；
`active-stream-fixed-4m-degrade=forbidden`；Trace causal authority=`unchanged`。

#### §11.10.144 r325：B551-A 获生产正证；B551-B/B550/B552/B553 分层立案

r325 exact-two runner 2/2、人工 0/2。C++ 与 Python 均由 typed runtime-selection 门阻止过早 completion，并在模型补齐 factory return、registration、
assignment/call 后放行，B551-A 生产正证成立。成文侧仍把 mixed typed relation 投影成 call/callback-only skeleton，selection、return/construction、guard、
registration/type/binding 与 assignment 无图层载体；正确概念图经硬拒后被删，确认 `B551-B/P0`。施工必须在 typed semantic graph 到 Mermaid family 的映射层
保留非 message 关系，不得改关系种类/方向，不得系统画业务边或代写模型答案。

`B550/P1` 确认为 `answer_facet_coverage` 的披露失真：缺口是 typed branch guard / diagram spine 等 exact facet，出厂却只有“某些维度可能不充分”。
应按 typed cluster value 给出局部可行动附注，完整 coverage 时不发。`B552/P1` 为 selection evidence source-line 精度：factory return 事实引用了函数签名行，
completion 只验 citable shape 未验 source-owned return。`B553/P1` 为纯 title-only 空块触发多轮 missing-id；只对无任何语义 payload 的块做机械剔除可安全降低重试。

活跃流红线补钉：累计 4 分钟本身不得触发降级；heartbeat、reasoning、assistant/tool chunk 或工具执行均表示仍活跃。只有 typed 首包超时、真实停滞、断链、
独立绝对上限或重试耗尽可进入恢复；恢复仅能发布模型已有草稿并披露，不能系统合成答案。该批未触及 Trace，显式窗、自动补齐、因果投影、链上根因与
实际占用/规则可消双轴保持不变。

状态：`B551-A=production-positive`；`B551-B=P0-next`；`B550=P1-next`；`B552/B553=P1-open`；
`system-answer/edge-authorship=none`；`fixed-four-minute-degrade=forbidden`；Trace causal authority=`unchanged`。

#### §11.10.145 B551-B：mixed relation 在 sequence 恢复稿中不再被 call-only 压扁

copy-ready sequence 现将 typed relation 分载：call/callback 维持 message arrow + exact anchor；guard、register、return、assignment/data-flow、type relation、
precedence、import、observe、temporal、contain 等改用无 anchor 的 `Note over`。同端点多 relation 的歧义组也只发 Note，不猜选箭头。Mermaid parser/validator
继续只把真实箭头纳入 edge authority，Note 不能偷渡调用关系。

可见 Note 是模型作者需要替换的业务措辞占位，事实源仍是已有 typed evidence；系统不选业务关系、不补边、不改方向或答案。六个相关包套件全绿，Trace 与活动流策略未改。

状态：`B551-B=implemented/pending-r326`；`sequence-mixed-relation=lossless-notes+strict-edges`；
`model-authorship=preserved`；`B550=next`；Trace causal authority=`unchanged`。

#### §11.10.146 B550：精确 facet 缺口不再降级整份答案

answer-coverage caveat 现仅在 surviving family 全为 exact `answer_facet_coverage` cluster 时，按 typed facet 发布具体用户可读缺口；12 个现有 facet 均有中英文映射。
同 family 混入 block/legacy/richness/unknown shape 时继续使用保守 fallback，防止伪精确。accepted surface 的既有 telemetry suppression 先执行，因此完整 relation/row 或
Trace projection 已覆盖的内容不会被早期 advisory 重新降级。

该逻辑不读请求/模型/终稿 prose，不改正文与结论。orchestrator/types/tracediag 测试通过，Trace source authority 与因果投影未改。

状态：`B550=implemented/pending-r326`；`exact-facet=actionable-local-disclosure`；`covered-content=no-global-downgrade`；
`B552/B553=open`；Trace causal authority=`unchanged`。

#### §11.10.147 B553：重复标题空壳可无损吸收，独立标题继续 fail-closed

full emit 在 block 无 id/kind、除 title 外全部字段为空，且该 title 与另一合法 structured block 逐字相等时，删除重复空壳并保留真实块。唯一标题、不同标题或带
facet/claim/role 等任何 annotation 的对象不吸收，继续给出精确 schema rejection。该规则是结构化 exact equality，不扫描 prose 语义、不改模型内容。

tool/types/orchestrator/tracediag 套件全绿。状态：`B553=implemented/pending-r326`；`retry-noise=exact-duplicate-only`；
`model-content-deletion=forbidden`；`B552=open`；Trace causal authority=`unchanged`。

#### §11.10.148 r326：B551-B/B550 生产正证与两项关系表达残余

r326 exact-two runner 2/2、人工 `pass-with-caveat + fail`。C++ 修复稿保留四条 exact call edge 与 factory-return Note，证明非 message typed relation
可以在 sequence 中无损保留且不扩大 invocation authority；Python 的 exact answer-coverage 附注只指出缺少“关系图主干及其已证关系”，B550 精确披露获正证；
两案均未再出现 duplicate title-only 重试。

新确认 `B554-UNARYSEMANTICNOTE1/P1-high`：已接地的 C++ branch guard 只有 subject+condition，当前 binary relation recipe 在缺 object 时直接过滤，导致图层丢失
运行时选择条件。最优形是一席 participant 的无 anchor Note；禁止补 object、自环或关系边。新确认 `B555-OPTIONALDIAGRAMESCAPE1/P1`：Python 已有 call/callback+
factory Note 的可靠 capsule，repair teaching 仍把删图作为同等最省力出口，模型遂删除整图。只应软引导优先替换已有 typed skeleton，保留模型判断无视觉价值时删除的权限。

B552 本轮错误签名行 registration 被 grounding 拒绝，随后 exact guard/return 行获接地，改记 `production-self-corrected/watch`，不新增重复硬门。
活动流累计 4 分钟继续不是降级条件；系统不得因仍活跃但耗时较长而生成或替换模型答案。

状态：`B551-B=production-positive/partial`；`B550=production-positive`；`B553=no-recurrence`；
`B552=watch`；`B554/B555=next`；`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.149 B555：有用 typed skeleton 成为可选图修补的首选软路径

optional diagram 被关系门拒绝后，repair teaching 现优先建议局部采用已有 verified skeleton，保留 exact topology、Notes 与 anchors，避免模型因重建成本高而直接删除
有价值关系图。删除仍是合法的模型选择，但收窄为“模型判断该骨架不比接地正文增加结构价值”时的出口；没有新增保图硬门、prose 扫描或系统绘图。

状态：`B555=implemented/pending-r327`；`optional-removal=retained`；`model-authorship=preserved`；
`B554=next`；Trace causal authority=`unchanged`。

#### §11.10.150 B554：binary relation graph 外新增封闭 unary guard Note 车道

citable guard 若只有 subject/owner + exact condition，不再因 object 为空而从视觉 authoring capsule 消失。系统以独立 typed unary carrier 发布，并在 sequence
skeleton 中复用 participant alias、生成无 anchor 单席 Note；binary guard 保持原路径，二者不重复。Note 不铸 object、自环、edge anchor 或调用权威。

能力矩阵当前只开放 `sequence + guard`，其他 diagram family 和 relation kind fail-closed；这避免为了当前 C++ case 把任意单实体事实泛化成图边。
相关七包测试通过，模型仍负责 visible business wording、关系取舍、布局和结论。

状态：`B554=implemented/pending-r327`；`typed-unary-authority=closed-matrix`；
`system-edge/answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.151 r327：flow-family unary 与 optional boundary repair 两层残余

r327 exact-two runner 2/2、人工 0/2。B554 的 typed unary acquisition 两案均出现，但 call_dag/flow skeleton 不消费单实体 guard，故只获 partial 正证。
最优扩展是无 edge/anchor 的 standalone fact node；禁止为了 flow 语法制造 guard arrow、自环或虚拟 object。

Python 暴露 `B556/P1-high`：AxisFlow 合理扣留 whole-diagram skeleton 时，optional repair 没有重复 exact relation-boundary recipes，软引导失去可执行载体，模型最终删图。
应提供 bounded recipe subset，仍由模型决定布局/措辞/是否保图。C++ 暴露 `B557/P1`：annotation count=0、kind=return 与“已由 Note 保留”同页矛盾；receipt 必须从实际
rendered annotations 同源，未渲染关系只能披露 omitted。

C++ 5 次修补、319s 后仍由模型成功交付，固定 4 分钟未触发系统降级；多轮收敛记 B558/P2-watch，待上述根因修复后再判。

状态：`B554/B555=partial`；`B556/B557=next`；`B558=watch`；`model-authorship=preserved`；
`active-stream=no-degrade@319s`；Trace causal authority=`unchanged`。

#### §11.10.152 B554/B556/B557：现有图 family 的 unary 载体与修补 receipt 单源

unary guard 现对 sequence 映射为单席 Note，对 flow/architecture/call_dag 映射为 standalone fact node；所有载体均无 edge/anchor，未知 family/relation fail-closed。
copy-ready receipt 只把实际渲染载体计入 annotation count/kinds，无法表达的 typed relation 改列 `visual_omitted_relation_kinds`，消除零计数却声称已保留的自冲突。

optional AxisFlow repair 在 whole story skeleton 被权限边界扣留时，现重复 bounded exact relation/unary recipes，并声明只是 relation boundary；模型自行选择 faithful subset、
业务措辞与布局，图仍可在无价值时删除。无系统补边/绘图/结论代写或 raw prose hard gate。

状态：`B554/B556/B557=implemented/pending-r328`；`B555=pending-r328`；`B558=watch`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.153 r328：关系骨架恢复闭正证；正文跨分量与 4 分钟活跃流残余

r328 exact-two runner 2/2、人工 `fail + pass-with-caveat`。C++ 的 copy-ready sequence 已完整携带 call、return Note 与单实体 guard Note，annotation receipt
与实际载体一致；Python 一次 patch 后保住两条 exact edge 与 registration Note，验证 B554-B557 的正向生产路径，B558 多轮风暴未复现。

仍确认两项系统 gap。`B559/P1-high`：diagram edge 有 typed authority，但 ordered-list 只有 block-level claim annotation，模型可把多个 disconnected relation component
写成一条连续路径。当前没有足够精确的 item→typed-edge/component 载体，故禁止用原文关键词、标签相似度或本 case 名称做 hard gate；后续先补 item-level authority carrier，
再做 component-topology 校验。`B560/P0`：流式 no-visible watchdog 虽已移除，`2×request_timeout` 绝对帽仍会杀死持续 reasoning/content/tool progress 的活跃连接，
120s 配置下即约 4 分钟。总帽只应约束无 usable model progress 的 transport-only 流；已有真实模型进展时不得因累计时长降级，更不得系统代答。

状态：`B554-B557=production-positive`；`B558=close-to-watch`；`B559=carrier-first/open`；`B560=P0-next`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.154 B560：4 分钟活跃流不再被累计时长降级

OpenAI-compatible SSE adapter 现把绝对总帽严格限定在“只有传输存活、尚无真实模型进展”的状态。reasoning、assistant content、tool-call delta、finish 或 usage
任一出现即关闭该帽；累计耗时不再能终止健康活跃流或授权系统替换模型答案。纯 keep-alive/空帧/畸形心跳仍无 model-progress 权限，继续由
`2×request_timeout` 有界 fail-loud；stall、精确重复退化和调用方取消均保持原强度。

三类活跃正臂跨过旧帽后完成，keep-alive-only 负臂继续失败；`llm/agent/orchestrator` 通过。无 prompt/prose 扫描、无 finalizer 或 Trace 行为变化。

状态：`B560=implemented`；`fixed-four-minute-active-degrade=retired`；`transport-only-cap=retained`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.155 r329：部分验证通过被终态文案抹零，Trace 机理口径再次越权

r329 exact-two runner 1/2、人工 0/2。Trace 的请求窗、自动补采、链上根因/可消量、确定性语义优化、调度供给、背景隔离和因果投影均未回归；模型仍把
`pre_wakeup_dependency` 的重叠候选解释成“等待上游工作完成”，而该 span 在 wake 后仍延续 0.400ms，typed context 也明确 direct blocking 未证。
该复现归 `B544=P1-soft`，只增强 typed 机理/端点上下文或未来 answer-side structured carrier，不以模型/答案原文作硬门。

write 案确认新的 `B561/P1-high`：报告内已有 `make-test passed=true`，但 Java runner_missing 使总体 unavailable 后，controller/final 两面均只显示不可用原因，
最终谎称“没有断言验证过”。正确口径应是“局部源码检查已通过；Java 行为验证不可用，故仍未完全验证”。判定与文案必须共同从 typed report 派生，
保留 partial evidence 但禁止由此升级 verified。

状态：`B544=recurrent/P1-soft`；`B561=next`；`partial-pass-evidence=currently-hidden`；
`model-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.156 B561：unavailable 不再抹掉已通过的局部检查

controller prompt 新增 typed verification evidence 计数与非 passed 边界，避免只投影 failure summary 后诱导模型声称所有检查均未执行。用户 unverified 卡片同步按
`TestResults` 分臂：有 passed row 时披露“局部检查通过 + 必要验证不可用”，无 passed row 时才保留“没有断言验证”的旧形。整体 unavailable 判定、行为证明要求与
merge 风险均未放松，源码检查不会被升级成目标语言运行时证明。

相关 agent/orchestrator 套件通过；无 prose 扫描、无系统代写模型答案、无 Trace 改动。

状态：`B561=implemented`；`partial-evidence=retained`；`full-verification=no-promotion`；
`model-context=typed-counts`；Trace causal authority=`unchanged`。

#### §11.10.157 r330：B560/B561 生产正证；participant 修补协议仍未把“证据端点”和“业务身份”拆成可执行动作

r330 exact-two runner 0/2、人工 `pass + fail`。Gson write 案确认 B561 正证：`make check` 的一项源码检查仍可见，Java runtime 缺席只使完整行为验证保持
unverified，终态不再谎称“没有断言验证过”。QF 读案持续 743s 且 reasoning/tool/model content 一直有进展，没有被旧 4 分钟累计帽终止，B560 获生产正证；
最终降级来自 19 次成文合同拒绝耗尽，而非流式时长。

QF 深审否证了“同一条关系在代码里绝对同时必带、必拒”的初判。合同存在可满足形：canonical operation/stage endpoint 留在
`edge_anchors.from_identity/to_identity`，模型把用户点名的 participant 作为该 endpoint 的可见业务标签或 Mermaid group；直接 stage identity 节点上的
checkout-verified precedence 也可通过。生产失败的真实根因是 `B562-PARTICIPANTREPAIRACTION1/P1-high`：一个长 repair 段同时解释
`available_typed_incident_edge_not_rendered`、`required_participant_identity_not_visible`、stale/missing boundary，却只给 canonical candidate 文本，没有把
“保留哪条 exact edge”“anchor 必须复制哪些 identity 字段”“只补业务可见身份、不得把技术 endpoint 改成 broad participant”“该状态是否需要 boundary”拆成
逐席、逐状态、可执行的 typed repair action。模型因此在 exact endpoint 与 `Mutable/BusContext` 宽身份之间反复重定向，并在“画 candidate edge”与“标
unproven”之间振荡；关系门本身正确地拒绝了这些被重定向的伪边。

最优修法不是放宽 relation/participant hard gate，也不是系统替模型画图：candidate capsule 增加与 schema 同构的 exact anchor fields；repair 按 typed mismatch
枚举分流——edge missing 只要求模型从现有候选选一条，identity missing 要求保留已证边与 canonical endpoints、仅补可见 label/group，stale boundary 只删
boundary，真正无 candidate 才允许 visible disconnected node + unproven boundary。候选仍是选择集而非全部必画，不读取 request/model/final prose。

状态：`B560/B561=production-positive`；`B562=next`；`contract=in-principle-satisfiable`；
`system-edge/answer-authorship=none`；`fixed-four-minute-active-degrade=retired`；Trace causal authority=`unchanged`。

#### §11.10.158 B562：participant repair 按 typed 状态分流并携带可复制的 canonical anchor

participant coverage reject 现为每个失败 participant 发布独立 `repair_action`。`edge missing` 只要求从该席现有 typed candidate 中选择一条并保持方向；
`identity missing` 明令保留已证边/canonical endpoint，仅补业务可见 label/group；`stale boundary` 只删除 boundary；只有无 typed candidate 时才允许 exact
disconnected node + unproven boundary。不同状态不再共用一段需要模型自行反推的互斥教学。

每条 candidate 同步携带与 AnswerDocument schema 同构的 `edge_anchor_identity_fields={from_identity,to_identity,relation_kind}`。`from_node/to_node`、业务标签、
选哪条候选、布局与最终解释仍由模型决定；系统不画边、不把 broad participant 写回 technical endpoint，也不把候选全集升级成必画全集。判据继续只读 typed
participant slate、citable operation 与 checkout-verified stage precedence，不扫描请求/模型/终稿 prose。

定向 participant tests 及 `internal/tool`、`internal/agent`、`internal/orchestrator`、`internal/tracediag` 全绿。Trace family 仍在 coverage 入口显式排除，显式窗、
因果投影、自动补齐、链上根因与双轴归因均未改；活动流累计时长策略未改。

状态：`B562=implemented/pending-r331`；`repair=typed-state-specific`；`canonical-anchor=copyable`；
`model-visible-diagram-authorship=preserved`；Trace causal authority=`unchanged`。

#### §11.10.159 r331：participant repair 已生效，显示换行却截断 exact endpoint；Trace 机理越权再现

r331 exact-two runner 2/2、人工 0/2。QF 的逐席 `repair_action` 与可复制 canonical anchor 已进入每轮反馈，证明 B562 接线生效；但模型把
`explorerEvaluator.ParseOutput`、`ctx.Mutable.SetTurnAArtifacts` 等技术身份按成员分隔符换行后，relation resolver 只取第一行 owner，已有同向 typed call
仍被判 `call_edge_unproven`。14 次成文拒绝、1132s 后虽交付，最终图仍由 stage 主链、两段 Mutable 方法调用和 BusContext stage 写入三个断片组成，未形成
共享状态读写的完整逻辑视图。立 `B563-DISPLAYWRAPIDENTITY1/P1-high`：只允许以语言成员分隔符开头的续行还原候选，且候选必须唯一命中 citable typed
endpoint；任意多行业务标签继续 fail-closed。系统不得据换行文本自铸关系。

Trace 案保留显式 5.000000..5.007000 窗、自动补采、因果投影、链上 class-verification/runnable 双席、实际占用/规则可消双轴与背景隔离；但 typed
authority 明示 `target_direct_blocking_authority=not_provided`、`direct_blocking_decision=not_established`，模型仍称目标“等待 VerifyClass 完成/被其阻塞唤醒”。
该 span 在 wake 后还延续 0.400ms，故只是 `pre_wakeup_dependency` 席的再次 B544 生产见证，不能升级为完成式直接阻塞。后续仍走 typed 上下文降心智/软教学，
禁止扫描答案原文做 hard gate 或由系统改写结论。

活动流方面，QF 连续 1132s 有 reasoning/tool/content 进展且没有因 4 分钟累计时长降级，B560 获长时生产正证。只有 transport-only、真实停滞、断链、
调用方取消或独立安全边界可触发失败恢复；恢复只能发布模型已有草稿并披露，不能系统代答。

状态：`B562=production-partial`；`B563=next`；`B544=recurrent/P1-soft`；`runner=2/2`；`human=0/2`；
`active-stream=no-degrade@1132s`；`system-answer/edge-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.160 B563：成员分隔符换行不再破坏 typed endpoint identity

Mermaid label identity resolver 现额外构造“显示换行还原候选”：仅当后一行以 `.`、`::`、`->`、`#` 或 `/` 这类跨语言成员分隔符开头，且拼接结果本身是
合法 code identity 时才进入候选集。候选仍必须唯一命中 citable typed evidence 的 subject/object/owner/anchor；换行、位置或相似度本身没有任何关系权限。
因此 Java/Go/ArkTS/Cangjie/Rust/C/C++ 等语言的 method-qualified endpoint 可为图宽换行而不被截成 owner，同时普通
`业务名称\\n阶段说明`、无分隔符多行、路径后缀和多身份歧义继续 fail-closed。

端到端关系测试覆盖 `.`、`::`、`->` 三种代表语法及任意多行不得拼接的负臂；`internal/tool`、`internal/agent`、`internal/orchestrator`、
`internal/tracediag` 全绿。修复只影响 source diagram 的精确身份解析，不读取用户/模型/终稿 prose，不补 edge、不改 relation kind/方向；runtime Trace family、
显式时间窗、自动补齐和因果投影均未改。

状态：`B563=implemented/pending-r332`；`display-wrap=typed-candidate-only`；`ambiguity=fail-closed`；
`system-edge/answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.161 B544：唤醒前链上工作不再被教学成“可能阻塞”

`pre_wakeup_dependency` 的模型上下文已收敛为一条明确的 typed 机理上限：它只表示上游链上工作与下游目标唤醒前区间重叠，是可排序的工作候选；该席自身不证明
目标在等待这项工作、不证明等到工作完成，也不证明直接阻塞。只有另行存在 typed holder/waiter 或 blocking relation 时才允许使用对应强机理；目标唤醒后的
runnable/dispatch delay 仍只能来自目标自身 runnable 区间与同核调度顺序。

同一语义由 `traceDecisionWritePhase` 单源写入完整 decision handoff 和最终 compact authority，携带
`mechanism_ceiling=on_chain_prewakeup_work_candidate_only`、`target_wait_for_work_authority=not_provided_by_this_seat`、
`work_completion_dependency_authority=not_provided_by_this_seat` 与 `direct_blocking_authority=not_provided_by_this_seat`。若该节点自身已有 typed
`BlockingKind`，这些否定字段不发，避免软教学抹掉更强精确信号；`post_wakeup_delay_authority` 仍保持未提供。

该批只改模型所见 typed context，不扫描请求/推理/终稿、不拒绝或改写模型答案、不改变排序、可消量、投影树或任何 Trace 事实。agent/orchestrator/tracediag
套件全绿。

状态：`B544=context-fix-implemented/pending-replay`；`typed-blocking=preserved`；`raw-prose-hard-gate=none`；
`system-answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.162 r332：B563 生产正证；unproven participant 仍可挂在已证技术边上

r332 exact-two runner 2/2、人工 1/2。QF 用时由 r331 的 1132s 降至 499s，finalizer reject 由 14 降至 3；所有 member-qualified label 换行均未再出现
`call_edge_unproven`，B563 获生产正证。C++ virtual-chain 在一次修补后完整保留 Logger→Sink→ConsoleSink→fputs/fputc、虚分发以及
SinkRegistry::create 的 console/file/rotating 三分支，`::` 身份和 mixed-relation Note 无回归。

QF 最终图仍确认 `B564-UNPROVENINCIDENT1/P1-high`：model-authored edge anchor 诚实指向
`appendStageOutputEvidenceToMutable -> MutableState.AppendEvidence`，可见 sink label 却写成宽身份 `Mutable`；同一 diagram 又保留
`participant_boundaries[{participant:Mutable,status:unproven}]` 并披露关系未证。现实现对无 candidate 的 boundary 只验证 exact node 可见，不验证该节点是否真无 incident
edge，导致“已连接”和“未证断开”同时通过。最优修法是精确 Mermaid 结构门：无 typed candidate + unproven boundary 时，该 exact participant node 必须零入边零出边；
已有 valid typed edge 必须保留在其 exact technical endpoint 上，宽 participant 另作断开节点。系统不删边、不改标签、不选择关系，只返回分离动作。

另立 `B565-PARTICIPANTBINDINGACQ1/P1`：探索已读到 `Mutable *MutableState`，但未发 field→declared-type typed binding；因此现有
`AnswerCodeIdentityIncidentViaDeclaredBinding` 无证据把用户点名 field identity 与 `MutableState.AppendEvidence` operation 接合。先补通用 declared-binding acquisition
教学/载体，再决定是否可安全去掉 unproven boundary；禁止从命名相似或答案文本猜 alias。

状态：`B563=production-positive`；`B564=next-highest-ROI`；`B565=open`；`runner=2/2`；`human=1/2`；
`system-edge/answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.163 B564：unproven participant 边界落实为文档级零 incident edge

participant coverage 新增精确 issue `unproven_boundary_has_visible_incident_edge`。当某个 incident-required participant 没有 typed candidate、模型选择
`status=unproven` 时，其 exact business identity 在整份文档所有 Mermaid 图中必须零入边、零出边；不再只检查“有一个同名节点”便放行。检查输入仅为 analyzer 的 typed
participant slate、结构化 boundary、Mermaid AST 节点/边与 typed relation evidence，不读请求或终稿 prose。

若已有合法 technical edge，只返回分离动作：把原边保持在 exact technical endpoint 上，另保留 exact business participant 断开节点和唯一 unproven boundary。
系统不删边、不换标签、不挑关系、不为两者猜 alias。单图同节点矛盾、跨 sibling diagram 矛盾、技术边与断开业务节点诚实共存三臂均有 pin；既有 disconnected
boundary 正臂保持通过。

状态：`B564=implemented/pending-r333`；`unproven=zero-incident-document-wide`；`typed-edge=preserved-on-exact-endpoint`；
`B565=open`；`system-edge/answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.164 r333：B564 未被本轮 typed 清单触发；B544 机理上限送达但仍被模型忽略

r333 exact-two runner 2/2、人工 0/2。QF 最终图只保留 Analyzer→Explorer→Extractor→Finalizer，Orchestrator、BusContext、MutableState、
TaskState 均为断开节点，请求要求的共享状态/数据流没有进入图。深审确认本轮 analyzer 发出的 `diagram_hint.participants=[]`，虽然同一次 typed analysis 的
relation scope 与 exact targets 已包含这些实体；因此 B564 没有 participant obligation 可执行，不能把本轮失败误记为 B564 回归。首次草稿中的多条无证数据流边被
关系门正确拒绝，模型修补时删除关系后通过。立 `B566-ANALYZERPARTICIPANTSLATE1/P2-watch`：这是 analyzer structured output 的跨字段不一致，r331/r332
又曾正确发出清单，先按模型波动观察；禁止扫描 relation quote、请求或答案 prose 后硬铸 participant，未来若稳定复现，只能从更上游 schema/typed planning
一致性降低模型心智。

Trace 案保留显式 5.000000..5.007000 窗、自动补采、因果投影、链上根因、背景隔离与实际占用/规则可消双轴。B544 新增的
`mechanism_ceiling=on_chain_prewakeup_work_candidate_only` 及 target wait/completion/direct blocking 三项 `not_provided_by_this_seat` 已逐字进入 finalizer
上下文，但模型仍称目标“被迫等待 VerifyClass 完成”，连续 r329/r331/r333 均复现，已不能归为单次波动。下一步只在最终选中席位确属
`pre_wakeup_dependency` 且没有 typed blocking relation 时，于模型落笔前的最后 synthesis scope 发一条简短自然语言机理上限；若另有精确 blocker 则不发。
该提示只读 typed 席位，不扫描/拒绝/改写模型 prose，不改变 Trace 排序、可消量、投影或模型结论所有权。

活动模型流方面，r333 未触发任何固定累计时长降级。B560 不变量继续生效：reasoning/content/tool-call/finish/usage 任一真实模型进展存在时，即使超过约 4 分钟也
不能按累计时长终止或降级回答；只有 transport-only、真实停滞、断链、调用方取消或独立安全边界可进入失败恢复，恢复也只能发布模型已有草稿并明确披露，系统不得代答。

状态：`B564=implemented/not-exercised-r333`；`B566=P2-watch/model-variance`；`B544=recurrent/P1-next`；
`runner=2/2`；`human=0/2`；`fixed-four-minute-active-degrade=retired`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.165 B544：最终选中唤醒前席位获得简短 typed 机理边界

final synthesis tail 现只对“已发布 fix-direction leader + `pre_wakeup_dependency` + 席自身无 `BlockingKind` + 目标无精确 typed waiter/holder”这一合取条件，
追加一条简短自然语言边界：该席只能表述为目标唤醒前的链上重叠工作候选，不能据此声称目标等待该工作、等待其完成或被其直接阻塞。判定只读投影席位、目标状态账户与
typed blocking relation；不扫描请求、模型推理或终稿，不拒绝、删除或替换模型答案，也不选择根因。

若投影存在精确 target waiter/holder relation，该负边界整席不发，由既有 compact authority 发布更强阻塞凭证，避免软教学抹掉精确信号。提示只覆盖每个投影最多三个
已发布方向 leader，并紧邻最终 synthesis scope，减少长上下文中重复字段对模型心智的消耗。无 Trace 排序、可消量、窗口、补采、投影树或答案渲染改动。

状态：`B544=implemented/pending-r334`；`trigger=typed-final-seat-only`；`exact-blocker=preserved`；
`raw-prose-hard-gate=none`；`system-answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.166 r334：B544 获生产正证；inline probe runtime enum 可被命令 wrapper 绕过

r334 exact-two runner 2/2、人工 1/2。Trace 正文保留显式 5.000..5.007s 窗、确定性补采、因果投影、链上/背景隔离及实际占用/规则可消双轴；模型明确将
worker-200 `class_verification` 限定为目标唤醒前链上工作候选，分别披露目标 sleep 原因、直接 blocker、holder/waiter 和帧因果未证，不再声称 app-100
等待该工作完成或被其直接阻塞。0.800ms wake 后 runnable_wait 独立归为调度供给，强 typed blocker 车道未被负边界抹除。B544 关闭到生产正证。

C++ write-plan 的单行 patch 正确且可 apply，但确认新的 `B567-PROBEWRAPPEREXEC1/P1-high`：planner 在 `language=python` 的 probe 中调用
`subprocess.run(['g++', ...])`，绕过 inline runtime enum 去执行 C++ 编译器；change-plan 教学和 schema 描述均已明确禁止，`emit_change_plan` 却接受并持久化。
现有 coupling validator 只在 changed source 与 probe runtime 同族时检查 import/require；纯 C++ change 对 Python provider 产生空 target，故错误放行。

最优方案不是扫描请求、计划摘要或某个编译器名，也不是列举 `g++/rustc/cjc` 关键词。应在 plan-time 从结构化 `changes[].path` 扩展名派生 changed-source
language family，再与 `verification_probes[].language` 的 runtime family 求交：存在已识别 source target、但某 probe 与全部 changed-source family 不相容时，拒绝该 probe，
要求删除它并把 native build/test 命令留在 acceptance tests；多语言计划只要至少有一个同族 changed target，继续交由既有 import/require coupling 验证其实际绑定。
无已识别 source target 的配置/文档计划保持旧行为。该门只读 typed plan fields，不读取任何模型 prose，覆盖 C/C++/Rust/Cangjie/ArkTS 及未来扩展语言。

状态：`B544=production-positive`；`B567=P1-high/next`；`runner=2/2`；`human=1/2`；
`probe-command-wrapper=currently-accepted`；`fixed-four-minute-active-degrade=retired`；
`system-answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.167 B567：inline probe runtime 与 changed-source family 在 plan-time 对齐

`emit_change_plan` / 分段 plan 的共享 full-content validator 现新增 typed runtime-target compatibility：从 `changes[].path/new_path` 的精确扩展名取得已识别 source family，
从 schema-normalized `verification_probes[].language` 取得 inline runtime family；若某个 probe 与本计划全部 changed source family 均无交集，直接拒绝并要求删除 probe、
把 native build/test 命令留在 acceptance tests。C/C++/Rust/ArkTS/Cangjie 被 Python wrapper 绕过的五臂已钉；Python、JS、TS、Ruby、Java、Go 同族正臂、
Python+C++ 多语言计划及 config-only fail-open 臂均保留。

该门不读取 probe code、请求、summary、rationale 或模型输出，不枚举 `g++/rustc/cjc/hvigor` 等命令词，因此不是 case-by-case 关键词封堵；它只约束结构化执行权限。
多语言计划中只要 probe 与至少一个 changed source target 同族，仍进入既有 import/require、changed-symbol、实际执行和 changed-path coverage 校验，不能仅凭 family 匹配
升级为验证证明。JavaScript 保留现有 JS/TS provider 兼容；未知或纯配置/文档目标不被路径后缀猜成行为语义，维持 fail-open。

状态：`B567=implemented/pending-r335`；`gate=typed-path-plus-runtime-enum`；`probe-code-scan=none`；
`native-project-verification=preserved`；`system-answer-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.168 r335：类型关系名册与图校验断桥，真实 implements 边被迫全部删除

r335 exact-two runner 2/2、人工 1/2。C++ 单行 patch 的范围和 acceptance tests 正确，planner 本轮没有提交 inline verification probe；因此 B567
保持实现验证通过，但本轮属于 `not-exercised`，不能冒充 wrapper 拒绝臂的生产正证。

类型关系案确认 `B568-TYPERELATIONHANDOFF1/P1-high`，runner 的正则 oracle 假绿。analyzer 以 typed `predicate_axis=implement` 和完整
`LoopController` implementer 名册驱动探索；最终上下文也正确投影出 12 个 production principal rows、3 个 test auxiliary rows。然而 diagram contract
把该请求表示为 `call_dag`，而 `type_relation` 硬门只从 citable `EvidenceItem` 读取精确方向关系，没有消费产生上述名册的同一份 typed relation hint。
本轮探索通过 `repo_map` 与 grounded definition rows 完成，未形成 deterministic implementer EvidenceItem 桥，结果系统一边要求关系图，一边把模型先后绘制的
`LoopController -> implementer` 和修正后的 `implementer -> LoopController` 12 条边全部判为 `type_relation_edge_unproven`。模型随后尝试改成 call，又被正确的
call authority 拒绝，最终只能删除全部边；407s、5 次 finalizer reject 后交付的是节点集合而非类型关系图。

这不是 Mermaid 渲染失败，也不是单次模型波动，而是两个 typed 消费面的精确断桥。最优修法是把已经由 relation provider 铸造、带 source/file/line/role 的
implements/extends/embedding typed rows，在 pre-emit 前以同一关系方向投影为 citable relation evidence，供既有 `type_relation` gate 使用；模型仍决定选择哪些
principal 边、节点业务标签、布局和解释。禁止从 edge label、请求、reasoning、终稿 prose 猜 `implements`，禁止把 type relation 降格成 call，也禁止系统代画图。
该桥必须复用跨语言 typed relation carrier，统一覆盖 Go 隐式接口、Java/Kotlin/ArkTS/Cangjie/C++ 显式继承/实现、Rust trait 等已支持图关系，而非按本 case
或语言加关键词。test/production role 仍只决定 principal/auxiliary 展示，不改变关系真值。

活动模型流连续 407s 正常完成，没有按累计 4 分钟降级或由系统替换答案。失败恢复披露来自成文重试耗尽，不是流式时长；`B560` 不变量继续保持。

状态：`B567=implemented/not-exercised-r335`；`B568=P1-high/next`；`runner=2/2`；`human=1/2`；
`typed-roster=present`；`type-relation-evidence-bridge=missing`；`system-edge/answer-authorship=none`；
`fixed-four-minute-active-degrade=retired`；Trace causal authority=`unchanged`。

#### §11.10.169 B568：类型关系图与名册统一消费 exact provider

diagram pre-emit 现仅在模型明确提交 `relation_kind=type_relation` 时，重新查询与 member-set coverage 共用的
`TypedRelationPurposeCoverageGate` provider；只有 `exact_symbol_id` / `exact_file` / `exact_evidence` 候选可投影为本次校验的 citable
type-relation evidence。prompt-only `TypedRelationHints` 继续只是教学输入，`name_only`/heuristic 候选不能借此获得 hard-gate 权限。

implements 与 extends/继承均保持 provider 的精确方向 `implementer/subtype -> interface/base`。反向边继续拒绝，call 重标继续拒绝；模型没有提交
type-relation anchor 时不查询、不投影，更不会由系统添加可见边、选择 principal member、生成布局或修改解释。source role 只决定 principal/auxiliary 展示，
不撤销已由 parser/graph 确认的关系真值，所以 test/support 图中的真实边仍可被校验，但不会被自动提升为 production 主表。

实现完全复用跨语言 typed relation carrier，不检查文件正文、请求、reasoning、终稿或 edge label。测试钉住：正向 exact implementer 通过、反向失败、name-only
失败、无 model-authored anchor 零投影，以及 Go/Java/Kotlin/ArkTS/Cangjie/C++/Rust 文件面走同一 provider 协议。`internal/tool` 全包及
`internal/agent`、`internal/orchestrator`、`internal/tracediag` 全绿。

状态：`B568=implemented/pending-r336`；`provider=coverage-gate-exact-only`；`prompt-hints=not-promoted`；
`model-edge/answer-authorship=preserved`；`reverse/relabel=fail-closed`；Trace causal authority=`unchanged`。

#### §11.10.170 r336：B568 精确桥生效，但 classDiagram 关系可绕过统一证据门

r336 exact-two runner 2/2、人工 1/2。Trace 案 144s 一次成文：显式 5.000..5.007s 窗、确定性 `frame_root_cause_bundle`
补采、Trace 因果投影、实际占用/规则可消双轴全部保留；4.600ms `class_verification` 只作为链上唤醒前工作候选，0.800ms
`runnable_wait` 独立归为调度供给，背景没有被加冕，帧因果与直接 blocker/holder/waiter 继续诚实标为未证。B568 未影响 Trace 权威面。

类型关系案证明 B568 的 provider 桥已经工作：模型第一版把 `LoopController -> implementer` 画反后，12 条边均被精确的
`type_relation_edge_unproven` 拒绝；这不是旧版“精确关系也不可用”。但随后暴露 `B569-CLASSRELATIONEDGE1/P1-high`：模型改用合法
Mermaid `classDiagram`，以 `LoopController <|.. plannerEvaluator` 表示实现关系，并提交 canonical `implementer -> LoopController`
anchors；公共 `mermaidcompat.ParseEdges` 只识别 flowchart/sequence 运算符，看不到 `<|..`，于是所有 anchors 被误报为
`typed_anchor_without_visible_edge`。模型删除 anchors 后，同一 12 条可见类关系反而通过最终校验。最终答案视觉上有完整关系，
但 typed evidence contract 被 class syntax 绕过，runner 的字符串 oracle 再次假绿。

最优修复是扩展共享 Mermaid syntax parser，而不是加 LoopController、implements 或语言关键词：识别 classDiagram 的
`<|--`、`<|..`、`--|>`、`..|>`，按 UML 箭头头部把端点统一为语义方向 `subtype/implementer -> base/interface`，随后复用现有
body-edge/anchor/evidence gate。解析只产生 syntax-level endpoint/operator，不猜 `relation_kind`，不改模型图、不补边、不代写答案；
relation_kind 仍须模型通过 schema 明确提交，B568 exact provider 仍负责真值授权。该修复天然覆盖 Go、Java、Kotlin、ArkTS、Cangjie、
C++、Rust 等 provider 已支持语言。

本轮另记 `B570-FIRSTDRAFTDOMINANCE1/P2`：最终类图接受后，答案仍附带整份失败的无边第一稿，造成主体重复和内部恢复语句外露。
这是“发生过真实 rewrite 就保留第一稿”的既有审计设计，不是 4 分钟超时降级；活动流持续 447s 正常完成。后续若抑制，必须基于
两份 structured carrier 的 typed dominance，不能扫描或相似匹配模型 prose。当前优先通过 B569 消除制造该附录的根因，B570 单独保留。

状态：`B568=production-positive/blocked-by-B569`；`B569=P1-high/next`；`B570=P2/open`；
`runner=2/2`；`human=1/2`；`active-stream-447s=no-degrade`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.171 B569：classDiagram 有向关系进入共享 syntax/typed-authority 管线

共享 `mermaidcompat.ParseEdges` 现识别 Mermaid classDiagram 的 12 种有向关系写法：继承 `<|--`/`--|>`、实现
`<|..`/`..|>`、关联 `<--`/`-->`、依赖 `<..`/`..>`、组合 `*--`/`--*`、聚合 `o--`/`--o`。同一语义的左右箭头写法
统一为 canonical endpoint direction；继承/实现为 child/implementer -> base/interface，组合/聚合为 whole -> part。类图 cardinality、
namespace-qualified endpoint 与关系 label 均保留，quoted node label 中的箭头字节不产生伪边。无方向的 `--`/`..` 不被铸成有向事实。

该层只解析模型已经画出的语法拓扑与 operator，不从 arrow、label、请求、reasoning 或答案 prose 猜 relation_kind。模型仍须在
`edge_anchors[]` 明确提交 schema-valid relation kind，随后由既有 call/type/guard/import/contain 等 typed evidence gate 决定真值；系统不改
Mermaid bytes、不补边、不选成员、不代写结论。因而 B568 的 exact provider 可直接授权合法 `Base <|.. Impl` 的 canonical
`Impl -> Base` anchor，删锚现在报 missing relation owner，反向锚同时报 stale metadata，不能再把同一可见类图降格成免检 presentation。

回归覆盖：12 operator matrix、cardinality/qualified endpoint、quoted literal、undirected negative；tool 层钉住 exact provider + classDiagram
端到端正臂、删锚负臂、反向负臂。`internal/mermaidcompat`、`internal/tool`、`internal/agent`、`internal/orchestrator`、`internal/render`、
`internal/preview`、`internal/tracediag` 全包通过。flowchart/sequence 与 runtime Trace 独立 authority 未改。

状态：`B568=implemented`；`B569=implemented/pending-r337`；`B570=P2/open`；
`class-directed-operators=12`；`raw-prose-hard-gate=none`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.172 r337：classDiagram 主校验通过，后置校验漏接同一 exact provider

r337 exact-two runner 2/2、人工 1/2。类型关系案证明 B569 已生效：`LoopController <|.. analyzerEvaluator` 等 12 条
classDiagram realization 均被共享 parser 识别为 canonical `implementer -> LoopController`，模型提交的 `type_relation` anchors 与可见图方向一致，
pre-emit 也通过 B568 exact provider bridge 正确授权。

但 orchestrator 的 post-finalizer 关系检查重新调用 `DiagramCallEdgeEvidenceMismatchesWithRuntimeContext` 时，只传入原始 evidence slice，
没有运行同一 `preEmitEvidenceWithExactTypedDiagramRelations`。于是同一份结构化答案在前置层通过、在后置层被误报为 12 条
`type_relation_edge_unproven`，触发无效 finalizer 修补；恢复层最后保留了正确第一稿，却附加“检查仍需补充验证”的系统降级说明。
立 `B571-POSTTYPERELATIONBRIDGE1/P1-high`：post-finalizer 必须先消费与 pre-emit 完全相同的 exact provider projection；仍仅在模型已经提交
`relation_kind=type_relation` 时启用，name-only/heuristic 候选继续无 hard-gate 权限，系统不画边、不改方向、不代写答案。

时序案最终人工通过：答案正确说明 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith` 是共享汇点而非
`buildAnalysisIR -> gate.Run` 调用链，图中方向和 typed anchors 均正确。但 827s、18 次中循环暴露 `B572-SEQUENCEPROOFCHURN1/P1`：
`gate.Run` definition 已精确发射后仍反复出现 endpoint existence 缺失诊断，随后又要求模型手工转交两条 already-read parser call relation。
该项先拆分 endpoint ambiguity 与 parser handoff 两种精确信号，不以放松关系证据门或系统自动选择图边换速度。

本批两条模型流分别持续 271s、827s，均有真实 reasoning/tool/content 进展并由原模型正常产出。固定累计四分钟不得成为答案降级、草稿替换或
系统代答条件；只有 transport 断链、真实停滞、调用方取消或独立安全边界可进入既有恢复，恢复也只能发布模型已有载体并明确披露。

状态：`B569=production-positive`；`B571=P1-high/next`；`B572=P1/open`；`B570=P2/open`；
`runner=2/2`；`human=1/2`；`active-stream-827s=no-degrade`；`fixed-four-minute-active-degrade=forbidden`；
`system-answer/edge-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.173 B571：前置与后置图关系校验共享 exact typed provider

`DiagramCallEdgeEvidenceMismatchesWithRuntimeContext` 现与 pre-emit 走同一个
`preEmitEvidenceWithExactTypedDiagramRelations` 投影。没有 report-local runtime temporal block 时在原文档上投影；存在 runtime-owned block 时，先排除该独立
Trace authority 面，再只对剩余 source diagram 投影，避免把两类关系权限混在一起。

投影触发和 B568 保持字节同源：只有模型已提交 `relation_kind=type_relation` 才查询 provider，且仅
`CoverageGateEligible()` 的 exact symbol/file/evidence 候选可进入本次校验证据。正向 classDiagram realization 在 pre/post 两层均通过；反向 anchor 仍被
可见拓扑/锚一致性门拒绝，name-only 候选仍报 `type_relation_edge_unproven`。系统不添加可见关系、不改 Mermaid、不决定成员、方向、布局或答案措辞。

新增 post-finalizer 集成 pin 同时覆盖 exact 正臂、reverse 负臂、name-only 负臂。`internal/tool` 167.947s 全包通过，
`internal/agent`、`internal/orchestrator`、`internal/render`、`internal/preview`、`internal/tracediag` 全绿。

状态：`B571=implemented/pending-r338`；`pre-post-provider=single-source`；`runtime-trace-authority=separate`；
`model-edge/answer-authorship=preserved`；`reverse/name-only=fail-closed`；`B572=P1-next`；Trace causal authority=`unchanged`。

#### §11.10.174 B572 冷读裁定：角色说明伪装声明；已读 parser 真值仍要求模型抄送

r337 的 827s 不是单一慢模型问题，代码与逐轮日志确认两个独立的 typed 管线 gap。

`B572a-ENDPOINTROLEPAIR1/P1`：模型首次在 `gate.go:134` 发射 `anchor_kind=definition, anchor_symbol=Run` 后，工具将其判为既有重复；同一证据池还含
系统自动生成的 `auto_pair_role_description`，它位于定义前 doc comment 的 126 行，却同样携带 `AnchorDefinition/Run`。端点存在性在用文件作用域把 bare `Run`
限定为 `gate.Run` 时，把 126、134 两行计成两个声明位置，因而报 ambiguous。第二次模型显式补 `subject=gate.Run` 后 exact identity 优先命中才通过。
这与该 producer 自身的类型注释“role/WHAT context，不是第二个 declaration identity”直接矛盾。根修是在 endpoint-existence authority 中排除该精确 producer；
它仍保留为角色解释与 citation context，但不能证明符号存在或制造声明歧义。不得按 `gate.Run`、Go 或注释文本做特判。

`B572b-PARSERHANDOFFCOPY1/P1`：`callChainReadParserRelationHandoffDowngrade` 已严格收窄到 AST/Cangjie parser provenance、exact source+line 已读、caller/callee
唯一落入模型已提交 principal member-set（或完整性义务下同 statement sibling）才触发；这些信号已经足以作为精确当前源码事实。继续要求模型逐条重发
`emit_evidence` 只增加循环和漏抄风险，并不保护答案所有权。最优方案是沿用 deterministic evidence provider 形，将这些 exact rows 投影成 citable
`ClaimCallEdge`，带独立 producer、source/line/caller/callee，并进入 finalizer 可见 evidence；模型仍决定是否把某条边画进图、如何组织说明和最终结论。
系统不得从请求、模型 prose、终稿或名字相似度补边；unread、regex fallback、端点歧义、非 principal relation、runtime Trace 均继续不投影。

施工分两批：B572a 先修 endpoint identity，pin 自动角色说明 + 真实声明共存仍唯一；B572b 再把既有 gap selector 改成 deterministic evidence projection，
钉住 Rust/Cangjie、unread/regex/ambiguity/runtime 负臂和“系统只供事实、不写 diagram/answer”。完成后以同一 sequence case 加类型关系案做 r338 exact-two。

状态：`B572a=P1/next`；`B572b=P1/frozen-after-a`；`typed-selector=existing`；`raw-prose-scan=none`；
`system-answer/diagram-authorship=none`；`active-stream-fixed-time-degrade=forbidden`；Trace causal authority=`unchanged`。

#### §11.10.175 B572a：角色说明不再参与端点声明存在性

`AnalyzeCallChainEndpointExistence` 的 exact identity map 与 source-scoped bare-definition fallback 现都显式排除
`EvidenceProducerAutoPairRoleDescription`。该 producer 的行仍是 grounded/citable 角色上下文，仍可告诉模型定义“做什么”；但它不再证明符号声明存在，
也不会和真实定义行一起把唯一端点制造成 ambiguous。

回归按 r337 真实形钉住：`gate.go:134` 的 bare `Run` 定义与 `gate.go:126` 自动角色说明共存时，`gate.Run` 得到唯一
`definition_only` proof；仅有角色说明时仍是 unproven。既有 Go/Java/Kotlin/C++/Rust/Python/JS/TS/Ruby/Swift/Lua/Proto/ArkTS/Cangjie
source-scope 正臂与 wrong-owner/multi-location 负臂保持。`internal/types` 全包 22.211s，endpoint/tool 相关回归全绿。

状态：`B572a=implemented`；`role-context=citable-not-declaration-authority`；`endpoint-ambiguity=real-locations-only`；
`B572b=P1-next`；`system-answer/diagram-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.176 B572b：已读 principal parser calls 直接进入 typed evidence，不再要求模型抄送

completion preflight 现以 `callChainReadParserRelationHandoffEvidence` 消费既有精确 selector。命中关系被投影为
`EvidenceProducerRepoMapPrincipalMemberCall`：`EvidenceRelationship + AnchorCall + exact caller/object/source/line`，并带 AST extractor、confidence、
`GroundingGrounded/TierSymbolTable`。该行进入 Mutable evidence 与当前 preflight 的 effective evidence，后续 finalizer 可直接引用；不再创建
`RepairEmitEvidence`，也不再以“already-read parser call relations lack typed handoff”阻塞模型。

权限边界未扩张：模型必须先提交 principal `member_set`；caller/callee 必须在该集合唯一解析，或在显式 completeness obligation 下与模型已选同 statement
sibling call 精确接合；source line 必须在本轮 read closure；provenance 只接受 tree-sitter/Cangjie parser。regex fallback、unread、endpoint ambiguous/absent、
project-orientation、runtime artifact/Trace 全部不投影。稳定 key 与既有 Mutable 行联合去重，重复 preflight 不追加第二份。

这是一条源码事实供给，不是答案接管：系统不创建/修改 AnswerDocument、Mermaid body、edge anchor、成员名册、关系方向选择或结论；模型仍决定哪些已证边与用户问题相关、
是否画入图以及如何解释。Rust roster、Cangjie、decorated member、same-statement completeness 正臂，以及 regex/unread/runtime/ambiguity/duplicate 负臂已钉。
`internal/types` 22.604s、`internal/tool` 166.385s、`internal/agent` 10.095s、`internal/orchestrator` 12.325s、`internal/tracediag` 全绿。

状态：`B572a=implemented`；`B572b=implemented/pending-r338`；`parser-fact-provider=exact-read-principal-only`；
`model-copy-loop=retired`；`system-answer/diagram-authorship=none`；`raw-prose-hard-gate=none`；Trace causal authority=`unchanged`。

#### §11.10.177 r338：B572 降低证明抄送开销；裸节点引用与端点模式仍造成两类假绿

r338 exact-two runner 2/2、人工 0/2。时序案由 r337 的 827s 降至 160s：`auto_pair_role_description` 不再制造端点声明歧义，
已读 principal parser call 通过 `repo_map_principal_member_call` 进入 typed evidence，旧的逐条抄送 repair 循环消失，证明 B572a/b 均已进入生产路径。
但答案仍把真实的共享汇点 `buildAnalysisIR -> gate.RunWith <- gate.Run` 说成 `gate.RunWith -> gate.Run`。finalizer 第一稿的虚构边被现有关系门正确拒绝并删除，
错误 prose 却保留；runner 只核字符串而假绿。

根因是 `B574-ENDPOINTMODEERASURE1/P1-high`：analyzer 首次已经提交 `source=buildAnalysisIR,sink=gate.Run`，只是把枚举误写成
`sink_mode=discover`；wire validator 硬拒后，重试把两个端点同时清空为 `discover_path`，从而关闭 exact directed reachability 和 typed
`no_directed_path` 边界。该形不是语义歧义，而是结构化 payload 内部唯一可修复的局部枚举矛盾。最优修法是在当前请求 provenance 校验前将
“discover + 两个非空端点”规范化为 exact；若任一端点不是 typed current-request identity，既有 normalization 仍降成 discover/discover_path。
不得扫描请求/模型/终稿文本、不得按实体提及顺序选方向，也不得让概念型空端点 discover_path 失效。

类型关系案 266s、7 次 finalizer reject，最终仅剩 12 个节点和名册、没有一条关系，runner 同样假绿。`B573-FLOWALIASBARE1/P1-high`
确认发生在共享 Mermaid 标签身份层：`AE["analyzerEvaluator\\n..."]` 是唯一显式标签，随后 subgraph 中的裸 `AE` 只是引用；当前聚合却把
裸引用当成第二个标签 `AE`，因“冲突”删除 label mapping，使 endpoint 退化为 `AE`，exact provider 无法匹配。修复应让一个或多个裸引用继承
唯一显式标签；两个不同显式标签仍必须 fail-closed。该规则与 Go/implements 无关，统一覆盖 flowchart 的 call/type/guard/import/data-flow 等关系。

266s 的活动模型流超过四分钟但仍由原模型正常结束，未发生时长驱动的系统代答。固定累计四分钟继续不得触发答案降级、草稿替换、删答或系统结论；
只有 transport 断链、真实停滞、调用方取消或独立安全边界可以进入恢复，恢复也只能发布模型已经产生的载体并明确披露。B573 先以共享解析层小批修复，
B574 再以 typed schema normalization 独立提交，随后同案 r339 exact-two 回放。

状态：`B571=production-positive`；`B572a/b=production-positive`；`B573=P1-high/next`；`B574=P1-high/queued`；
`runner=2/2`；`human=0/2`；`active-stream-266s=no-degrade`；`fixed-four-minute-active-degrade=forbidden`；
`system-answer/edge-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.178 B573：flowchart 裸引用继承唯一显式 endpoint label

共享 diagram identity 汇总现把 standalone bare node（如 subgraph 中单独一行 `AE`）作为引用候选，与 shaped declaration
`AE["analyzerEvaluator\\n..."]` 分层保存。只要同一 node ID 存在唯一显式标签，一个或多个裸引用均继承该标签，不再制造第二个冲突 identity；
只有裸节点、没有 shaped declaration 时仍保留 Mermaid 合法的可见节点语义。两个不同显式标签继续判 ambiguous，裸引用不能替其中任何一方选权。

这项修复只恢复模型已写入 Mermaid 的显式展示身份，不从 label、edge label、请求、reasoning 或答案 prose 推断关系。边仍须携带模型提交的
`relation_kind`，type relation/call/guard/import/data-flow 等真值仍由各自 exact typed provider 授权；系统不补边、不改方向、不选成员、不写结论。
因此规则与 LoopController、Go 或 implements 无关，统一适用于所有语言和 flowchart 关系族。

回归钉住 r338 真实形：inline `AE[analyzerEvaluator…] -> LC[LoopController…]` 加 subgraph 裸 `AE` 可复用 exact provider；
两个显式标签加同名裸引用仍 fail-closed。定向测试与 `internal/tool` 全包 162.560s 通过。

状态：`B573=implemented`；`bare-reference=non-competing`；`explicit-label-conflict=fail-closed`；
`typed-relation-authority=unchanged`；`system-answer/edge-authorship=none`；`B574=P1-next`；Trace causal authority=`unchanged`。

#### §11.10.179 B574：双端点 discover 枚举矛盾局部自愈，精确方向权限不再被重试抹除

`emit_analysis` 现于 provenance normalization 前处理唯一可由结构化字段本身完全确定的 call-chain wire 矛盾：当
`sink_mode=discover` 同时携带非空 ordered `source` 和 `sink` 时，把枚举规范化为 `exact`，原样保留两个模型提交的端点，并在 tool summary
记录 normalization。该规则不读取用户/模型/终稿 prose，不按 entities 或提及顺序选择方向，也不发明任何 endpoint。

枚举修复不等于确权。修正后的 exact pair 随即进入既有 current-request provenance normalization：两端都获精确 provenance 才形成 exact authority；
只有 source 获证则降为 discover；两端均未获证则清空为 discover_path，由 grounded exploration 选择。显式空端点 discover_path 保持原义，
`discover_path + candidate endpoint` 和其余非法 wire shape 继续 fail-loud。这样 r338 的 `buildAnalysisIR/gate.Run` 不会因一次 enum 失误在重试中丢掉
directed reachability/no-directed-path 合同，同时概念路径请求不会被强行具体化。

回归钉住两端均有 provenance 时 discover→exact 且方向/字节不变；虚构双端点经过相同 enum repair 后仍降为 authority-free discover_path；
既有 discover_path candidate 拒绝与 exact-target contradiction 测试保持。系统仍只维护分析 IR 的 typed 约束，不改 AnswerDocument、Mermaid 或模型结论。

流式路径同时复核：SSE total wall-clock cap 仅在 `modelProgressReceived=false` 时生效；reasoning/content/tool delta 任一真实进展后，累计四分钟及更长时间
不能触发系统代答、删答、草稿替换或结论降级。首字节超时、真实字节静默、精确重复退化、调用方取消和纯 keep-alive transport cap 仍是独立精确信号。

状态：`B574=implemented`；`discover+two-endpoints=normalize-exact-before-provenance`；
`unproven-candidates=discover-path`；`raw-prose-hard-gate=none`；`fixed-four-minute-active-degrade=forbidden`；
`system-answer/edge-authorship=none`；Trace causal authority=`unchanged`。

#### §11.10.180 r339：关系别名与端点模式闭正证；no-path 边界缺少反向 wrapper typed 事实

r339 exact-two runner 2/2、人工 1/2。类型关系案 248s、1 次 finalizer reject，最终保留 12 个 production implementer、文件定位与
12 条 `implementer -> LoopController` 有向关系；不再发生 `AE(AE)` identity 退化或 7 次删边风暴，B573 production positive。首稿在 flowchart
里使用 class relation operator，被现有语法/锚一致性门拒绝后，模型一次 patch 改成带 type_relation anchors 的普通 flow edge；记为 syntax-family
教学观察，不按本 case 或 operator 做自动改图。

时序案 245s，B574 production positive：analyzer 首次即形成 exact `buildAnalysisIR -> gate.Run` endpoint profile，completion 的 directed
reachability gate 明确报告目标不可达，模型随后提交并获准 typed `principal_span_waiver=no_directed_path`。finalizer 第一稿虚构
`gate.RunWith -> gate.Run` 图边时，现有关系证据门也正确拒绝并删除该边。但最终正文与 ordered list 仍声称“RunWith 内部调用 Run”；源码真相恰好相反：
`gate.Run` 在 gate.go:135 调 `RunWith`，而 `buildAnalysisIR` 在 analyzer.go:2720 直接调用 `RunWith`，绕过 `Run`。runner 的字符串 oracle 再次假绿。

确认 `B575-NOPATHBOUNDARYEDGE1/P1-high`：no-path completion 已要求 exact endpoint definition，但 finalizer 只收到定义与正向不可达说明，没有收到已读
endpoint body 中解释边界的 AST call。B572b handoff 当前只投影同一 principal member-set 内的 parser relation；请求 endpoint `gate.Run` 不在旧 roster，
所以真实 `gate.Run -> gate.RunWith` 被漏掉。最优扩臂仍复用同一 deterministic provider：仅当 typed no_directed_path active，且一端精确匹配请求
source/sink、另一端唯一匹配 principal member-set 时，投影已读 line 上的 tree-sitter/Cangjie call。不得投影任意 endpoint 内部调用，不得消费 regex/unread，
不得从 waiver rationale、请求、reasoning 或终稿猜边。系统只提供反向/平行 source fact，模型自行决定解释、图、业务措辞和结论。

两路均在约四分钟后由原模型正常结束；累计时长没有也不得产生答案降级。B575 独立施工后继续同案 r340 exact-two，人工以“正文明确 Run 调 RunWith、
buildAnalysisIR→Run 不可达，图无反向虚构边”为验收，不依赖宽松字符串 oracle。

状态：`B573/B574=production-positive`；`B575=P1-high/next`；`runner=2/2`；`human=1/2`；
`active-stream-248s=no-degrade`；`fixed-four-minute-active-degrade=forbidden`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.181 B575：no-directed-path 边界携带已读 endpoint↔principal parser 关系

`callChainReadParserRelationHandoffEvidence` 新增一条严格 typed 的边界车道：仅当模型已经提交并通过
`principal_span_waiver=no_directed_path`，请求仍持有 exact ordered source/sink，且已读 AST/Cangjie call 的一端双向精确兼容该请求 endpoint、
另一端唯一解析到模型 principal member-set 时，才把该真实 call 投影到 finalizer evidence。r339 的 `gate.Run -> gate.RunWith` 因而与
`buildAnalysisIR -> gate.RunWith`、`buildAnalysisIR -> gate.Run` 不可达边界同时可见，不再只给一个 endpoint definition 让模型猜 wrapper 方向。

权限没有扩成“扫描 endpoint 函数的所有调用”：没有 active typed no-path waiver 时该车道关闭；counterpart 不在 principal member-set 时关闭；
regex fallback、unread line、runtime Trace、端点歧义继续关闭；已存在等价 call evidence 仍按 stable key 去重。匹配只读 typed endpoint profile、
waiver enum、parser relation、read closure 和 member-set，不读 waiver rationale、用户/模型/终稿 prose，也不按名字前缀把 Run/RunWith 当同一符号。

系统仍不修改 AnswerDocument、Mermaid 或结论：它只提供一条源码本来就存在的反向/平行事实；模型决定是否展示、如何用业务语言解释边界。
回归钉住 Go wrapper 正臂，以及无 waiver、regex provenance、非 principal counterpart 三个负臂；既有 Rust/Cangjie principal handoff 与
unread/runtime/ambiguity 边界保持。

状态：`B575=implemented/pending-r340`；`no-path-boundary-fact=exact-endpoint-to-unique-principal`；
`arbitrary-endpoint-body-expansion=forbidden`；`raw-prose-hard-gate=none`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.182 r340：B575 方向事实闭正证；同一 typed callee 被全限定名/短名画成两个节点

r340 exact-two runner 2/2、人工 1/2。类型关系案 172s、零 finalizer reject，首稿 classDiagram 即完整保留 12 条 implements 关系、production 名册与
文件定位，B573 回归稳定。时序案 248s，B575 production positive：finalizer context 明确携带 `buildAnalysisIR -> gate.RunWith`、
`gate.Run -> gate.RunWith` 和 exact `buildAnalysisIR -> gate.Run` 不可达三项；最终正文也按正确方向说明两个独立入边在 RunWith 汇合，r339 的
RunWith→Run prose 反转已消失。

但图仍不能判人工通过：模型声明 `n9 as gate.RunWith` 和 `n11 as RunWith` 两个 participant，再分别承接两个入边；两者实际是同一函数，文字说“共享汇合点”，
视觉却是两个节点。确认 `B576-TYPEDENDPOINTALIASTWIN1/P1-high`：现有 `duplicate_participant_identity` 只以 endpoint 完全相同字符串分组，未利用
同一 citable call EvidenceItem 已同时携带的 `Object=gate.RunWith` 与 `AnchorSymbol=RunWith`。这不是 fuzzy label 问题，而是 typed evidence
同源别名未归一。

最优修法是从每条 citable call row 构造 caller/callee 各自的 alias family：Subject/OwnerSymbol 只归到该 caller canonical，Object/AnchorSymbol
只归到该 callee canonical；一个 alias 只有唯一 canonical owner 时才参与重复节点检测。同一短名若同时属于多个 owner，映射删除、保持 fail-closed；
class/actor carrier + 不同 operation 的合法多 participant 继续通过。不得从 Mermaid label、请求、reasoning、终稿或路径猜 owner，系统只拒绝同一 typed
identity 分裂，不合并/改写模型节点。

时序模型流超过四分钟仍由原模型正常交付，无时长降级。B576 施工后用同案加异语言短名歧义 pin，随后再进入不同 read/write/Trace 优先级 eval。

状态：`B575=production-positive`；`B576=P1-high/next`；`runner=2/2`；`human=1/2`；
`active-stream-248s=no-degrade`；`fixed-four-minute-active-degrade=forbidden`；`system-answer/edge-authorship=none`；
Trace causal authority=`unchanged`。

#### §11.10.183 B576：duplicate participant 复用 typed call-row 的唯一 endpoint alias family

`duplicate_participant_identity` 不再只比较 endpoint 的完全相同字符串。校验器现从每条 citable call EvidenceItem 构造两个互不串位的 family：
caller canonical 只来自 Subject；callee canonical 来自 Object，而同一行的 Object/AnchorSymbol 都登记为该 callee 的精确 alias。Mermaid participant label
先经既有 exact label parser，再通过 alias→canonical 映射分组；`gate.RunWith` 与 parser-short `RunWith` 因而被识别为同一 typed callee，两个 node ID
会得到一个可执行的 duplicate repair，而不是把“汇合”画成两个节点。

唯一性继续 fail-closed：若 `RunWith` 同时由 `pkg.one.RunWith` 与 `pkg.two.RunWith` 两条 typed call row 携带，该短 alias 没有唯一 owner，映射不生效；
class/actor participant 依赖不同 message operation 的合法复用也不受影响。规则不读请求、模型 reasoning、终稿、edge label 语义或路径，不用大小写/前缀/相似度猜 owner；
系统只拒绝 identity split，不自动合并节点、改 Mermaid、选择边或代写答案。

回归覆盖 r340 的 qualified+short callee twin、跨 owner short-name ambiguity、既有 exact duplicate、class carrier 多 operation。定向测试通过，完整包回归随后记录。

状态：`B576=implemented`；`typed-alias-family=per-call-row-side-aware`；`ambiguous-short=unresolved`；
`model-diagram/answer-authorship=preserved`；`raw-prose-hard-gate=none`；Trace causal authority=`unchanged`。

#### §11.10.184 r341：B576 生产闭环；写模式单批 patch/verify 正证

r341 继续遵守 exact-two，并把维度从同类关系双跑切换为「时序关系 + 写模式」。runner 2/2、人工 2/2。

时序案 165s、零 finalizer reject。最终 Mermaid 只声明一个 `gate.RunWith` participant，`buildAnalysisIR` 和 `gate.Run`
分别以各自已证 call row 指向同一汇点；正文同步说明 `buildAnalysisIR -> gate.Run` 不可达，没有再出现 r339 的方向反转或 r340 的双节点假汇合。
这证明 B576 的 side-aware typed alias family 已进入生产路径。校验器没有合并或改写图、没有补边，也没有替模型生成结论。

写模式案 96s。ChangePlan 只有一个 `kind=patch` 的 `main.go` 目标，统一 diff 只将第 25 行 `retrun` 改为 `return`；
应用引擎为 `structured_builder/git_apply`。随后 verifier 从 typed TestSurface 选择 `go test -json ./...`，exit=0、1/1 测试通过，
changed-path coverage 明确为 `main.go/covered/project_runner/target_behavior`，最终 workflow 只在 `batch_verified/tests_passed` 后 finish。
该案是单批 plan→apply→verify 正证，不经过 replan，不能被外推成对历史 T7-1「重规划后累计验证域」的替代验收；本轮未发现新的写模式高危 GAP。

两案模型活动流均未触发累计时长降级。继续冻结运行时不变量：只要 reasoning/content/tool delta 仍有真实进展，4 分钟及更长累计时长不得
发布系统答案、删减模型答案、切换旧草稿或以超时名义终止；只有首字节超时、真实 stall、transport 断链、调用方取消或独立安全边界可进入恢复，
恢复也只能尽力发布模型已经产生的载体并明确披露，不能新造关系、Trace 主因或业务结论。

状态：`B576=production-positive`；`runner=2/2`；`human=2/2`；`write-single-batch=positive`；
`write-replan-cumulative-domain=not-exercised`；`active-stream-fixed-time-degrade=forbidden`；
`system-answer/diagram-authorship=none`；`raw-prose-hard-gate=none`；Trace causal authority=`unchanged`。

#### §11.10.185 r342：Cangjie 清单正证；Trace 模型越界待复跑，同一自身状态双席已根修

r342 exact-two runner 2/2、人工 1/2。Cangjie 清单案 103s：typed inventory 的 2 个 extend、2 个 foreign func、8 个
public class 全部逐行进入答案，每项均携带真实文件/行号与源码 package 声明；未按目录推断 package，零成文拒绝，证明异语言清单面保持稳定。

Trace 显式窗案 168s，系统供给的大部分事实正确：5.000..5.007s 请求窗未漂移；app-100 四态为 running 1.200ms、runnable
0.800ms、sleep 5.000ms、D/IO 0；链上 VerifyClass 原始墙钟 5.000ms、有效可消除 4.600ms 居首，目标 runnable 0.800ms 居次；
background 供给压力未升格；主要占时与规则可消除量两轴同时发布。模型还正确声明 `trace_causal_claim_caliber=bounded_window_candidate`，
但自由文本反而越过同一轮 typed 边界：把唤醒前 sleep 叫作 5ms wakeup 延迟，把没有 target-wait/completion/direct-block authority 的
worker 工作写成“强制等待”，并仅凭低优先级关系宣称优先级反转。finalizer context 已明确给出 sleep→wakeup 与 wakeup→run 的区间语义，且给出
`target_direct_blocking_authority=not_provided`、`target_wait_for_work_authority=not_provided_by_this_seat`、
`work_completion_dependency_authority=not_provided_by_this_seat`；上下文 27%，不是预算挤压。立案
`B577-TRACECLAIMCALIBERDRIFT1/P1-observe-repeat`：下一批用同案复跑；若不复现按模型波动留档，若稳定复现只能考虑让模型自持的结构化 claim/review
协议或软提示，禁止扫描用户/模型/终稿 prose 做硬门，禁止系统删除、替换或重写模型结论。

同案同时抓到独立确定性 GAP `B578-SELFSTATETWINESCAPE1/P1-high`：同一 app sleep 物理段由
`wakeup_causal_impact` 与 target-self rank view 发布，树和指标表各出现两个 `自身·sleep 5.000ms` 席。现有 SELF-TWIN typed matcher
错误要求第二视图 predicate 必须为 `root_cause_target_self_state`，但生产引擎在该段不具有效归因时合法发布
`predicate=root_cause_context_only,tier=target_self_state`，于是被简化 fixture 掩盖。修复以 typed `target_self_state` tier 作为身份权威，继续要求
subject、state class、display/cumulative caliber、line segment 与 selected window 全部精确相等；不按渲染字符串去重，任一键不同或 carrier 多义仍 fail-open。
端到端请求窗 fixture 已先红后绿，新增 production predicate 正臂，原 mismatch/ambiguous 负臂保持。

活动流规则本轮继续冻结：168s 正常由原模型交付；只要 reasoning/content/tool delta 仍有真实进展，4 分钟或任何固定累计时长都不得触发系统代答、
删答、旧草稿替换或结论降级。只有首字节超时、真实 stall、transport 断链、调用方取消或独立安全失败可进入恢复；恢复只可发布模型已产生的载体并披露，
不能新造 Trace 主因、图关系或业务结论。

状态：`B577=pending-repeat`；`B578=implemented/pending-full-regression`；`runner=2/2`；`human=1/2`；
`cangjie-inventory=positive`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
`active-stream-fixed-time-degrade=forbidden`；Trace explicit-window/materialization=`unchanged`。

#### §11.10.186 r343：自身状态双席闭正证；调度相位污染与 actor-operation 多边冲突

r343 exact-two runner 2/2、人工 1/2。Trace 案 138s，B578 production positive：因果树中同一 app sleep 物理段只剩一条
`自身·sleep 5.000ms [E1+E2]`，rank 与 wakeup 两个证据入口合并但没有重复计量；显式 5.000..5.007s 窗、四态、链上
VerifyClass 4.600ms、目标 runnable 0.800ms、实际占时/规则可消双轴、背景权限与因果投影均未退化。模型也不再声称直接阻塞或经典
优先级反转，并明确 frame evidence absent；B578 因而关闭。

但模型连续第二次把 `5.000000 sleep → 5.005000 sched_wakeup` 写成“唤醒延迟 5.000ms”。这次日志进一步定位到上游污染源：
perf_triage advisory 先把 5.005800 switch-in 说成“实际被唤醒”，又把 sleep+runnable 合写成 5.8ms 阻塞；随后 finalizer 的 typed
相位说明虽正确，模型仍受早期导航事实影响。`B577-TRACECLAIMCALIBERDRIFT1` 从 observe-repeat 升为
`P1-soft/context-source-fix`。修复只调整 perf_triage 教学：`t_sleep→t_wake` 是 sleep/blocking，`t_wake→t_run` 是 runnable
scheduling delay，`t_sleep→t_run` 是 total non-running；禁止把前两者混叫 wakeup latency、禁止把 switch-in 当 wakeup。该面仍是模型
advisory，最终权威继续来自 deterministic trace_query；不扫描模型终稿、不硬拒、不删除或改写结论。

read combo 案 360s，正文、阶段表和最终四阶段 precedence 图准确，人工按题面通过；但 3 次 finalizer reject 暴露新的结构 GAP。
模型最初用一个业务友好的 `Orchestrator` actor 承载两条不同的已证内部调用：
`runAnalyzePhase→dispatchStage` 与 `runTaskGraph→runReadSchedulerLoop`，并为每条 message 携带各自 exact
`from_identity/to_identity`。旧校验按裸 `orch→orch` node pair 聚合全部 anchors，视为冲突，连续拒绝后迫使模型删掉两条真实关系。
这不是模型波动，也不是证据不足，而是图中“可见 actor 载体”与“typed operation endpoint”被错误合一。立案并实现
`B579-ACTOROPMULTIEDGE1/P1-high`：同一 raw node pair 有多个 distinct exact identity pair 时，按 Mermaid visible occurrence 与
anchor 首次出现顺序一一绑定；只有一个 identity pair 时保留既有多 occurrence 复用语义。每个选中 pair 仍须逐条通过 citable call evidence，
多出的可见 message 继续 fail-closed；反向基数也新增校验，多出的 exact anchor 若没有 visible occurrence，报
`typed_anchor_without_visible_edge`，不得成为隐藏关系图。message label 只负责业务展示，不能选择、反转或证明关系。

该修复与 Go/本 case 无关，适用于所有支持语言以及 sequence/call relation 中 actor、class、component 等 carrier 复用；系统不补 Mermaid
边、不合并 participant、不改模型正文或结论。回归使用中文业务 message，钉住两条 self-message 正臂、第二条无证据、第三条无 anchor、第二个
hidden anchor、重复同一 call-site occurrence budget 五类边界。

关于“活跃连接 4 分钟没有答案是否降级”的复核结论是：**不会，也禁止这样做**。stream watchdog 的
`modelProgressReceived` 只由已解析 reasoning/content/tool-call/finish/usage typed delta 置位；一旦存在真实模型进展，
`2×request_timeout` transport-only 总帽退出判定域。纯 keepalive、空 data frame、malformed heartbeat 或 role-only framing 不算模型进展，
仍由总帽兜底；真实字节静默由 stall watchdog 处理。本轮 360s 生产 eval 和
`TestDoStreamRequest_ActiveVisibleProgressMayOutlastTotalCap` 都证明活跃模型可跨旧 4 分钟边界正常交付。任何恢复仍只能发布模型已产生载体并披露，
不能用系统答案替换模型答案。

状态：`B578=production-positive/closed`；`B577=implemented-soft-guidance/pending-r344`；
`B579=implemented/pending-full-regression-r344`；`runner=2/2`；`human=1/2`；
`active-stream=360s/no-elapsed-degrade`；`transport-only-cap=preserved`；`raw-prose-hard-gate=none`；
`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.187 r344：软教学未阻断预分析语义污染；内部关系合同词泄漏到用户图

r344 exact-two runner 2/2、人工 1/2。Trace 案 162s，B578 继续稳定：同一目标 sleep 物理段只发布一次
`自身·sleep 5.000ms [E1+E2]`；显式 5.000..5.007s 窗、四态账户、链上 VerifyClass 4.600ms、目标 runnable
0.800ms、实际占时/规则可消双轴和 bounded causal projection 均未退化。但 B577 连续第三次生产复现，证明上一批仅增加
perf_triage 教学不足以闭环。预分析模型仍把 5.005800 switch-in 称为“被唤醒”，把 5.000ms sleep 与 0.800ms runnable
混成 5.8ms 等待，并把这些自由语义写入 `PerfObservation.Subject/Summary`；后续 analyzer/finalizer 即使同时拿到正确 deterministic
trace_query 相位与 `target_direct_blocking_authority=not_provided`，仍沿用“阻塞唤醒”叙述，形成同页权限矛盾。

根因不是缺少另一句禁令，而是预分析模型产物的权限与投影形错误：`compilePerfBundleObservations` 默认把每条
`authority=pretriage_model_extraction` 记录铸成 principal answer；`formatPerfTriageStructured` 虽写“navigation hypothesis”，仍把模型作者的
Subject/Summary/Evidence/Tags 原样交给下游，嘈声语义因此与 deterministic 查询竞争。立案 `B577b-PRETRIAGESEMANTICAUTH1/P1-high`：
模型预分析 observation 在任何阶段只能是 supporting/navigation；下游结构上下文只给 candidate line/time/duration locator，不再把其自由语义字段
当事实复述。原 bundle 与 ledger 仍完整保留供审计；deterministic-validator 记录保持全文与 principal 权限。该修法不扫描用户、reasoning 或终稿，
不硬拒成文，不替模型改答案，也不削弱显式窗 Trace 查询、链上主因、自动补齐或无 trace_query 场景对原始 trace 的访问。

read combo 案 276s，最终表格、四阶段顺序以及 typed call/data-flow/precedence 关系正确，人工通过；B579 的同 actor 两 operation
生产分支本轮没有被模型选用，故仍只具单元/包级正证。新确认 `B580-INTERNALRELLEXICONLEAK1/P2-soft`：finalizer 把
`relation_segment_class=invocation_segment/value_flow_segment` 和 `unproven_between_components` 作为内部边界元数据提供后，模型逐字复制成
用户图 Note。最优处置是同一 typed component handoff 明确这些 token/index/status 仅供校验推理，不得直接进入可见图、标题、表格或正文；模型用业务
participant/operation 表达已证关系，若没有有价值的业务注释则省略 Note。该面只做软教学，不新增关键词硬门，系统不重写 Mermaid。

276s 活跃模型流由原模型正常交付，继续证明累计四分钟不是降级信号。只有首字节超时、真实静默、transport 断链、调用方取消或独立安全失败可进入恢复；
恢复仍只能发布模型已产生载体并披露，禁止按累计时长系统代答、删答、切旧稿或降低结论。

状态：`B577-soft=insufficient`；`B577b=next`；`B580=next-soft`；`B578=stable-positive`；
`B579=unit-positive/production-pending`；`runner=2/2`；`human=1/2`；`active-stream-276s=no-degrade`；
`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.188 B577b/B580：预分析语义降为 locator；内部关系元数据退出读者词面

`PerfObservation` 新增 validator-owned 权限判定 chokepoint：只有 `authority=deterministic_validator` 能携带结构化语义；
`pretriage_model_extraction` 和旧 bundle 的空 authority 均 fail-closed 为 navigation-only。observation ledger 在 deterministic query 尚未发生前
就把这类记录定为 `supporting_coverage + soft + inferred_upstream_possibility`，并用注册 marker 留存权限；稍后 query 出现时只追加既有
pretriage-vs-query 优先级标记。validator-owned observation 即便共享 `perf_trace` producer，也不会被宽泛 producer precedence 误降级。

下游 `formatPerfTriageStructured` 对 navigation-only observation 只发布 candidate trace line/start/end/duration locator；模型作者的
Subject/Summary/Evidence/Tags 仍完整保存在原 `PerfBundle` 与审计 ledger，但不再作为结构化事实重复灌入 analyzer/explorer/finalizer。
deterministic-validator 记录保持原全文、数值与 principal 权限。已有 trace_query 时，空 authority 的 legacy 自由语义也不再通过“兼容”名义逃逸。
该改动没有删除原始 trace：模型仍可读取工件并调用 deterministic trace_query；没有扫描请求/推理/终稿文本，没有硬拒或系统改写答案，也没有变更
显式窗、链上根因、优先级反转/调度/算力/D/IO/确定性 span 的计算与自动补齐。

B580 在 typed component boundary 同源增加 reader-facing language boundary：`verified_component`、component index、
`relation_segment_class`、`answer_role` 和 bridge/status token 明确仅是内部校验元数据，不得逐字进入 Mermaid Note、标题、表格或正文；模型应以仓库领域
participant/operation 表达已证 segment，无有用业务注释时省略 Note。它是软教学，不是对模型输出的关键词硬门；不授权跨 component 边，也不由系统
重写图或结论。

回归覆盖：navigation 自由语义四字段均不进入 downstream prompt，但 line/time/duration locator 保留；validator-owned observation 全字段保留；
pretriage 与 validator observation 在 deterministic query 同在时分别保持 supporting/principal；ledger marker registry tripwire 与 component boundary
业务词面指令均钉住。定向 `internal/context`、`internal/types`、`internal/agent` 全绿，下一批用相同 Trace/read combo exact-two 做 production 验收。

状态：`B577b=implemented/pending-r345`；`B580=implemented-soft/pending-r345`；
`pretriage-freeform-downstream=withheld`；`deterministic-observation-authority=preserved`；
`raw-trace-access=preserved`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.189 r345：预分析污染解除后暴露 deterministic 相位误名；关系修补仍删主脊

r345 exact-two runner 2/2、人工 0/2。Trace 案 122s 首先证明 B577b 已进入生产：下游上下文的六条模型 observation 只剩
`navigation_locator + candidate line/time`，Subject/Summary/Evidence/Tags 均未泄漏；validator-owned priority/timestamp 行保持全文。
最终答案也不再把 5.000ms sleep 与 0.800ms runnable 合成 5.8ms wakeup-to-run 延迟，显式窗、四态、链上 rank、双轴、frame absent
限定、因果投影与 B578 单席 sleep 全部稳定。

但答案仍把 5.000ms 称为 `wakeup latency`，本轮定位到新的确定性源错误 `B581-WAKEEDGEPHASECALIBER1/P1-high`：
`BuildWakeupChain` 的 `WakeupEdge.LatencyMs` 实际公式是 `(sched_wakeup.ts - wakee_sleep_segment.start)`，即 pre-wakeup
sleep/blocking→wake wait；`trace_wait_evidence_summary` 却发布 `wakeup latency Xms`，typed observation summary 也只写无口径的
`latency=Xms`。模型因而按系统精确载体复述错误术语，并进一步说 VerifyClass“完成后才触发 wakeup”，而真实 span 结束 5.005400 晚于
wakeup 5.005000。最优根修是保持 JSON `latency_ms` 兼容，但给 WakeupEdge 增加/发布固定 typed caliber
`sleep_start_to_sched_wakeup`，所有 LLM-facing 面改称 `pre-wakeup sleep→sched_wakeup wait`；wake→run 调度延迟继续只来自 runnable 账户。
不得用终稿关键词扫描纠正，也不得把系统字段误名交给模型背锅。

read combo 案 316s，B580 production positive：终稿不再出现 `invocation_segment`、`value_flow_segment` 或 bridge-status 内部词。
但四轮 `成文校验未通过` 后模型不断删边，最终必需 sequenceDiagram 缺少 Analyze→Explore，只剩两条脱离 stage 主脊的 orchestrator call
和 Explore→Extract→Finalize；正文/表格正确但图关系不完整，人工失败。确认 `B582-REQUESTSPINEREPAIRFOCUS1/P1-high`：现有上下文虽声明
requested spine，却仍把九条跨 component recipe 同权交给修补模型，模型在“补 call anchor/删 unsupported edge/保 precedence”之间反复搬运。
最优方案是在 typed request-spine 已存在时，由同一个 evidence provider 生成一份紧凑的 principal-diagram recipe，仅列该 spine 的 exact node、
edge、relation_kind 和 anchor JSON；disconnected supporting component 明确退为可选正文/独立图证据。它只减少修补心智，不生成 Mermaid、不补边、
不删除模型边、不改正文结论，也不使用请求/终稿关键词硬门。

316s 活跃模型流再次跨过旧四分钟边界并正常结束，未发布系统答案、旧稿或降级答案；累计时长规则保持不变。

状态：`B577b=production-positive`；`B581=next`；`B580=production-positive`；`B582=next`；
`runner=2/2`；`human=0/2`；`finalizer-rejects=4`；`active-stream-316s=no-degrade`；
`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.190 B581：WakeupEdge 等待值补齐相位口径，禁止冒充唤醒后调度延迟

代码复核确认 `WakeupEdge.LatencyMs` 的唯一生产公式为
`(sched_wakeup.ts - selected_sleep_or_blocking_segment.start) * 1000`。它度量的是线程从进入该 sleep/blocking 段到
`sched_wakeup` 的**唤醒前等待**，不是 `sched_wakeup → sched_switch(next=target)` 的 runnable 调度延迟，也不能证明相邻业务 span
已完成或触发了 wakeup。旧实现却在 query banner、typed observation summary、via-thread 逐跳摘要和 finalizer wait evidence 四面分别发射
`latency`、`per-hop latency`、`wakeup latency`，把确定性正确数值配上了错误相位语义；这是系统上下文污染，不是模型波动。

本批保留兼容 wire key `latency_ms`，在 `WakeupEdge` 与 `ChainViaHop` 增加闭合口径
`latency_caliber=sleep_start_to_sched_wakeup`。引擎新构造直接铸造该口径；legacy/手工结果在发布 chokepoint 按这个字段唯一既有定义补齐，
不猜测其他延迟。查询横幅改为 `pre_wakeup_wait`，typed summary 同时给出值与 caliber，模型 wait-evidence 明确写出
`sleep/blocking start → sched_wakeup`，via-thread 每跳也携带同一口径；`sched_wakeup → switch-in` 仍只由 runnable 调度账户提供，两个相位
不得相加或改名。新增 note key 进入 registry，WakeupEdge R2' schema 完成逐字段处置后重钉；没有绕过 schema tripwire。

该修复只校准系统提供给模型的精确信息，不从时间相邻推导业务完成依赖，不扫描终稿术语、不硬拒或重写模型答案，也不改变链构造、根因排序、
可消除量、显式时间窗、因果投影或自动补齐。回归覆盖真实 BuildWakeupChain via hop 的 caliber、legacy edge 发布归一、query summary/note 双面、
finalizer evidence 的正词与 `wakeup latency` 负 pin、note registry 和 tracediag schema pin。

验证：`go test ./internal/tracequery ./internal/tool ./internal/context ./internal/tracediag` 中 tracequery/tool 全套先绿；字段重钉后
`go test ./internal/context ./internal/tracediag`、typed observation/note 定向和 via-thread 定向均绿。下一批实施 B582，再以 exact-two
Trace/read-combo 回放同时验收 B581/B582。

状态：`B581=implemented/pending-r346`；`B582=next`；`legacy-latency-ms=preserved`；
`post-wakeup-scheduler-latency=separate`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.191 B582：请求主关系提炼为 repair-first typed recipe，支撑边不再挤掉主脊

r345 的 read-combo 不是模型单次波动：系统已经通过 request-scoped typed authority 精确持有
`Analyzer → Explorer → Extractor → Finalizer` 三条 precedence 主脊，但修补上下文仍把它与六条断开的 call/data-flow 支撑 recipe 同权铺开。
四轮成文校验中，模型在补调用锚、保 precedence 与删除无支撑边之间反复搬运，最终删掉 `Analyze → Explore`，却保留两条与主脊断开的
orchestrator call。证据没有缺失，缺的是一份紧凑、优先、不可被支撑清单淹没的 typed 必保集合。

本批在 `renderAnswerDocMechanismRelationComponentBoundary` 的同源 recipe 投影上新增 repair-first principal block：只选择
`edge.requestSpine=true` 的既有行，逐条发布 exact node alias、endpoint identity、`relation_kind`、完整 `edge_anchor_json` 与源码位置；断开的
grounded sibling 明确为可选正文或独立有界图，不得替换、截断、桥接或插入请求主脊。完整 capsule 仍保留所有可引用事实，模型仍负责可见 Mermaid、
业务措辞、总结与结论；系统不生成图体、不补关系、不删模型边，也不从用户/模型/终稿原文关键词推断主脊。

这是 relation-family 通用机制，不依赖 Go/Java 或单个 case：任何未来由 request-scoped typed authority 标记的 spine 都走同一投影，支持项目全部语言；
没有 requestSpine 时零输出、旧行为不变。内部 recipe token 明确不得进入读者可见图，visible label 继续由模型使用业务领域语言撰写。

回归钉住：四个主节点、三条 precedence 边、首尾两条完整 anchor JSON；supporting call 的 endpoint 与 `relation_kind=call` 不得进入 compact
principal block，但仍存在于完整 capsule；component bridge 继续 unproven，禁止为成图而造桥。`go test ./internal/agent` 全绿。

状态：`B582=implemented/pending-r346`；`principal-recipe=typed-subset-only`；
`supporting-relations=preserved-outside-principal`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.192 r346：等待相位与请求主脊生产转正；表格行协议混用、泛化标签取证死循环立案

r346 exact-two runner 2/2、人工 1/2。Trace 案 122s，B581 production positive：5.000ms 不再以 `wakeup latency`
发布，而是明确为 sleep/blocking 段起点到 `sched_wakeup` 的 pre-wakeup wait；答案同时说明 VerifyClass span 结束晚于
`sched_wakeup`，没有从时间邻近虚构“完成后才唤醒”的依赖。显式 5.000..5.007s 窗、目标四态、链上 VerifyClass
4.600ms 与 runnable 0.800ms 排序、实际占时/规则可消双轴、frame absent 限定和因果投影全部保持。

read combo 案 499s，B582 production positive：首稿仍忽略 compact principal recipe 并画入无证 orchestrator call，但仅一次
finalizer reject 后，第二稿完整保留 `Analyze → Explore → Extract → Finalize` 三条 typed precedence 主脊；支撑 call 没有再挤掉或替换主脊。
相较 r345 的四次拒绝和主脊缺边，repair 心智与结果均明显改善。

该案同时确认两个新 GAP。`B583-TABLEROWMODEMIX1/P1-high`：同一 structured table 的前三个 label-first 行使用
`len(cells)==len(columns)`（columns 省略 label header），最后一行使用 `len(cells)+1==len(columns)`（columns 包含 label header）。
旧 normalizer 对每行独立判合法，却不要求全表协议一致；renderer 按表级 `hasLabel/maxCells` 统一渲染，导致最后一行从 Stage 起整体左移一列，runner
仍签 PASS。最优解是只读 schema carrier 的精确硬门：全表固定一种 row convention，禁止同行混用；不按表头或单元格词义推断，不补值、不改写模型内容。

`B584-PARTICIPANTROLEENDPOINT1/P1-high`：analyzer 为满足 request-verbatim provenance，把 `codrax read mode`、`stage` 等泛化请求标签
铸成 `incident_required`；completion gate 随后要求每个标签必须有 citable source operation。它们是读者可见范围/领域标签，不是源码 endpoint，因而
结构上无法满足，造成 47 次 explorer iteration、11 次 midloop inject、34 次 read 与 499s 延迟，并在终稿留下两个断开且无价值的 participant。
需要在 typed participant role 中区分“请求可见上下文身份”和“源码操作入射义务”，不能删掉 provenance，也不能靠扫描请求/答案词语放宽所有证据门。

499s 活跃模型流由原模型正常交付，没有按累计时长降级、切旧稿或发布系统答案。运行时不变量继续冻结：真实 reasoning/content/tool delta
活跃时，4 分钟或任何固定累计时长都不构成恢复条件；只有首字节超时、真实 stall、transport 断链、调用方取消或独立安全边界可以恢复，且恢复只能发布
模型已产生载体并明确披露。

状态：`B581=production-positive/closed`；`B582=production-positive/closed`；`B583=confirmed/next`；
`B584=confirmed/design-audit-next`；`runner=2/2`；`human=1/2`；`finalizer-rejects=1`；
`active-stream=499s/no-elapsed-degrade`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.193 B583：结构化表格采用全表单一行协议，拒绝合法单行的非法组合

根因不是 renderer 少了一个补齐分支，而是 normalizer 只逐行接受三种独立合法形：纯 `cells[]`、label 加包含 label header 的 columns、
label 加省略 label header 的 columns；它没有记录首个可见行选择的协议，导致后续行可以切换协议。渲染器只能对整张表选择一次 label 列与列宽，
因此协议混用必然把某些行错列。静默补空单元格或按文本含义猜测列归属都会替模型编造数据，不可采用。

本批在 `validateEmitAnswerStructuredTableRows` 内建立 table-wide precise carrier invariant：当 columns 存在时，第一个可见 row 固定三种协议之一，
后续每行必须完全一致；混用 label presence 或两种 labeled width 均在 emit JSON 边界给出带 `items[index]` 的可修复错误。无 columns 的 fallback table、
完整 Markdown table 以及三种各自全表一致的合法形保持不变。该硬门只读取 schema 字段、布尔 label presence 与整数 cell/header 数量，不读取用户输入、
标题、表头词义、单元格文本或模型终稿 prose；系统既不补单元格也不改模型结论。

回归覆盖 r346 原形（同为 label row、先 synthetic-label-header 后 label-header-in-columns）和 label-first/cell-only 混用两类负臂；三种单独合法形继续通过。
定向 normalizer tests 与 `go test ./internal/tool` 全套均绿。

状态：`B583=implemented/pending-r347`；`table-row-convention=single-per-table`；
`model-cell-authorship=preserved`；`raw-prose-hard-gate=none`；`system-answer/table-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.194 B584：请求可见参与者与唯一源码端点拆权，消除不可满足的取证循环

r346 的 499s 不是单纯模型慢：analyzer 在 request-verbatim 约束下把 `codrax read mode`、`analyze`、`finalizer`、`stage` 全部铸成
`incident_required`。其中 `analyze/finalizer` 可被 checkout-verified stage precedence 覆盖；`codrax read mode` 是范围/概念标签，无法映射为源码
operation；`stage` 虽语法上像标识符，但 repomap 中存在多个同名定义，并不指向一个唯一 endpoint。旧合同却只看 role，要求后两者必须找到 citable
operation，导致 explorer 反复读文件、completion repair 重入，最终仍只能发布 disconnected boundary。

本批没有通过字符串黑名单识别 `stage` 或项目名，而是复用 analyzer 已有 typed `EntityProvenance` 并补齐“多定义”语义：repo resolver 返回一个
symbol hit 时仍为 `symbol/resolved/use_for_shape`；返回多个 hit 时铸为 `ambiguous_symbol`，仅允许导航搜索、禁止作为 hard shape/source identity；
零匹配概念、scope、prescan-only 也不能驱动源码操作硬门。qualified identity 仍由原严格 qualified oracle 解析，跨 Go、Java/Kotlin、ArkTS/TS、
C/C++、Rust、Python、Cangjie 等共用 repomap identity 层，不依赖语言或 case 词表。

新增 `DiagramParticipantHasPreciseSourceOperationIdentity` 作为单一判定源。production provenance 在场时，只有唯一已解析 symbol 才进入
`source_operation_required`；无唯一源码身份的 incident label 进入 `request_visible_boundary_only`。Explorer 教学、completion gate 与 finalizer typed
coverage 共用该拆分：前者只为唯一端点发起 operation search，后者对边界标签明确禁止搜索/连接同名 operation。参与者本身没有被系统删除或改写；
未证身份仍可作为断开边界由模型披露，系统不造边、不补图、不替模型决定业务关系。

Analyzer SSOT 同时压缩歧义：多展示面请求中，单独表格的 `each stage/every step/all rows` 是集合/列角色，不是一个图参与者；
`system/mode flow from A to B` 中 A/B 是关系参与者，system/mode 只是范围，除非用户明确要求画出周边边界，否则应省略。该教学是软分类指引；
硬门仍只消费 typed role/provenance/cardinality，不扫描用户原文、reasoning 或终稿。

回归覆盖：两文件同名 `stage` 被标为 ambiguous/navigation-only；唯一 Analyzer 与 decorated alias 保留硬 operation obligation；概念/范围标签不再进入
completion repair；legacy 无 provenance fixture 保持旧行为；Explorer 与 finalizer 同时发布 source/boundary 两个 typed 集合；Trace intent 继续完全绕开
source-flow participant 车道。`go test ./internal/types ./internal/skill`、`go test ./internal/agent ./internal/tool` 全绿。

状态：`B584=implemented/pending-r347`；`participant-display/source-operation-authority=split`；
`ambiguous-symbol=soft-navigation-only`；`model-participant-authorship=preserved`；`raw-prose-hard-gate=none`；
`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`；
`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.195 r347：表格/端点拆权生产转正；来源排除 JSON 错路由与回复箭头语义缺口立案

r347 exact-two runner 2/2、人工 0/2。Trace 案 126s：显式 5.000..5.007s 窗、目标 running/runnable/sleep/D/IO
账户、链上 VerifyClass 4.600ms 与 runnable 0.800ms 排序、实际占时/规则可消双轴、frame absent 限定、因果投影以及 B581
`sleep/blocking start -> sched_wakeup` 口径全部存在。模型终稿仍出现两处自由语义漂移：把结束于 5.005400 的 VerifyClass
说成在 5.005000 wakeup 之前“完成后唤醒”，并把 idle/1 叙述为抢占 CPU 的竞争者。系统给出的 typed context 已明确 span
晚于 wakeup 且 idle 不构成竞争，因此该项按人工失败记录；禁止通过扫描或替换终稿纠正模型结论，后续只观察跨样例复现，必要时升级模型自填
structured claim carrier。

Trace analyzer 同时暴露 `B585-SOURCEEXCLUSIONCARRIER1/P1-high`：用户明确说只分析 trace、不分析源码，模型虽然正确填写
`external_observation_policy.current_source_mode=exclude`，又把同一句话错误复制进 `answer_exclusion_policy`，设置
`is_exclusion_requested=true` 但 `excluded_candidate_roles=[]`，造成分析 JSON 拒绝/重试。前者约束证据来源，后者只排除 principal answer
row 的 candidate role；源码/仓库不是 candidate role，两个合同不应重叠。最优方案是同时在 analyzer SSOT 与 JSON schema 明确互斥路由：
禁止源码/当前 checkout 属于 external observation policy；只有变量、测试、生成文件、私有符号等答案成员类别进入 answer exclusion。
这只是教学/结构合同修正，不从用户文本二次硬判。

read combo 案 323s。B583 production positive：最终阶段表的四行统一为五列，StageFinalize 不再左移；B584 production positive：
provenance 报告三个 `ambiguous_symbol`，Explorer 收到 `source_operation_required=[]` 与
`request_visible_boundary_only=[analyze finalizer]`，不再为 `codrax read mode`/`stage` 搜索虚构 operation。探索由 r346 的
47 次 iteration/34 reads/499s 降为 25 次/25 reads/323s。参与者没有被系统删改，源码端点权限拆分已进入真实链路。

但该案四次成文拒绝后仍以 `runReadSchedulerLoop-->>executeStageRequest` 和
`renderFinalAnswerWithLastMileSupplements-->>BuildAgentContext` 表达正向调用。现有结构规则只把“反向且与实线 invocation
配对”的 `-->>` 认作免锚回复，却允许显式 `relation_kind=call` 的独立 `-->>` 通过；于是 typed 调用方向正确，读者看到的时序语义却是
回复线。这是 `B586-SEQUENCECALLREPLYOPERATOR1/P1-high`：对 sequenceDiagram 中显式 call owner，正向调用必须使用非回复 operator；
`-->>` 只允许作为与反向可见 invocation 配对的结构化 response，不能靠 call anchor 改写 Mermaid operator 的含义。判定只读 diagram kind、
operator、exact endpoint 和 typed relation，不读 label/prose/request，不生成或修改图。

323s 活跃模型流持续超过四分钟并由原模型正常完成，没有切旧稿、抽取半成品或系统代答。累计时长仍不是恢复信号；只有首字节超时、真实
静默、transport 断链、调用方取消或独立安全边界可进入恢复，恢复也只能发布模型已产生载体并披露。

状态：`B583=production-positive/closed`；`B584=production-positive/closed`；`B585=next`；`B586=next`；
`runner=2/2`；`human=0/2`；`finalizer-rejects=4`；`active-stream=323s/no-elapsed-degrade`；
`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`。

#### §11.10.196 B585：来源排除与答案成员排除在 analyzer JSON 边界彻底拆路

`answer_exclusion_policy` 的 candidate role 枚举描述“哪些 principal answer row 成员不应出现”；
`external_observation_policy.current_source_mode` 描述“当前源码/checkout 证据车道是否可用”。r347 中 analyzer 同时填写二者，且前者为
`is_exclusion_requested=true + excluded_candidate_roles=[]`，说明只在外部策略段举例仍不足以克服“看到否定句就填 exclusion”这一 JSON
心智捷径。

本批在 analyzer 单一 OutputFormat 与 `emit_analysis` 参数 schema 两个模型可见面使用同一边界：source code、repository、current checkout
是 evidence source，不是 candidate role；`only analyze this trace; do not analyze code` / `只分析 trace，不分析代码` 只进入
`external_observation_policy.current_source_mode=exclude`。若没有同时排除 variable/test/generated/private 等真实答案成员，
`answer_exclusion_policy` 必须省略或为 false，禁止用 true+空角色表达来源边界。反向也保持：答案成员排除不会关闭源码探索。

这是教学与 schema 路由修正，没有新增读取用户原文的 deterministic 分支，没有把中文/英文例句变成硬关键词门，也没有系统代填、改写或接受
畸形 JSON。回归同时钉住 skill SSOT 和实际工具 schema 的五条互斥语义；`go test ./internal/skill` 与 emit-analysis schema/执行定向测试全绿。

状态：`B585=implemented/pending-r348`；`source-exclusion=external-observation-only`；
`candidate-exclusion=principal-member-only`；`raw-prose-hard-gate=none`；`system-answer-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.197 B586：sequence 调用关系与回复 operator 建立同一 typed 语义合同

Mermaid `-->>` 是 dotted response/return presentation。旧 gate 已经要求无 anchor 的 `-->>` 只有在反向存在可见 invocation 时才可免检，
却又允许同向 `relation_kind=call` anchor 把独立 `-->>` 重新解释为调用。r347 因而出现“typed endpoint/direction 正确、可见线型却告诉
读者这是回复”的双真相；模型甚至可用换 operator 规避对正向调用语义的清晰表达。

本批在共享 source-diagram relation chokepoint 增加 `call_reply_operator_conflict`：任意非 Trace `sequenceDiagram` 中，只要既有
typed owner 声明 `relation_kind=call`，可见边就必须使用 invocation operator（如 `->>`，异步/丢失调用仍使用 Mermaid 对应
`-)`/`-x` 家族）；`-->>` 不得由 call anchor 改义。合法 `A->>B` 加 `B-->>A` 结构化回复继续通过，反向恰好还存在另一条 typed call
证据也不会把回复重新捕获；若确实是反向调用，模型应保留 exact endpoint 和 call anchor，只把 operator 改为 invocation 形。

该检查对所有问题 family 生效，避免 QFGeneric 成为绕过车道；它只读取 block kind、Mermaid parser operator、exact endpoint 和 typed
relation enum，不读取 message label、请求、reasoning 或终稿 prose。系统不生成图、不翻转边、不改关系、不替模型回答。QFRootCauseTrace
在入口继续完整绕过源码图合同，显式时间窗因果投影及自动补齐不受影响。

回归覆盖 typed forward call 的 dotted 冲突、generic-family 同形、合法 paired reply、无 anchor 独立 dotted fail-closed、显式反向 call
同时报告 operator 与证据问题，以及 Trace 独立权限负臂；sequence 定向测试全绿。

状态：`B586=implemented/pending-r348`；`sequence-call-operator=invocation-only`；
`paired-response=preserved`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.198 r348：预分析语义越权与概念工作流 authority 断链

r348 exact-two runner 1/2、人工 0/2。Trace runner 通过且 B585 在生产链路转正：源码排除仅进入
`external_observation_policy`，答案成员排除保持 false；显式窗、目标状态账、链上排序、实际/可消双轴、frame absent 限定、因果投影和自动补齐
均未退化。人工失败来自更早的 authority 混线：perf_triage 把 5.8ms 写成 `5800ms`，把 VerifyClass 猜为 `sync-rpc/direct cause`；
analyzer 随后把同一猜测抄入 `artifact_value_profile` 与 `diagnostic_profile.observation_summary`。后续 trace_query 已发布
`pre_wakeup_dependency`、`target_wait_for_work_authority=not_provided_by_this_seat`、
`work_completion_dependency_authority=not_provided_by_this_seat`，但早期错误载体仍影响终稿，使模型声称 app-100 等待 VerifyClass 完成。

立案 `B587-PRETRIAGEANALYZERAUTH1/P1-high`：pre-triage 模型输出只能定位原始 trace 区域；不得成为 analyzer 的 artifact scalar、诊断事实摘要、
实体身份或 hard ArtifactObservationProfile。最优方案不是删掉 trace 信息，而是按现有 Authority enum 分层：analyzer prompt 仅看 validator 语义和
pre-triage 行号 locator；非 scalar 请求丢弃 artifact_value_profile；附带 runtime artifact 时清空 analyzer 自填 observation_summary；
硬 observation profile 只消费 deterministic-validator rows。原始附件、用户显式窗/目标和后续 trace_query 均保持可用。

Read runner 因答案遗漏 Explorer/Extractor 而失败。B586 已在真实链路阻止 `-->>` 作为正向 call；但 analyzer 的非法
`discover_path + source/sink` 只被正确降格，并没有把概念工作流任务导向 canonical topology。RequiredFiles 现有 stage authority seeder 仍要求
某个 `Stage*` const 实体；当前 typed 请求已经有 `scenario=architecture_explain`、flow diagram 和 required
`stage_or_workflow` 维度，却因用户使用概念词 analyze/finalizer 而未触发，Explorer 误选同名 `dataflow.Analyze`。

立案 `B588-CONCEPTUALWORKFLOWAUTH1/P1-high`：对 required `stage_or_workflow` 维度，使用该 typed 角色加 architecture/flow/diagram 组合播种
checkout 内 canonical topology/binding authority，不再依赖同名 `Stage*` symbol。播种只要求模型读取权威源，不直接生成成员、边或答案；
最终关系仍由 checkout 验证后的 stageauthority 与模型成文共同完成。Trace family 继续绕开，跨语言普通源码图不受 Codrax 自身 authority 污染。

274s read 活跃流正常完成，不存在 4 分钟累计时长降级。状态：`B585=production-positive/closed`；
`B586=production-positive/closed`；`B587=implementing`；`B588=queued`；`runner=1/2`；`human=0/2`；
`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`；
`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.199 B587：pre-triage 只保留导航权，analyzer 事实载体收权

本批把同一 Authority 分界贯通三个消费面。Analyzer 的 PerfBundle prompt projection 清除模型产出的 meta summary/signals、frame/jank/stall/
startup 语义、layer-4 entities/intent，并把 model observation 缩到 artifact-local line locator；validator-owned observation 原样保留。
`buildAnalysisIR` 不再把 PerfBundle 派生实体合并为 analyzer entity，只有 deterministic-validator observation subject 可进入提示。

`emit_analysis` 同时执行两个 typed 归一化：附带 runtime artifact 时清空 analyzer 自填的 diagnostic observation_summary；
`artifact_value_profile` 仅在 `predicates.is_scalar_answer=true` 时保留，root-cause/mechanism 等非标量请求从后续 typed runtime rows 取值。
`BuildArtifactObservationProfile` 作为 hard routing 消费面只接受显式 deterministic authority；pre-triage summary/signal/stall/startup/entity 不再铸造成
强证据。全部判定只读 stage、typed predicate 和 authority enum，不扫用户或模型 prose，也不改写终稿。

状态：`B587=implemented/pending-r349`；`pretriage=navigation-only`；`analyzer-artifact-scalar=scalar-request-only`；
`hard-observation-profile=deterministic-authority-only`；`system-answer-authorship=none`；
Trace explicit-window/target/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.200 B588：typed 概念工作流维度稳定播种 canonical topology

旧 `stageTopologyAuthorityRequiredFiles` 只在 analyzer entities 中某个名字以 `Stage` 开头、且 repomap 将其解析为 const 时才播种 read-mode
authority files。这个条件适用于“枚举 Stage* 常量”，却漏掉更常见的概念请求：用户说 analyze 到 finalizer、analyzer 输出
`scenario=architecture_explain + predicate_axis=flow + required diagram + required stage_or_workflow dimension`，没有义务知道源码常量名。
结果 canonical binding/sequence 没被读，Explorer 可被任意同名 helper（如 `dataflow.Analyze`）吸走。

本批增加第二条完全 typed 的播种臂：上述四个 schema-validated 信号同时成立时，无需 `Stage*` entity 即把当前 checkout 的
`internal/types/enums.go`、`internal/types/stage_binding.go`、`internal/orchestrator/topology.go` 等 authority sources 放入 RequiredFiles
高优先级席。播种仅要求读取；`stageauthority.LoadReadMode` 仍需逐 AST 验证 checkout 与编译时 stage table/sequence 一致，模型仍需基于读取结果
产出证据、表格和图。系统不直接补 Analyzer/Explorer/Extractor/Finalizer 成员，不生成 precedence/call/dataflow 边，也不改终稿。

门只读 intent/family、scenario、predicate_axis、DiagramHint.required 和 RequestedAnswerDimension.role；不读用户、reasoning、答案 prose。
Trace intent/QFRootCauseTrace 显式拒绝该臂，因此时间窗、因果投影和自动补齐完全不受影响。回归既钉 helper，也钉
`analyzerRequiredFiles` 实际接线，避免未来出现“局部函数绿但生产挂点删掉仍全绿”的 M4 缺口。

状态：`B588=implemented/pending-r349`；`canonical-topology=required-read-only`；`checkout-verification=fail-closed`；
`homonymous-helper=no-membership-authority`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.201 r349：pre-triage 收权生产转正；typed 排除载体重叠与主关系脊柱遗漏立案

r349 exact-two runner 2/2、人工 0/2。Trace 案 165s，B587 production positive：perf-triage 模型仍曾把 5ms 猜成 5000ms，
但 analyzer 实际接收载体已没有 `artifact_value_profile`，`diagnostic_profile.observation_summary` 也被清空；后续显式
5.000..5.007s 查询、目标状态账、链上 VerifyClass 4.600ms 与 runnable 0.800ms 排序、实际占时/规则可消双轴、frame absent
限定和因果投影均完整保留。人工失败来自模型自由叙述仍把部分重叠的 runnable/VerifyClass 区间说成先后，并在 frame evidence absent 时
把目标 running 直接定为未完成帧表现；typed context 足够，禁止系统扫描或替换终稿。

B585 教学修复没有完全闭环。analyzer 第一次把当前源码排除同时铸成带多个 member role 的 `answer_exclusion_policy`，后两次又给出
true+空 roles；最终虽通过，却仍把唯一一句 `只分析这份 trace，不分析代码` 同时用作 source exclusion 和 function/method/type/file
answer exclusion。立案 `B589-SOURCEANSWERQUOTEOVERLAP1/P1-high`：若两个已验证 typed carrier 的唯一 provenance 是完全相同的
current-source boundary quote，则答案成员排除没有独立依据，应在 JSON 边界软化；存在任何独立 answer-exclusion quote 时必须原样保留。
判定只能比较 typed quote 的精确字节，不能扫描请求或答案词义。

Read 案 332s，B588 production positive：canonical binding/topology 文件进入 RequiredFiles，最终正文与表格正确列出
Analyzer/Explorer/Extractor/Finalizer 及各自输出。finalizer prompt 也明确携带三条 checkout-verified precedence relation，模型 repair
阶段已识别“主图应画完整 spine”，但终稿仍只画 Orchestrator/helper supporting calls，整条 requested relation spine 在图中消失，runner
仍签 PASS。立案 `B590-REQUESTEDPRINCIPALSPINE1/P1-high`：required diagram 已由 typed provider 选出 principal relation spine 时，至少一个
模型成文图必须在同一 connected diagram 内完整覆盖全部 selected relations；支撑图可追加但不能替代主脊。系统只校验模型可见边与 typed
relation owners，不生成图、不补边、不改答案。

332s 活跃流正常完成；4 分钟累计时长没有触发恢复，今后也不得作为降级条件。只有首字节超时、真实 byte stall、transport 断链、调用方取消
或独立安全边界可进入恢复；恢复只能发布模型已产生的载体并明确披露，不能用系统答案代替。

状态：`B587=production-positive/closed`；`B588=production-positive/closed`；`B589=next`；`B590=queued`；
`runner=2/2`；`human=0/2`；`raw-prose-hard-gate=none`；`system-answer/diagram-authorship=none`；
Trace explicit-window/causal projection/auto-supplement=`unchanged`；`active-stream=332s/no-elapsed-degrade`。

#### §11.10.202 B589：同一来源边界 quote 不再双重铸造答案成员排除

本批没有继续堆叠中文/英文教学例句，而是在两个 typed carrier 都完成各自 provenance 校验后建立精确消歧。仅当
`ExternalObservationPolicy.ExcludesCurrentSource()` 成立，且 `answer_exclusion_policy` 的全部 `source_quotes` 都与该
current-source exclusion quote 精确相等时，后者被软化为空：同一句来源边界只能证明“当前 checkout 不参与证据”，不能同时证明
function/method/type/file 等答案成员都应消失。

只要 answer exclusion 携带任一不同的独立 quote（例如来源边界为“不分析代码”，答案边界另有“不包含测试”），整个成员排除合同原样保留；
系统不猜每个 role 对应哪句，也不做部分删减。实现只比较 schema-validated typed quote 字符串和 policy enum，不读取 raw request，不扫描
reasoning/终稿，不按语言关键词分类。模型仍负责分析和回答；系统只阻止一个 provenance 被错误复用为两个不同权限合同。

生产接线 pin 直接复现 r349 的多 role 形，并与 B587 runtime-artifact normalization 同轮执行；独立 quote 负臂确保真实双重边界不被吞掉。
定向 `internal/tool` 测试全绿。

状态：`B589=implemented/pending-r350`；`source-boundary/member-exclusion=exact-provenance-split`；
`independent-answer-exclusion=preserved`；`raw-prose-hard-gate=none`；`system-answer-authorship=none`；
Trace explicit-window/target/causal projection/auto-supplement=`unchanged`；`active-stream-fixed-time-degrade=forbidden`。

#### §11.10.203 B590：required 主关系脊柱建立 provider 驱动的可见完整性合同

B588 已把 checkout-verified read-mode stage precedence 送入 finalizer prompt 与单边 relation authority，但旧 validator 只回答“模型画出的边是否有证”，
不回答“typed request 明确要求的整条 principal spine 是否真的出现在图中”。因此 r349 的 supporting call 图每条边都可能各自合法，却完全替代
Analyzer→Explorer→Extractor→Finalizer 主脊。继续加教学无法保证模型最终选择，系统直接补图又会越过模型成文权。

本批在同一 stageauthority provider 上增加 request-scoped completeness 消费面：仅当 typed request 要求 diagram，且 provider 根据
checkout + typed request/evidence 选出非空 contiguous precedence span 时，要求至少一张模型成文图同时满足三件事：全部相邻 relation 都有
可见 Mermaid edge；每条 edge 有 `relation_kind=precedence` typed owner；相邻关系共享同一个可见中间 node，构成一条 connected spine。
把三条边拆到多张图、用多个同标签 node 伪装断开的三段、仅画 Orchestrator/helper supporting calls 都不能履约。其它 grounded 支撑图可完整保留。

这不是“所有图必须完整”的全局硬化：未选出 request-scoped provider spine、optional diagram、普通跨语言源码图、call/dataflow/type relation 图均
保持原子边取证合同；required diagram block 本身缺失仍由既有 block coverage 单独诊断。Trace family 在 provider 和 checker 两个入口均显式绕开，
显式时间窗、因果投影、自动补齐与链上根因权限不变。

pre-emit 与 post-finalizer 共用同一 checker。失败提示只列 provider 选出的缺失相邻关系，要求模型保留支撑图并自行补全 principal diagram；
系统不插边、不改 relation kind、不替换答案。回归钉住完整单图通过、单边/支撑图失败、跨图分片失败、同标签不同 node 的可见断链失败、
Trace bypass，以及 pre-emit/post-finalizer 两个生产挂点。

状态：`B590=implemented/pending-r350`；`requested-principal-spine=one-connected-model-diagram`；
`supporting-diagrams=preserved`；`provider=checkout-verified`；`raw-prose-hard-gate=none`；
`system-answer/diagram-authorship=none`；Trace explicit-window/causal projection/auto-supplement=`unchanged`；
`active-stream-fixed-time-degrade=forbidden`。

---

## §40 MERGE-AUDIT-7 增量审计(2026-09-02):1,311 笔(08-09→09-02)= PR23 根因 sidecar + 答案 patch 车道 + evaluator/analyzer/门链/write/转换/data/图表八大波

**范围**:ad462a6ce(CHATFIX-1)..HEAD 非本席合入 **1,311 笔**(三周半;81 个新非测试 Go 文件,含新包 `internal/analysis/tracefinding`、`internal/stageauthority`)。**方法**:12 主题读者+逐条 2 席否证(64 agent,wf_c54dc652-cf8,多处 go test -overlay 可执行判别)+全仓基线实测+裁定敏感项主会话亲验;只审计不修码。产出:**18 确认(9 高)+2 存疑+12 低+6 项指控被否证席驳回;51 项核验通过**。

### §40.1 基线与核验总评(正面)

- **main 绿**(本席实测零失败);orchestrator.go 9132/9135(ratchet pin 在案);trace_query Description 两次改动均带 EVOLUTION RECORD;`causal_token_registry.go`/provenance/thread-incarnation 零动;**我方全部裁定面完好**(crown 前缀+帧因果未证限定注/CROWNCAL 臂/TWODIM 账/closed-matrix/CHATFIX-1 五车道/思考语言机器/退避探针;`首要可消除` 生产零命中)。
- **PR23 自审五条补救(A–E)全部在代码属实**:可选 sidecar 永不拒答案;模型只选 enum 候选 ID、运行时绑全部语义字段;无自动 PrimaryCause;单一加锁成功尾声;死 `TraceFindingRequired` 已删。附加核验:候选准入精确 typed 信号(Rank≤0/target_self_state/context_only/口径旁栏排除)、证据打包不丢引用、默认 sidecar 带 typed unavailable 信封。
- **我方 §11 三项关账声称属实**(三主题独立核验):M6-U3-1 全部关系臂走 `diagramRelationEdgeHas…` 单点投影助手;M6-U3-2 `CallChainEndpointCompatible` 零残留;M6-U1-1 错算式 e2e 真实存在(`82.1 + 9.1 = 91.2ms` + 禁评判词臂)。
- 其他核验通过:S6-1 类 grep 零复发;L3/L5/L6 pin 在;write 权威波把模型 outcomes 移出权威(方向正确);`llm_stream_activity` 为被动遥测不削弱 W1/W2;stageauthority 只在被分析仓=codrax 时激活;eval 提交无降杆(B1190 写评步预算 24 有据);6 项 reader 指控被驳回(含"sidecar 与主根因加冕不对账"——两者均模型序/席位级同源,非缺陷)。

### §40.2 高危确认(9 件)

| # | 位置 | 问题 | 亲验/复现 |
|---|---|---|---|
| **V12-1 ★裁定推翻** | `tracequery/query.go:19110`(332fabb21 B829/B830,08-15) | `semanticProjectionIsRelationOnly → publishedEffectiveMs=0`:非目标宿主线程的语义 span 即使持 typed 唤醒边(`host_wakeup_edge_pre_span`)或链窗交集也计价归零、rank=0、判"仅关系凭证·优化项非根因"——**直接反向 §29.88.1 R3**(宿主对目标有窗内 typed 唤醒边且 span 在边前→算链上席)与 **§29.88.2 R4 终判通则**(边=凭证/边前=有效/边后=解除,全状态族唯一精确判据)。提交未引任何用户裁定,未走 registry §7.2.1 车道变更协议(registry 文件未动但语义在 query.go 改) | **本席亲验**(代码+裁定原文逐字对照);tieba 哨兵(ZeusThreadPo VerifyClass 0.285ms 边前)可 eval 复现 |
| **V1-2 ★T3-1 类** | `tracefinding/root_cause_report.go:85` + `agent/trace_finding_contract.go:45` | sidecar 契约顶棚取 `authority.CausalUnproven‖FrameFlowUnproven`——**会话 ANY 聚合**(T3-1 裁定已否决的信号,crown 面已改席位级);且 `.root-causes.json` 无任何限定/状态字段,同一运行 Markdown 头行带「（帧因果未证）」而 JSON 发布无限定根因(status=available 是交付态非因果态)。否证席指出:修法若绑 Decision.Status 会把 ANY 信号再引入公开工件——**必须用席位级信号**;v2 wire 加附加字段需版本兼容裁定 | 否证席 overlay 复现 + 本席核 ceiling 来源 |
| V4-1 | `emit_analysis.go:2812`(c8354ec0a) | **T3-2 类第五例**:必需流程图"遗漏参与者"硬臂用无界 substring 锚定(`Reader`⊂`FileReader` 即硬拒),同一校验器的兄弟臂两周前刚因同样 substring 导致"不可能重试环"改为全词面别名键 | 静态(同文件两臂对照) |
| V5-1 | `run_tests_source_contract.go:45` | **静态源码行子串铸行为契约满足**(证明车道):按 analyzer 的 apply 前 evidence_ref 行号读 apply 后文件,`Contains(expected)` 即记 `source_contract_refs satisfied` 并被 hard-required 契约消费为完整观测权威;插行位移后任意含 '0' 的行即"满足" | 静态 |
| V5-2 | `run_tests_worktree_effects.go:281` | 追踪路径漂移校验门**无 typed escape、无 unverified 车道**:`cargo test` 重写 Cargo.lock 即 verification_side_effect 硬失败→replan(§1.6 违例,M5-S3-1 类) | 静态(Rust runner 无 --locked 逐点核) |
| V6-1 | `hitraceconv/streamerdb_export_raw_ftrace.go:290` | 源 block 族接管按**class** 抑制整个 DB `block_storage` 类,但源车道只治理 7 个 block_rq_*/bio_* 名——ufshcd_/mmc_request_/scsi_dispatch_cmd 行随之静默消失(30dc7eeba 前 MMC 精确端点可见) | 静态 |
| V6-2 | `hitraceconv/source_raw_visibility_recovery.go:304` | 可见性载体以**原事件名**发布(`sched_migrate_task: codrax_source_raw_visibility/v1 …`),tracequery 窗口索引在 parse 前跑子串键完整性审计→成千载体被判畸形语义行 | 静态(审计序逐点核) |
| V7-2 | `orchestrator/accepted_closure_retry_authority.go:14`(476129c6d) | 已接受闭包重试权威臂读**永不清零**的 RetryState(唯一写者 finalize 契约失败;ResetRetryState 零生产调用)→回溯后探索者的**新鲜**完成决定被旧 RetryState 否决(完成门权属+§1.6 缺代次绑定) | 静态 |
| V9-1 | `dataquery/decision_lineage.go:19` | 一对多派生行共享源行 ledger 身份(`items.csv#1`)→ dedupe 静默折叠同值兄弟贡献 + 一致性硬门见同身份 include/exclude 冲突误拒;"每 item N 个 tag 计 1"错数发布无违规 | 静态 |

### §40.3 中危确认(9 件)

V1-5 死的 legacy trace_finding 车道仍对未教字段整答案硬拒(应隔离/记日志);V2-1 两阶段暂存快照在归一化前(patch 容差/模型提交项被后继代次看不到);V2-2 关系修复租约在事务提交前消费(持久层拒绝后模型无租约重试);V2-3 无效根因选择器降为 WARN 并 SUCCESS 返回(精确 typed 不匹配不应静默,PR23-A 不拒答案≠不告知);V3-1 同一目标态账在 finalizer prompt 内三种"不可中断等待"口径(D-only vs D+IO)并存;V4-3 slash-pair 规范化把模型**单一**实体 X/Y 拆两个再判"两个分别发射";V4-4 required_files 维度归属硬门对合法未知归属 fail-closed(唯一逃逸=降置信);V5-3 verify 失败契约退役对**任何**失败种类(构建失败/超时)触发,静默丢软契约;V5-4 兄弟 contract_refs 门解析 id 用不同集合(rebase 前/后)。

### §40.4 存疑(2)

V1-1(高,1/2 分歧)sidecar 证据句统一写「链上有效影响为 X ms」且 impact_seconds 无口径字段——窗口投影席被冠以"有效"(CROWNCAL 拒造数纪律的反向);分歧点=是否属"口径披露缺失"而非"造数"。V1-4(中)多工件投影集折成单 roster/sidecar 无工件标签。

### §40.5 低危(12)与继承债

V2-4 patch 校验逐项首错;V3-3 后 emit 顾问车道每轮一条(最多 4 轮);V4-5 prompt 出现未定义行话 "LOW-MIND RULE";V6-4 可见性覆盖把 withheld 标 not-applicable;V7-4 硬图归一化键在 analyzer 看不到的 typed bool;V7-5 ratchet 靠压缩既裁 §29.60 注释达标(文件贴线 9132);V8-7/V11-3 模型面拒绝文案与新 finalizer prompt 泄内部管线词(agent 侧渲染器在 glossary lint 外);V9-4 派发前投影门对草稿契约校验而执行对另一契约;V10-4 class diagram 归一化静默改写模型 typed 锚方向;V11-2 trace_query 新 `patterns` 参数未同步 tracediag Step schema(R2' 类);V12-3 同事账本新自报残留(B1560 P1 节点 ID 当显示标签/B1558 P1 待回放/B1548 默认 sidecar 空 roster 而长答案有候选/B1541/B1546/B1554 P2 Rust/Swift/Cangjie callee 抽取/PR23 §4.1 版本化决定)。

### §40.6 处置序与裁定请求(未动码)

1. **裁定请求 ①(V12-1)**:B829/B830 语义 span 边前份额归零是否成立?两选一——**恢复 R3/R4 计价**(边前份额入链上有效归因,仅无边/边后归 ◇)并走 §7.2.1 协议落 registry 判词;或**明示改裁**取代 §29.88.1/.2(须落账为新裁定,不能由修复提交隐式覆盖)。本席建议恢复:R4 是全状态族终判通则,单为语义 span 开例会重开"边=凭证"的整套一致性。
2. **裁定请求 ②(V1-2)**:sidecar 顶棚改用**席位级**帧因果信号(与 crown 面同源 `runtimeTraceProjectionLeadFrameCausalityUnproven` 索引),wire 上以 v2 追加字段 `causal_qualifier`(append-only 兼容,NEXTINFO 尾追加裁定同构)承载;是否照准。
3. 高危批:V4-1(与兄弟臂同源全词面解析)/V5-1(观测型契约只认探针/项目测试见证,源码行只治 file_layout)/V5-2(漂移按 owner 分类进 advisory 披露臂+lockfile --locked)/V6-1(按名接管非按类)/V6-2(载体用保留名发布)/V7-2(RetryState 绑代次或消费即清)/V9-1(派生行身份=locator+展开序数)。
4. 中低危随批;V12-3 继承债并入排期。

**方法学**:①同事修复合规连续第四轮 100%(PR23 五条+我方 §11 三项+既往全部核验通过,51/51),**新缺陷集中在新子系统与新硬门**(tracefinding/patch 车道/写校验权威/转换恢复)——审计资源继续向新功能面倾斜;②**裁定推翻类首次出现**(V12-1):修复提交在无用户裁定引用下改写终判通则语义——建议同事流程增加"触碰账本已裁语义必须引用裁定或申请改裁"检查;③T3-2 类第五例(V4-1)与 §11.4 提议的 census tripwire 尚未落地——单点助手已建但"每个硬臂必过同一解析权威"的结构 tripwire 仍缺,应作为泛化解优先落地(而非逐例修)。

### §40.7 逐项细化 ① V12-1:非目标语义 span 边前份额计价归零(裁定推翻件)

**定性**:B829(332fabb21,08-15)/B830 在 `tracequery/query.go:19110` 引入 `semanticProjectionIsRelationOnly`(basis∈{host_wakeup_edge, semantic_chain_interval_relation})→`publishedEffectiveMs=0, rank=0, context-only`。这不是实现缺陷,是**语义裁定被修复提交隐式改写**:§29.88.1 R3 明文"宿主对目标有窗内 typed 唤醒边且 span 段位于边前→该部分算链上席;跨边按边界二分";§29.88.2 R4 明文"边=凭证/边前=有效/边后=解除为**全状态族**(含语义 span)链上唯一精确判据,终判通则"。提交与其账本 §123.834 均未引用任何用户裁定。

**零 LLM 确定性复现**(`--tracediag`,tieba 哨兵 `eval/fixtures/real_traces/donghu_tieba_frame.systrace`,pid 59566,窗 34579.495000..34579.497500;脚本 collect_v12_1_tieba_narrow.yaml):
```
items[2]:  T7@ZeusThreadPo-61839 runnable 0.095ms fully pre-edge: … (edge=credential, pre-edge=effective,
           post-edge=released; latest credential edge 34579.496810, via=direct)                 ← R4 计价
items[16]: class verification span "VerifyClass …LacUtils" … pre-edge share … for 0.285ms;
           effective_impact=0.000ms; … (edge=relation credential; semantic completion/delay
           binding=unproven; … edge at 34579.496810, via=direct)                                ← R4 未计价
```
**同一宿主、同一条边、同一窗**:状态席按 R4 计价,语义席按 B829 归零——一份报告内两套判据。B829 自己也保留了 `host_wakeup_edge_pre_state` 对 runnable/D-IO 按边前份计价,与 R4"一体适用"直接矛盾。

**同事论据与辨析**:§123.834 称"r506 目标在 5.005000s 被唤醒而 VerifyClass 5.005400s 才结束,'目标等待语义完成'物理不可能"。此论据把 R4 的**凭证语义**(边前占用=推迟了边的到达=有效延误)偷换为**机理证明**(目标等待 span 完成)。R4 从不要求机理证明——closed matrix 护栏正是"机理主张只来自链/阻塞证据,计价按凭证";且 R3 已明文处理跨边形(按边界二分:边前份计入、边后份 ◇)。r506 案例在 R3 下的正确答案是:5.000400..5.005000 的边前份计入、5.005000..5.005400 边后份归 ◇——而非整段归零。

**泛化根因类**:「同一凭证在不同状态族走不同计价规则」= 单源判据被按族碎片化;此类一旦开例,every 新族都会再问一次"要不要机理证明"。**泛化解不是逐族修,而是把 R4 落成结构不变式**:

1. **判据单源化**:`host_wakeup_edge_pre_*` 全族(state/span/family/remainder)共用一个 pre-edge 计价函数(输入=段区间+最近凭证边时刻,输出=边前份/边后份),禁止任何 basis 分支单独归零;跨边二分只在此单点实现;
2. **registry 判词**:在 `causal_token_registry.go` 为语义 span 家族登记"pre-edge share = priced(R3/R4)",并写入 §7.2.1 变更协议要求——任何 basis→零值的映射变更必须引用裁定编号;
3. **结构 tripwire**:census 测试——对每个 on_chain_basis∈host_wakeup_edge* 的发布行,若 pre-edge 原始份>0 则 effective>0,除非 registry 显式登记豁免(带裁定引用);tieba 哨兵双席(items[2]/items[16])作为 e2e 一致性 pin(两席必须同规则);
4. **机理层**保持不变:closed matrix 双面仍教"机理主张来自链/阻塞证据",树面可继续显示"语义完成机理未证"作为**披露**而非计价依据(披露≠清零,与 TWODIM/排除≠消失同构)。

**需裁定(用户)**:A=恢复 R3/R4 计价(本席建议,零裁定推翻,与 B829 自身对状态席的处理一致);B=明示改裁——若采纳"语义 span 边前份不计价",须作为新裁定落账并同步改写 R4 的"全状态族"表述,且状态席同样适用(否则族间不一致仍在)。任一选项都必须走 §7.2.1 协议。

**验收 pin**(施工时):①tieba 哨兵 e2e:items[2] 与 items[16] 同规则(A 下 items[16] effective=0.285;B 下 items[2] 亦归零);②r506 跨边形二分 pin(边前 4.600 计入/边后 0.400 ◇);③registry 判词 golden;④Description golden 走 UPDATE RITUAL 反向修订 B829 教学句。

### §40.8 逐项细化 ② V1-2:PR23 sidecar 的帧因果顶棚取会话 ANY 信号且 wire 无限定字段(T3-1 类复发)

**定性**:`agent/trace_finding_contract.go:45-48` 以 `authority.CausalUnproven || authority.FrameFlowUnproven`(`answerDocRuntimeTraceGuidanceView` 对全会话 trace_query 结果做 OR)铸 contract 顶棚;`tracefinding/candidate_compiler.go:189` 派生每候选 `Decision.Status`;但 `boundRootCauseItem` 只拷 Category/identity/ImpactSeconds/Evidence,`TraceRootCauseItemV2` 无任何限定字段(否证席 overlay 实证:顶棚 unproven 下 wire 零处出现 unproven/status/qualifier)。同一运行 Markdown 头行「（帧因果未证）」而 `.root-causes.json` 无限定。

**泛化根因类**:§7.3 裁定(2026-08-04)把 crown 面的触发从"会话 ANY-sticky"收窄到席位级(`buildRuntimeTraceProjectionSeatAuthorityIndex`,按 unproven 结果的 EvidenceID 键控),但裁定只修了**一个消费者**;信号本身未被定性为"不得作硬决策/发布输入",于是新消费者(PR23 契约)再次直接消费会话 ANY。**泛化解=把裁定表述为信号级禁令**:

1. **单一席位级权威提供者**:把 `buildRuntimeTraceProjectionSeatAuthorityIndex` 提升为 types 级 `TraceSeatCausalAuthorityIndex(ObservationLedgerInput)`(EvidenceID→unproven),crown 面与 tracefinding 契约同源消费;候选编译按 `node.EvidenceID ∪ MergedEvidenceIDs` 查索引得每候选 `CausalQualifier ∈ {proven, frame_unproven}`——**席位级、精确、与头行限定注同一真值源**;
2. **会话 ANY 信号降格为 advisory-only**:`answerDocRuntimeTraceGuidanceView.CausalUnproven/FrameFlowUnproven` 只允许喂 prompt 提示行(现有"Runtime causal ceiling hint"),禁入任何硬门/发布面;结构 tripwire=census 测试列出该两字段的全部读者,白名单只含提示渲染器;
3. **wire 追加字段**(v2 兼容,append-only,同 NEXTINFO 尾追加裁定):`TraceRootCauseItemV2.CausalQualifier string json:"causal_qualifier,omitempty"`,值域闭集 `proven|frame_unproven`;摘要句在 frame_unproven 时追加「（帧因果未证）」与头行同词;roster 教学同步(每候选行带限定,模型选择时可见);
4. **status 语义澄清**:`status=available` 是交付态,文档与字段注释明示"非因果证明态",避免读者误读。

**需裁定(用户)**:v2 wire 追加 `causal_qualifier` 是否照准(PR23 §4.1 要求公开 schema 扩展须版本化决定;append-only 附加字段与既有 next_info 尾追加裁定同构,不破 v2 消费者)。

**验收 pin**:①契约 e2e:席位级索引 unproven 候选→ wire 带 `causal_qualifier=frame_unproven` + 摘要含限定注;proven 候选字节不变;②会话 ANY 读者 census 白名单 tripwire;③头行限定注与 sidecar 限定字段同源一致性 pin(同一 fixture 两面必须同真值)。

### §40.9 逐项细化 ③ V4-1:必需流程图"遗漏参与者"硬臂用无界子串锚定(T3-2 类第五例)

**定性**:`emit_analysis.go:2812` 硬臂对每个 AnalyzerHints 实体用 `sourceQuoteAnchoredInCurrentRequest`(`strings.Contains` + 去标点 Contains,:6162-6168)判"被 relation_scope_quote 锚定却无参与者行"→硬拒;同文件同校验器的兄弟臂(:2972-2986,11ceec1d9,08-16)两周前**恰因同样子串语义制造"不可能重试环"**而改为 `sourceQuoteExplicitlyMentionsTypedEntity`(全词面别名键)。新臂(c8354ec0a,08-21)把刚退役的语义又写回来:`Reader`⊂`FileReader` 即硬拒,模型按教学逐字拷贝实体反被惩罚。

**泛化根因类**:「同一硬车道内多臂各自选择身份比对权威」——T3-2 类五连发的共同结构:单点解析助手已存在,但没有任何东西**强制**新臂使用它。§11.4-1 已提出 census tripwire,至今未落地,故第五例出现。**泛化解=区分两类比对并各设唯一权威+census 强制**:

1. **两类语义分离**:①*引文锚定*(quote 是否逐字出现在原请求里)——子串是正确的精确判据(:5800/:6061/:6087/:6098 属此类,保留);②*身份成员判定*(实体是否被引文"点名")——必须走 token 边界/别名键权威(`sourceQuoteExplicitlyMentionsTypedEntity` / `RawRequestExplicitlyMentionsEntity`)。:2812 与 :2669/:2681(参与者裁剪归一化器,V4-2 已驳回为软形但同用子串)属②类;
2. **单点权威**:新增 `entityNamedInQuote(quote, entity)` 作为②类唯一入口(内部=现有全词面别名逻辑),`sourceQuoteAnchoredInCurrentRequest` 改名 `quoteVerbatimInRequest` 并在注释与函数名上标注"仅用于引文锚定,禁用于实体身份";
3. **census tripwire(泛化,一次解决五连发)**:测试静态扫描 `emit_analysis.go`/`answer_document_diagram_evidence.go`/`pre_emit_check.go` 中所有硬拒臂函数体,凡把实体/参与者/符号名与引文/标签比对者,必须调用 ②类单点(或登记豁免+理由);新增比对而未过单点→红。这与 §11.4-1 的 `diagram*EdgeHasTypedEvidence` census 合为同一 tripwire 家族;
4. **教学同步**:硬臂拒绝文案指向"逐字拷贝实体"的教学不变——修的是判据,不是模型。

**非过拟合声明**:不为 `Reader/FileReader` 加特例,不加停用词表;修法对任何前缀/后缀包含关系(`Parser`/`JSONParser`、`Cache`/`CacheStore`)与任何语言词面一体生效。

**验收 pin**:①`Reader`+`FileReader → Parser` fixture:实体三、参与者二、scope 引文含 FileReader→Parser → 不拒(红→绿);②`Reader` 被引文**独立点名**(「Reader 与 Parser 的数据流」)且无参与者行 → 仍拒(保真);③census tripwire 自身红检(临时把 :2812 改回子串 → 红);④兄弟臂回归不变。

### §40.10 逐项细化 ④ V5-1:apply 后源码行子串铸行为契约"满足"(证明车道越界)

**定性**:`run_tests_source_contract.go` 按 analyzer 的 **apply 前** `evidence_ref` 行号读 **apply 后**文件,`Contains/Equals(expected)` 即记 `source_contract_refs status=satisfied`,该记录被 `verification_proof_profile.go:995`/ledger contractCovered 当作 hard-required 契约的完整观测权威。两重越界:①行号陈腐(插行位移后任意行);②`observable/invariant` 契约的 `expected` 按其 schema 是**运行时值**,字节存在≠行为发生(`timeout = 10` 含 '0' 即"满足" `retries contains 0`)。

**泛化根因类**:「契约种类→见证种类」矩阵缺失——一条源码文本见证被授予对所有种类契约的放行权。这是 §29.21 证明车道红线在 write 域的同构:**静态存在不得铸运行观测**。

**泛化解(矩阵化,非逐种打补丁)**:
1. **typed 见证矩阵**:每种 `WriteBehaviorContractKind` 声明允许的见证来源闭集——`file_layout`→源码/文件系统见证;`observable/invariant/stdout/status_code/command_result/exception/output_path`→仅探针/项目测试/命令执行见证;矩阵为 types 级表,ledger 消费与 emit 教学同源(R2' 同步);
2. **行号绑定**:源码见证只在 PatchEffect 能把 `evidence_ref` 映射到 apply 后行(hunk 行表重映射)时成立;映射失败→`unresolved`(披露),永不"satisfied";
3. **降级不静默**:被矩阵拒绝的源码见证仍保留为 advisory 记录(`source_text_present`),供 replan 提示,但不计入 covered;
4. **tripwire**:矩阵表驱动的 census 测试——凡 `VerificationConfidenceRecord` 生产者写 `status=satisfied` 必带 `WitnessKind`,且 (Kind, WitnessKind) ∈ 矩阵,否则红。

**验收 pin**:①插 3 行位移后原行号含 '0' 的 observable 契约 → unresolved 而非 satisfied(红→绿);②file_layout 契约源码见证保持 satisfied;③矩阵外组合 census 红检。

### §40.11 逐项细化 ⑤ V5-2:追踪路径漂移校验门无 typed escape(§1.6 违例)

**定性**:`attachVerificationWorktreeAudit` 把校验期间**任何**追踪路径变更判 `Passed=false / FailureKindVerificationSideEffect`,该种类与 tests_failed 同路由(replan)、不在 accept_unverified 名单、无配置、无白名单;Rust runner 裸 `cargo test`(无 `--locked`),依赖变更必写 Cargo.lock → 合法计划必失败。同类:formatter 无语义 diff、声明目标的代码生成输出、`go mod tidy` 型 go.sum 刷新。

**泛化根因类**:「副作用=失败」的二值门,缺少**副作用 owner 分类**这一 typed 维度(M5-S3-1 同类:新硬门无 escape 车道)。

**泛化解**:
1. **副作用分类闭集**(typed):`dependency_lockfile_refresh`(Cargo.lock/go.sum/package-lock/Gemfile.lock… 由 runner 声明其 lockfile 集)、`formatter_no_semantic_diff`(格式化器可逆性验证:格式化 apply 前版本得到相同结果)、`declared_generated_output`(计划/目标声明的生成物路径)、`unclassified_tracked_drift`;
2. **门按类分流**:前三类→`Passed` 保持 + `verification_tracked_side_effect_disclosed` advisory 记录(路径与类别披露进报告);仅 `unclassified` 保持硬失败;
3. **runner 级预防**:每个 runner 声明"锁定标志"(cargo `--locked`、npm `--ignore-scripts`/`ci`、go `-mod=readonly`)作为默认,减少副作用发生;
4. **tripwire**:任何新增 `FailureKind*` 路由到 replan 的种类必须在 `accept_unverified` 或 typed escape 表中显式登记(census 红)。

**非过拟合声明**:不对 Cargo.lock 单独放行,分类按 runner 声明的 lockfile 族与可逆性判定一体生效。**验收 pin**:①Cargo.toml 加依赖计划:cargo 重写 Cargo.lock → Passed + advisory 披露;②非声明文件被测试改写 → 仍硬失败;③runner 锁定标志 pin。

### §40.12 逐项细化 ⑥ V6-1:源 block 族接管按 class 抑制整个 DB block_storage 类

**定性**:`streamerdb_export_raw_ftrace.go:290` `sourceBlockSupersedesDB && class=="block_storage"` 跳过 DB 行,但 `traceDBRawFtraceClass` 把 ufshcd_*/mmc_request_*/scsi_dispatch_cmd* 也归 block_storage,而源车道只治理 7 个 block_rq_*/bio_* 名(`traceDBRawBlockTargetNames`)。MMC/UFS/SCSI 精确端点行随之静默消失。

**泛化根因类**:「接管粒度(class)粗于治理粒度(name)」——两个词表各自演化而无结构绑定;任何"A 车道完整→抑制 B 车道"的规则,其抑制集必须**等于**A 的治理集。

**泛化解**:
1. 接管谓词改为**按名**:`sourceBlockSupersedesDB && sourceBlockGoverned(raw.Name)`,治理集直接引用 `traceDBRawBlockTargetNames()`(单源),不再经 class;`sourceExactSupersedesDB[class]` 同形改为按名族(通用化:每个源族登记自己的治理名集);
2. **结构 tripwire**:census 测试——对每个"superseded_*"rowReason,其可能抑制的名集 ⊆ 对应源族治理集,否则红(词表分叉即红);
3. 混合 fixture:block_rq_* + mmc_request_*/ufshcd_* 共存,源族完整时非治理名仍发布。

**验收 pin**:上述混合 fixture 红→绿;既有 P0-a3 E1 MMC 精确端点用例回归。

### §40.13 逐项细化 ⑦ V6-2:可见性载体以原事件名发布,被 tracequery 前置完整性审计判畸形

**定性**:`traceDBSourceRawVisibilityEventName` 对任何"安全"名直接透传,载体行形如 `sched_migrate_task: codrax_source_raw_visibility/v1 …`;tracequery 窗口索引在 parse 前跑子串键的原始候选预筛(`schedulerIntegrityRawCandidate`/`cpuInputRawCandidate`/`interruptEndpointRawCandidate`…)命中原事件名→以语义行审计→成千"畸形行"。

**泛化根因类**:「载体冒用被载事件的身份名」——载体是元数据行,却穿了语义行的外衣;下游任何按事件名索引/审计/统计的消费者都会误认。这是命名空间问题,不是各消费者的排除问题(逐消费者排除=打地鼠)。

**泛化解**:
1. **保留名发布**:所有载体统一 `codrax_source_raw_event: codrax_source_raw_visibility/v1 event_name=<原名> …`(原名进 payload 的 typed 字段/b64),名空间隔离一次到位;
2. **消费者零改动**:parse 前预筛按事件名天然不命中保留名;既有的 parse 后丢弃 `EventSourceRawVisibility` 逻辑保留为二道;
3. **tripwire**:发射面 census——载体行事件名 ∈ 保留名集(单一常量),任何新载体族必须复用;tracequery 侧 pin:含载体行的窗口索引审计计数为 0。

**验收 pin**:OpenHarmony sched 类目 fixture(含 sched_migrate_task 载体数千行)→ 窗口查询零畸形审计;原名可从 payload 恢复(诊断报告用)。

### §40.14 逐项细化 ⑧ V7-2:已接受闭包重试权威读永不清零的 RetryState

**定性**:`acceptedClosureHasActiveExploreContractBacktrack` 以 `RetryState.ActiveViolations≠∅ ∧ LastPrimaryOwner==explore` 否决"已接受闭包自动完成";但 RetryState 唯一写者是 finalize 契约失败时的 `populateRetryState`,`ResetRetryState` 零生产调用,回溯(`ResetForFallback`)也不清——回溯后探索者的**新鲜**完成决定被上一轮的陈旧 RetryState 否决(完成门权属:非致命必须尊重模型)。

**泛化根因类**:「跨代次载体无代次绑定」——一个硬臂读取的状态没有与它要仲裁的决定属于同一代次的证明(§1.6 同族:硬门读无生命周期的载体)。

**泛化解**:
1. **代次绑定**:RetryState 记录 `Generation`(finalize 尝试序)+ `BacktrackEpoch`(回溯计数);硬臂只在 `retry.BacktrackEpoch == 当前 epoch` 且探索窗尚未在本 epoch 产生新完成时才生效;
2. **消费即清**:回溯重派发探索窗时,把 RetryState 快照移入 `ConsumedRetryState`(供提示渲染),活动槽清空——"一次失败只否决一次";
3. **通用规则落 tripwire**:凡被硬臂读取的 Mutable 载体必须有显式生命周期写者对(populate/reset)且 reset 有生产调用(census:零调用的 Reset 方法即红);
4. 非致命尊重模型:该臂改为**只对同代次**生效,其余场景交给模型的完成决定。

**验收 pin**:①finalize 失败→回溯→探索者重新 emit 完成 → 自动完成不被陈旧 RetryState 否决(红→绿);②同代次内(尚未重探索)保持否决;③Reset 零调用 census。

### §40.15 逐项细化 ⑨ V9-1:一对多派生行共享源行 ledger 身份

**定性**:`actionRecordLedgerIdentity` 在 `item_id_field` 为空时以继承的 `_source_locator` 作 ItemID;`runExpandRecords`(一对多)对每个派生行 `stampActionRecordOriginFields` → 所有兄弟行同身份(`items.csv#1`)。后果:①`DedupeContributionRecords`(键含 ItemID)静默折叠同值兄弟贡献——"每 item N 个 tag 计 1";②`ValidateContributionDecisionConsistency` 见同身份 include+exclude 冲突误拒。

**泛化根因类**:「身份 = 来源」的假设在派生阶段失效——身份必须随**派生拓扑**演化(一对一继承、一对多分裂、多对一聚合各有身份规则),否则去重/一致性等所有以身份为键的机制同时失真。

**泛化解**:
1. **派生身份规则表**(typed,按 action 拓扑):`expand`(1→N)=`<source_locator>#<expansion_ordinal>`;`filter/qualify/project`(1→1)=继承;`group/aggregate`(N→1)=`group_key` 派生身份 + `_source_locators` 列表(已有);规则与 action 注册表同源(新 action 必须声明拓扑);
2. **溯源保留**:派生行同时保留 `_source_locator`(血统)与新 `_row_identity`(去重/一致性键)——两键分职,血统用于 lineage 展示,身份用于键;
3. **一致性门语义修正**:决策一致性以 `_row_identity` 为键;同源不同派生行可持不同决策(本来就合法);
4. **tripwire**:action 注册 census——每个 action 声明 topology∈{1:1,1:N,N:1},缺失即红;expand→count 合成 fixture(item 有 3 tag → 计 3)。

**验收 pin**:①expand→count 合成例 3 而非 1(红→绿);②expand→filter(include/exclude 不同兄弟)不误拒;③1:1 action 身份继承不变(既有 data eval 集回归:data_jsonl_filter_count/data_multifile_reference_projection)。

### §40.16 逐项细化 ⑩ V1-5:死的 legacy trace_finding 车道仍整答案硬拒

**定性**:`contract.Required` 生产恒 false,`trace_finding`/`replace_trace_finding` 不在 schema、不被教学,但隔离 profile 放行该顶层字段、解码器绑定、`resolveTraceFindingForEmit` 只要字段在场即报错→`failEmit` 整答案拒。**泛化类**:「已退役车道的输入面未同步退役」——退役必须是三面(schema/教学/解码)一体。**泛化解**:①退役 census:任何 schema 不含、教学不提的顶层字段一律走"未知字段隔离+日志"通用臂,禁专属硬拒;②删除 Required 车道全部死码(schema 臂/validator/setter)或在 `contract.Required` 常 false 的前提下加编译期 tripwire;③通用规则:`failEmit` 只可由 schema 内字段触发(census:failEmit 调用点的触发字段 ⊆ schema 字段集)。**验收 pin**:stray `trace_finding` → 答案接受+日志;census 红检。

### §40.17 逐项细化 ⑪ V2-1:两阶段暂存快照在归一化之前

**定性**:孤儿-only roster 或后置依赖租约路径把 `NewPartialMutation(patch).Apply(prev)` 装为 pending base **早于** 确定性归一化器(:3104-3148,citation 标记/关系元数据保留/id 表面归一化)运行→后继代次看到的基线缺失容差与模型提交标记。**泛化类**:「事务内多阶段各自持有不同基线」。**泛化解**:①单一"规范化后文档"作为暂存与提交的唯一基线——把归一化提到 stage 之前(或 stage 时调用同一 `normalizePatchForBase` 单点);②暂存/提交/回滚三态共用一个 base 构造函数;③tripwire:pending base 与最终提交基线的规范化不变量相等(测试:同一 patch 两路径得字节相同基线)。**验收 pin**:孤儿 roster 路径暂存的 base 含 citation 标记与关系元数据(红→绿)。
