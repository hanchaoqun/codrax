# HMC §227：共同窗口与恢复图隔离人工审计

固定版本 `4a2923bf80ee`，构建时间2026-10-12T06:36:27Z（dirty仅文档）。两例各一次、并行2；runner正式exit0，机器1/2、完整人工0/2。未重跑、未追加第三例。后续确定性诊断补救不倒签本次FAIL。

| 用例 | 机器 | 人工 | 时间 | 实际命中 |
|---|---|---|---|---|
| trace_dual_common_window_measurements | FAIL：无成功trace_query | FAIL | 外层52s，子进程51s | 分类后进入data流程，未进入量测准备/查询 |
| trace_business_io_sequence | PASS | FAIL | 外层583s，子进程580s | 原生Trace查询、正常首次成文和因果投影；未触发恢复 |

## 1. 双侧共同窗口：失败在跨路由来源交接，不是文件缺失

原始记录：`eval/results/hmc_common_window_recovery_20261011/trace_dual_common_window_measurements-20261011-233655/`。人工读取run-1.out、完整日志、metrics及data终态，而非仅依据机器摘要。独立oracle为`eval/cases/trace_dual_common_window_measurements.oracle.md`，不作为模型输入。

- 用户自然命名两份原件，共用1–2秒窗口，不要求内部工具或复述校验纪律。两SQLite实际可读且integrity_check=ok；原始行数15/10、各1条NULL时间，当前DB与SQL哈希未变化。不是case漏附件，也不能要求用户补写“不分析源码/必须调用trace_query”。
- 日志18141:128–129记录route=data/data_join。已有共享教学明确原生量测读取走原生reader，SQLite容器和表格形式不决定路由；这次模型未遵守，但不能把后续执行缺陷也归为波动。
- CLI分类时已做命名输入内容探测（cmd/root.go:1741），data执行创建新上下文（:1831）且配置未交接命名来源/reader。data_task_cli.go只发现repoRoot内候选；共用DiscoverCandidateFiles的格式白名单无SQLite/.data，runInspectMaterial又只按候选相对路径匹配用户绝对路径。此问题也影响普通支持格式的外部命名文件，不是单个量测type。
- 两份原件回`kind=unknown`、`material was requested but not found in candidate inventory`，外层却汇总`inspected`、最终`consumed=2`。这些并不是已读记录凭证。模型思考中提到coverage=true，但本次未独立从原始typed状态核实该字段，不能把思考复述当系统收据。
- 最终是needs_clarification，要求确认两个真实存在且可读的路径。trace_query=0，analyzer/explorer/extractor/finalizer均0；预期13+8选窗原值、11独立分组与两源状态均未执行，不应描述成解析丢行或共同窗数值计算错误。

归18.4跨路由来源/能力交接，16.4保留答案验收；不把全部路由问题扩充为10.1量测envelope的新退出条件。后继共用当前回合命名原件和读取能力，经执行器验证后沿既有准备/原生取数通路转交；候选不授予事实权限，未知检查不能计作成功消费。不新增关键词或扩展名硬路由。

## 2. 业务时序：取到正确事实，但正常接受稿仍有无证据关系

原始记录：`eval/results/hmc_common_window_recovery_20261011/trace_business_io_sequence-20261011-233655/`；最终报告`.codrax/output/20261011-234635.843-18140.md`。原工具载荷session为`20261011-233656-000-18140`。独立oracle为`eval/cases/trace_business_io_sequence.oracle.md`。

### 过程与上下文

analyzer共15轮：首次JSON截断，之后反复把附件0.999–1.051秒当用户显式窗。校验将引文来源与数值合法性合并，模型多次修改合法数字的精度；第15轮终于选择bounded_selector和真实用户短引文，无坐标进入探索。硬门未放行伪用户窗，但诊断歧义是可修的系统接缝，不靠放宽门处理。

