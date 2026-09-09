# r1048：显式窗全谱 Trace 与 C 双错误传播人工审计

- 基线：已推送 `d931ab3d8`；clean binary revision `d931ab3d8361`，built `2026-09-09T09:34:47Z`。最终冻结代码已通过86包全测。
- 运行：`2026-09-09T09:35:21Z`，严格两路并行、各一次、timeout1200s；原case/oracle、read15/apply24步预算不变，没有第三例或重刷。
- 选例：H7最近r1039，覆盖显式窗、全谱根因与小贡献；libgit2最近r1030，覆盖双错误路径与原生C交付。H8存在旧oracle与现语义计价裁定不一致，本轮不选、不为过case删合法贡献；H3/H10是有限事实题，不强绑完整投影。
- 机器1/2；**人工H7整份回答fail，C最终补丁/原生行为pass、正式证明仍未闭合**。原机器结果、模型答案、计划及正式报告不改写。

| case | 机器 / runner耗时 | 人工结论 | 过程 |
|---|---|---|---|
| real_trace_h7_self_seat_full_spectrum | PASS / 225s | fail：等待人口说明漏发；模型仍有跨尺推断和次数误读 | trace5、read0，ctx47%；成文1次、拒绝0、patch0；投影保留 |
| github_issue_libgit2_foreach_worktree_symptom | FAIL / 288s | 两处代码正确、独立原生验证通过；proof-unverified真实 | read7/repo_map2，ctx28%；midloop2、unavailable1；原生失败后模型修正 |

C case自身wall为285s，288s是并行runner边界，不混用。运行记录 `.codrax/tmp/20260909-r1048-live-runner.log`；完整日志在表名加`-20260909-023521`的 `eval/results/` 目录。

## 1. H7：投影存在，不以机评PASS替代答案正确性

最终 `.codrax/output/20260909-023903.542-59012.md`（1175行）及同名HTML；日志 `eval/results/real_trace_h7_self_seat_full_spectrum-20260909-023521/run-1.logs/codrax-20260909-023523-000-59012.log`。日志文本检索用 `rg -a`。

### 已保住的能力与实际正证

1. 13762.791708–13763.024898s、233.190ms明确窗保留；目标running74.915、runnable1.536、sleep118.586、D36.757、IO0，已归账231.794、未归账1.396明确。没有将未归账当零。
2. 只有一份因果投影。自身running原始74.915与规则折算65.912、理想等效9.003分账；11段D及36.757完整。内核`dma_fence_default_w`是调用位置，不证明具体fence对象、持有者或GPU/磁盘机理。
3. 链上小贡献四席仍列出，未如r1039将它们整体降到背景；aweme四条完成闭合IO等待0.985ms保留。logd.writer整段49.656ms中仅0.033有锚、49.623属于邻近，未据整段冒充主因。JIT2.388ms仍是业务线索，不自动升级确定性因果/可消除量。
4. 业务span的ProcessComposerFrame108.005ms/11、Repaint98.485ms/11、CommitAndGetReleaseFence69.337ms/22等保留，与规则可消除榜不可相加。未入榜链上7/邻近5的披露仍在，不声称穷尽贡献。
5. **B1633a真实入口正证**：log2520–2537五CPU均给目标实际运行时长与最后正值片段起始代表频率，明确不是全桶恒频/驻留。CPU0同结果policy1530000kHz，其余四CPU不借配。MD50–56保留代表采样说明；导语“主要以840–920MHz运行”仍须按代表值而非连续频率理解。
6. **B1633b附注未命中、B1624b导航未触发**：证据事实对照先8项被其它事实占用、总体披露另6项省略；已确认频率附注未出厂，但未逐项恢复省略库存。本例read_file/派生导航未发生。不得用本次Trace成功给两项签新live正证。

### 确认的系统缺口及最小修向

**B1635a/P1：部分状态账户触发不了等待人口尺注。** MD153称“关注线程等待116.963ms中链上已归因14.750ms(13%)，未归因102.213ms(87%)”；完整已测状态账户的等待却是118.586+36.757+1.536=156.879ms。

