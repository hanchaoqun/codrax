# HMC §198：查找身份与独立交付义务——真实双例人工审计

- 批次20260929-202449，结果目录后缀20260929-202451；恰好2并行、各1次，单例runner总时限1800秒，52931正式exit0。
- 真实二进制为`4b5589fbc265`；后续`4a7d4fe9e6a3`身份出处/探索游标与`0550003d4`统一身份匹配修复未重跑真实模型，不混签版本。
- 机器2/2 PASS，完整人工0/2。机器原判见[summary](parallel_selected_summary_hmc_lookup_outcomes_20260929.md)，不改原件、不追跑第三例。
- 两例均为读模式，没有命中超时；不代签写模式或操作分支，不改600/300/600秒及活跃流保护。

| case | 机器 | 完整人工 | 耗时/上下文 | 已改善与未过原因 |
| --- | --- | --- | --- | --- |
| trace_query_process_profile | PASS | FAIL | 230秒/39% | 正确调用概览，工具含3成员、17/15ms状态、8/7ms热点及未知状态；最终消息丢失聚合记录，答案有窗外成员、业务时序混淆、未知补零和无证据排除性诊断 |
| read_combo_command_current_source_explanation | PASS | FAIL | 289秒/35% | 混合路由、393计数和实际源码消费者解释已到场；结尾把“排除`*_test.go`”误述成“文件名不含`_test`”，与开头及实际命令冲突 |

完整人工FAIL不代表核心改动没有效果，也不能把核心维度改善当作整份答案通过。前批漏TID/完全不读源码的原始失败继续保留。

## 1. 进程概览：工具正确、上下文丢失与最终误述分开记账

原始答案：[run-1.primary.md](results/trace_query_process_profile-20260929-202451/run-1.primary.md)。日志：`eval/results/trace_query_process_profile-20260929-202451/run-1.logs/codrax-20260929-202452-000-2126.log`。

### 实际过程

- 分类49及系统消息95将纯Trace问题标成`measurement + source_explanation`，错误增加源码义务。最终current-source为soft、未硬阻断，但这是新载体的假阳性，留01.3/16.4，不以“所有explain都需要源码”修复。
- perf预分析305已把边界外线程和CPU运行段混入文字概览。analyzer850尝试将四个发现对象登记为runtime_targets，854拒绝artifact_metadata来源；889重试保ui-10为焦点，却用通用原句“有哪些线程”为worker/late/outside登记lookup身份。
- `4a7d4fe9e6a3`已补身份与原句绑定：原句存在还不够，须含该typed TID/线程身份；10不匹配310/100，不能借通用问句给附件对象授予用户输入身份。此修复只验公共入口/负控与race，不能倒签本例。
- explorer1334显式传`pid=10`调用概览；本例未验证lookup自动补齐，后者由公共测试覆盖。5次trace_query、2次read（均为payload，非源码），midloop=0、investigation=2/0、final reject=0。
- 完整工具结果`.codrax/blob/20260929-202452-000-2126/trace-query-result-2743adcd.json`正确：thread_count=3，成员10/11/12；12状态不可计量、unknown_ms=20；10运行17ms/S2ms/runnable1ms、Load单实例8ms；11运行15ms/IO4ms/runnable1ms、Load单实例7ms；13只在右边界出现。Load实际区间分别1.002..1.010和1.003..1.010，不是CPU状态区间。

### 最终消息缺口：P1未修复，不能归为纯模型波动

最终prompt3550..3624没有`已观测进程概览`，只剩4条runtime observation、线程选择器及独立调度并发测量。旧概览仅作为非当前闭环摘要/证据文字留在3629之后，后续文字总结又恢复4线程错误。完整payload到过explorer，不等于无损聚合已到finalizer。

代码交叉核对：`answerDocFinalizerObservationRecords`先调用`answerDocScopeProjectedObservationRecords`；本次`bounded_fact_set + named_target(ui-10)`走主体投影。`process_profile_observation`主体是`process 10`，不是线程标签，当前主体匹配及global-family分支不能保留该聚合；`renderAnswerDocProcessProfiles`消费的是筛过的promptLedger。§197的单独公开handoff测试未设置实际bounded/named组合，漏了这个接缝。