6次trace_query真实执行：其中wakeup_chain(pid100,[0.99,1.055))原载荷`trace-query-result-01688d52.json`及`trace_query-96be3602.txt`明确保留35ms请求驻留、31ms完成闭合的线程等待与non_additive边界、80→200和200→100唤醒。marker状态账明确OpenDocument [1,1.05)共50ms、running5ms/runnable1ms/sleep44ms；LoadDocumentIndex40ms包含running8ms/runnable1ms/sleep31ms。独立backup47ms请求未获目标依赖链资格。这些工具事实足以支撑正确区分，不能把所有错误称为工具未供数。

进一步逐行核对finalizer：3797–3799已有35/31ms及非加和、4207–4210与4388–4391有proven_blocking_wall_clock=31/exact_complete，并非31ms交接时被删。系统矛盾在3706–3709将无目标榜标background/target_causal_authority=not_provided，但3742–3745又把同一榜写primary/secondary cause；query.go摘要生产者无条件按Tier/Rank命名cause。实际root_cause_rank参数无pid/thread，`trace-query-result-2df0a212.json`的target为空，另一个pid100唤醒查询不能自动给该无目标榜授目标因果权。主决策摘要还混入宽窗7ms而非业务marker5ms。应修共享目标/范围/分级，不扫描正文，不把正确数据存在当全部上下文精准。

### 最终正文与图

成文首次emit直接accepted（日志18140:4523–4536），0reject/0patch，未走恢复。接受参数`tool-call_function_fz18cig8ghj0_1-emit_answer_document-params-6729178c.json`和最终报告均有以下问题：

1. 把35ms请求驻留称为IO阻塞/有效归因，并写全部计入主线程等待；遗漏独立31ms线程阻塞口径，不能用相互重叠的请求与等待混替。
2. 在50ms业务窗中写主线程运行7ms（正确5ms）；7ms来自更宽查询账。把0.99–1.055的65ms查询范围叫业务窗口，并把OpenDocument继续执行画至1.051，越过真实1.050结束。
3. 将backup47ms列“链上根因#2”或“背景根因”，虽又说不在链上，正文自身矛盾；同设备不证明竞争。
4. sequence图无edge_anchors，却增加主线程→worker派发、worker→backup派发、worker→storage线程请求等未经证实的箭头。背景backup被接进因果图。两个实际唤醒方向部分正确不能抵消额外伪边。
5. 原生因果投影仍完整出厂，未被恢复隔离吞掉；但其“未定位链上主根因”、worker完成闭合IO在背景区，与模型主文的链上主因说法冲突。该事实分级/目标来源交接须另行定位，不根据正文关键词修补。

因此机器PASS仅证明有成功工具与投影，不代表语义正确。图可解析不等于关系有证据；这次**正常接受**旁路不能归为恢复图隔离回归，也不能声称新恢复能力已被live验证。新恢复功能的公开RED/GREEN、合法/坏图并存、原生事实保留等确定性验收另见账本§227。

## 3. 留账与优先级

完整父能力本批不销账，仍20已交付/59开放。两组子能力（共同窗消费、恢复图隔离）及同批诊断补救独立记账；本次两份人工FAIL保原文，旧五个稳定验收父项与59交叉不相加。

收尾又用公开确定性测试复现并补救两条末出口旁路：失败Emit后的合法Patch残留图附件，以及真实两轮Run恢复后重新附首稿图。它们属于恢复/附件展示边界，不等于上例正常结构图缺关系校验已修；末版相关/全仓收据见账本§227.6–7。两例真实失败原件与原判定不变，没有再次调用模型验收这些补救。

下批按两轨推进：10.1已有设备/负载元数据的保真交接；缺陷轨优先18.4命名原件跨路由交接，正常Trace图接受资格及目标因果分级另挂16.4/12.5/18.4，按影响排序，不同时开多条局部追绿线。未跑写模式/只读登记/其它二进制入口不补签。没有证据证明上述正常接受和交接缺口是模型偶发，不能以“波动”免验。