- `internal/tool/answer_document_mutation_runtime_tree.go:17669`的TargetSymptomAdmission从已选板SelfRows取78.630+36.757+1.576=116.963，coverage于16664使用此人口，不是直接消费完整状态账户。
- 旧 `answer_document_mutation_runtime_run2fixa.go:163` 已有不同尺说明，但先经 `tree.go:16757` 的FourStateAccountProvable。后者要求五态显示精度恰等全窗，本例231.794≠233.190，返回nil并漏发说明。完整账户恒等式拒绝本身正确；错误是借此资格压掉另一条应有的范围披露。
- 下一批只修信息范围：等待句明确来自已发布自身状态行；有同capture/target/query账户时可独立展示已测等待与未归账边界，不能暗示两人口等价。保旧四态=全窗严格门，不替换分母/排名、不把39.916差额解释成缺失时段，不从prose决定发布。
- 需公开发布入口先红后绿，覆盖partial同域、完整同域、缺账户、异捕获/目标/查询、零值及正反序；身份不能只看相近窗口。目前生产见证+静态定位，尚未施工或签新RED。

**B1635b/P2：优先级候选标签丢失限定。** MD94的自身0.598ms不是模型凭空生成：query结果 `trace-query-result-ef584a21.json` 已有rank11 `priority_inversion_runnable_wait`，闭合优先级范围下的同CPU运行重叠，原Summary明确candidate。`internal/tracequery/query.go:27726`按实际同CPU运行重叠和同源稳定优先级判定，沿当前Harmony语义；不是已证锁持有者/唤醒依赖边。共享中文标签 `answer_document_mutation_runtime_typelabels.go:41` 却仅为“优先级反转·可运行等待”。最小修向是候选及同核重叠边界的共享展示，保token、时长、排序、方向，不借本样例撤销既有引擎合同；zh/en多面与模型原块不变需回归。本轮未改。

**B1622旧债及独立待证值差**：SelfRows的“共4段”、0.598的“5次”来自统计分组，不等于物理发生次数。沿既有次数口径清册，不另建同义工单。query本身同时有runnable1.576与目标状态1.536，0.040ms差异不是renderer改数；尚未追完区间生成/并集原因，不能归模型波动，不能用显示小批遮掉。

### 模型错误与JSON/旁路边界

- 首稿将5个依赖线程称4个；#4/#5授权小计5.324ms在导语像整方向总量，MD42有成员限定，不能夸大成“全文始终声称七席总和”。65.912+4.846=70.758仍作未经授权的方向相加，同句“三者”亦不对应两项；log2674已给重叠不可相加说明。
- MD86/1086将12条caller记录与11段D做差，编造一条溢出/另4条溢出，并怀疑实际等待更高；log2635已说明两人口未关联、禁止做差推测。D清单11段本身没有丢失。已有精准上下文不能保证模型每次正确，暂记成文质量残余，不加正文关键词硬门或系统代写结论。另核：排序表OS_IPC_14的14.763ms占用与1.354ms有效影响并列是合法双轴，答案没有求两者比值，不计模型错误。
- 根因旁路130B存在，`schema_version=2/root_causes=[]/status=unavailable/reason_code=model_root_cause_selection_rejected`，不是文件遗漏。前4完整ID有效，后4误用排名短hash；正确第5完整ID在log2124已入模，2834拒绝仅此可选选择，2851提供完整修复提示，未再发生patch。系统未猜ID、代选根因或删除正文；不能把空旁路写成成功程序化根因交付。
- 一次finalizer接受、零成文拒绝/重写，未见JSON教学自相矛盾。分析阶段两次拒绝是缺目标身份及scope与因果任务不符，不是成文合同振荡。无Mermaid正文，此例不计图语法修复正证；大段为系统文本投影而非丢图。

## 2. C：真实代码正确，验证闭包仍诚实未完成

结果 `eval/results/github_issue_libgit2_foreach_worktree_symptom-20260909-023521`；apply日志 `run-1.logs/codrax-20260909-023743-000-59217.log`；最终计划 `plan-1788946746876193000-59217` 的report/final文件原样保留。