还发现身份解释不一致：analyzer保留`ui-10 (tid=10)`而未填独立PID；tracequery的线程选择器能解析该typed标签，ObservationLedger的主体匹配仅支持裸数字括号等部分形态，不能解析`tid=10`括号。因而不只是进程与线程粒度不同，线程事实本身也可能在另一个消费者被误过滤。应统一typed身份规范化，并保留进程聚合的真实来源范围；不能仅给一个predicate加放行特例。

归04.5/01.3/16.4：需让生产者证实的查找主体、聚合范围和所求维度跨投影保留，来源/窗口独立校验；不能放行全部进程背景或把全部成员选成诊断焦点。下一片应覆盖实际分类形态×查找/焦点×来源/窗口的公共最终消息矩阵，并审计其它多主体聚合，不加原文关键词特判。

### 答案错误

1. 窗内3成员写4成员；outside-13窗外标记变成窗内缺测线程。
2. ui两段CPU运行5ms/12ms写成“两段Load、合计8ms”；worker Load7ms配CPU运行1.000..1.004区间。
3. “运行15ms，其中IO4ms”混淆互斥状态；late状态未知却在表格写0ms。
4. 并发正确分布为双线程15ms/75%、单线程2ms/10%、零贡献3ms/15%；答案把单线程写15%，又与另一runnable总体混用。
5. 无证据称所有线程prio=120、没有优先级反转/锁或IO竞争、ui睡眠是“正常间息”。概览不授予因果排除结论。未请求/产生图，不据此称图关系已验收。

上下文及答案一致性均未通过；不改fixture、不往用户问题追加系统本该承担的约束。

## 2. 测量＋源码解释：核心路由改善，局部语义仍留FAIL

原始答案：[run-1.primary.md](results/read_combo_command_current_source_explanation-20260929-202451/run-1.primary.md)。日志：`eval/results/read_combo_command_current_source_explanation-20260929-202451/run-1.logs/codrax-20260929-202452-000-2112.log`。

- 分类48为`hybrid/investigate/current_source=required/needs_repo=true`，不再纯operation；分析器674保留计数、递归范围、实现机制、源码和边界。
- 命令1085执行`find internal/tool -type f -name "*.go" ! -name "*_test.go" | wc -l`输出393；动态oracle绑定实仓，不是写死393。新增两个非测试工具文件使前批391变393。
- 18次read、3次repo_map、1次list、midloop=8、investigation=4/0、final reject=0。读到类型、账本消费者、聚合维度注入、证据路径软提示及explorer调用点，不再以命令receipt代替源码机制。
- 最终解释覆盖CommandMeasurement、`compileToolResultCarrierObservations→observationRecordForCommandMeasurement`、聚合维度补注及`postCommandMeasurementEvidencePathSignal→observeMidLoopWithContext`，说明静态定义/调用不证明每个分支实际执行。未完整读原生产者赋值实现，不扩大为全部路径运行证明。
- 结尾“文件名不含`_test`”比实际`! -name "*_test.go"`更宽；run_tests.go、emit_test_results.go、native_test_registration_binding.go等都应计入。开头及证据已正确，先归最终文字一致性残留，不让局部误述无限占能力轨，不加关键词硬门。
- 本例走分析管线，未live命中短输出操作目标评估。该分支的完成/partial、普通单操作负控、计划/续跑传递由公开测试验证，不混签。

## 3. 原件指纹

| 原件 | SHA-256 |
| --- | --- |
| Trace答案 | `8c71da31e2af8728116754460c160be00c22c0ccd19196313092f99f10efe098` |
| Trace日志 | `321d7df9290cfd2dba555666530ab8a638ba5a5a3f9d4f38b49f3fa4d8ce5d5f` |
| process_profile payload | `2743adcdaeaa709642cbc323541e80beb4f2c71b1a402502bc08e43bf8f32749` |
| Trace fixture | `3f3676fefc68363c1580f3dadcf0024dcda6d518cbcea1148bc8de36afdfe155` |
| 混合答案 | `aa620196b985cfa40f88cfd56dcbfb01b2b52570e7d7a0c04f36257e49ef1385` |
| 混合日志 | `9916abc71f60b0cfa2e26205d6fe7e8c9f951dd1f9d056cb63763156f5c7a672` |

原件在本地results/blob，仓库仅纳入小型汇总及审计。完整能力新增0，两组系统子能力/修复已实现，混合任务核心路由已改善；稳定验收父项仍5、本批完整答案残留2。79/16/63不因机器PASS或增加提交改变。