1. 首次模型只给函数调用加括号，赋值仍得到比较值；前置probe曾拒错误，随后实际原生`make check`又抓到三条返回1的断言失败（log670）。不是系统修补器改错代码。第二次模型改成 `if ((error = visit_worktree(callback_status)) != 0)` 与 `if ((error = lookup_worktree(lookup_status)) < 0)`，两条不同条件保留。
2. 最终交付树 `run-1.repo/.codrax/worktrees/trace-1788946663324156000-59217` 相对原始源仅 `repository.c:24/28` 两处；tests及Makefile不变。原运行repo HEAD仍 `13cd1b9d3f75ba57368802452b9d7bb05ab1597f`，fixture未改。交付repository.c SHA256为 `e92f6a99afc75d599c2332a66279ffd5c873c36277c4b5dbc622de93e2cb96b9`。
3. 系统原生make check后两次成功（2244–2245、2638–2639）。独立审计在真实保留树跑原4断言exit0（0.331s），并通过stdin C harness直接链接交付实现，对 `INT_MIN/-42/-1/0/1/17/INT_MAX` 两参数7×7全部49/49通过（0.458s），覆盖正负callback、负lookup及先失败优先。人工输出在本轮工具记录，未另存独立日志、未回填正式report；前后检查未改交付源或tests。
4. **B1634a生产正证**：第一次失败log820–832给当前9项缺口、列8项并显式省略1；最终2374–2381及2768–2775四项全部显示。它们是两个behavior contract各自缺 `project_test_assertion_not_observed` 与 `behavior_contract_observation_missing`，属于不同typed义务，不是同一字符串被重复算数。controller2830–2868明确消费“aggregate通过不等于证明闭合”，不再只收到泛化incomplete。
5. 正式report仍只有aggregate make-test/check；累计终验8项与当前计划4项是不同范围。现有 **B1561原生逐断言收据/绑定债** 未被上下文修复完成，不能把模型声明的project_test_observations自动配给一个make通过，也不能人工改final绿。机器`verification_proof_incomplete`真实，UI诚实未完全验证。
6. **B1122-MAKEEMBEDDEDFAILLOC1后续/P2**：原生失败时 `run_tests_parsers.go:397–421` 摘要只取第一行，本例为编译命令；controller先误叙述编译失败，后续planner拿到完整三断言才修复。底层正式类别`tests_failed`正确、完整FailureDetail存在，非编译/测试判据错。旧B1122补Make包unittest明细，未解决普通C输出首行摘要。追加摘要供给义务、不另建同根工单；后续有界保实际失败输出/范围，不用新prose扫描给执行结论或改变验证门。
7. 模型初始两个observable-equals合同用代码片段作“返回值”的expected，属结构化计划质量问题，不由系统偷偷重写合同。最终仍留可诊断、可继续的真实交付树，不把形式未闭合等同源码没有修复。

## 3. 下批任务顺序与保护边界

1. B1635a/P1显示人口尺注：先公开入口RED，保测量/选板/根因资格；独立核runnable0.040ms差异，不在显示批顺手改值。
2. B676-PROBEONLYDURABILITY1生命周期与B1634d配置值交接，沿统一台账§1716设计分别交付；B1561原生逐断言证据不能被“上下文已显示”销账。
3. B1635b候选标签、B1634b关系可见别名、B1634c显式已应用计划oracle域、B1122摘要后续，逐批正反矩阵，不放宽typed权限、不让系统编业务结论。
4. B1629b坐标范围、B1626多请求窗、B1622次数、B1616b跨计划证明及旧fork权限可达性仍在账。当前没有证明模型随机性，因此称“已供精确信息后的模型错误/观察残余”，不称确定随机波动；优先修确定性系统缺口。

两例没有按连接年龄降级；H7单次finalizer72.888s，C全流程大于4分钟包含多次短调用，**均不是单条活跃SSE超过4分钟的live见证**。本批4ms/旧4m活跃流、真正停滞、取消、显式deadline工程回归已通过；不因尚未生成最终答案主动降级。没有改用户原文/模型正文硬门，没有系统删改图或结论，没有将背景提升为链上根因。根因双轴、D/IO/业务线索、显式窗与自动补齐保持。
