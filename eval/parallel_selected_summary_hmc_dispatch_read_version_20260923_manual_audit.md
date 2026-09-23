# 写模式读取版本凭证批次：固定双例人工审计

- 日期：2026-09-23；代码版本`45823f0d38b2`，构建后缀dirty仅来自本批文档，Go/build输入干净且runner核验通过。
- 恰好2并行、每例1次；runner39298正式exit0，无第三次追绿。结果根`eval/results/hmc_dispatch_read_version_20260923`，原机器结果保留在同名非manual文件。
- 机器1/2 PASS，完整人工0/2 PASS。写入功能正确不等于验证闭环；静态目录出现所需词汇不等于单位、前提及可调用工具映射正确。

| 用例 | 机器 | 完整人工 | runner耗时 | 边界 |
|---|---|---|---:|---|
| empty_python_module_apply | FAIL | FAIL（功能正确，证明未闭合） | 298s | 仅实现文件变化，4个原生测试及1个probe通过；required c1无精确行为见证，最终诚实保留unverified |
| trace_capability_discovery | PASS | FAIL | 313s | 完整静态目录已到场，但查询view/指标混淆、计数单位及缺测前提误述；说明域仍被要求源码证据 |

## 1. 写模式：实际交付与证明分开验收

目录`empty_python_module_apply-20260923-015930`。已审计完整运行输出、两个计划报告、最终计划、应用提交与实际测试命令/断言。

评测主仓HEAD仍为`bd8316d316b47d8d32556c3feeacdf82d0c516a9`。交付提交`900d22953617b4b7aec916e0090ff0cec0269ec4`只给`totals.py`新增5行：以0初始化、单次for遍历、整数累加并返回。已有测试、配置和依赖未改，未自动合并评测主仓。算法满足空输入、负数、大整数和一次性生成器要求。启动时原有EnsureCodraxGitignore路径另产生未跟踪的运行时`.gitignore`，没有进入交付提交；不将主仓称为完全没有额外文件。

`plan-1790154100339560000-36194.report.json`与`plan-1790154234301248000-40781.report.json`均保留实际原生执行`python3 -m unittest "tests/test_totals.py" -v`，exit0；精确suite为`tests.test_totals.TotalTest`，4方法分别是`test_empty/test_large_integers/test_signed_integers/test_single_pass_iterable`。两份报告的原生invocation分别为`native:9f4d:18d7e7b6ed96fd08:2`、`native:9f4d:18d7e7cda1f9d430:3`，不是借用一次旧PASS。额外`verify_contract_c1` probe执行通过，共5条通过结果，不是5个原生测试。

报告同时明确`project_test_assertion_not_observed`与`verification_probe_missing_required_contract_ref`：普通Python assert与声明`contract_refs:[c1]`不自动产生逐条typed行为观察。最终`run-1.plan.json`的只读probe计划无PTO，c1仍required；其余planning-only项目也不能凭函数实现正确自动取得行为证明。控制器保留两个未闭合批次，最终输出“未完全验证”，机器FAIL为`write_final_verdict:unverified:verification_proof_incomplete`。这是未闭合能力，不是已证代码失败，更不是错误回退测试结果。

补登记尝试的PTO仍存在模型身份错误：apply日志2447用`python@unittest::TotalTest`，原计划用`unittest@tests::TotalTest`，均非实际suite。source-free入口仍确定拒绝PTO，但不能将本次说成“已正确登记，只差放行”，更不能因此自动改写模型suite或降低匹配门。

本次有一项真实窄路径正收据：新的补证PlanID为`plan-1790154234301248000-40781`，`cumulative_verification_scope`仍以原源码PlanID `plan-1790154100339560000-36194`及上述提交为唯一交付；probe的`target_execution`保当前PlanID/原SourcePlanID并status=complete。这次确实经过§160交付与补证执行身份分离路径，与之前只有普通apply的回放不同。但完整只读原生断言登记、模型材料投递和当前合同消费尚未实现；不能销§165或B2–B6。

本批新读取凭证是进程私有状态，Complete查询尚未接入生产登记/验证消费者，日志也没有发布完整版本/覆盖的独立收据，故不能仅凭8次read_file就宣布新的完整读取资格已live验收；该资格由公开真实ReadFile→Append回归覆盖。无强行降低required门、无扫描模型正文配对、无第三次回放。

最终停止决定为`accept_unverified_followup_without_failure_evidence`（apply日志3119；3108为模型申请），不是流式超时或预算耗尽；模型把3/5说成“已耗尽”并不准确。停止后保留实际已通过测试与未闭合证明，不强行归为代码失败，也未在活跃流期间按时间降级。

## 2. 能力目录：机器存在性检查通过，答案仍不准确

目录`trace_capability_discovery-20260923-015930`。审计完整`run-1.principal.md`、88行完整报告`.codrax/output/20260923-020441.135-36146.md`、完整过程及Finalizer输入。

错误位置以principal行号为准：

1. 第7–9、17行将`io_request_latency/file_io/io_pressure/scheduler_states`指标族混作可调用查询view，未给出到实际`window_stats/thread_timeline`的调用映射。用户据此不能准确选择调用。
2. 第16行将scheduler `count`与mean/分位时长统归ms；目录已经把计数和时长分开。
3. 第18行把wakeup/waking列可选，而原生唤醒边需要`sched_switch AND (sched_wakeup OR sched_waking)`。第47行又将缺唤醒事件扩大成所有runnable入口都不能确定，遗漏sched_switch可观察的抢占后重新运行区间。
4. 第49行说缺RQ端点就不能计算IO分位数，遗漏BIO与合格storage/filesystem配对替代来源，也与第7行自身矛盾。
5. 二进制部分方向基本正确，但“有条件而非全自动”未准确区分默认自动准备与格式/provider条件；泛称gzip/ZIP解码后处理，未交代完整文件入口、二进制stdin/inline边界及ZIP唯一合格成员限制。

通过的边界也保留：没有编造当前Trace测量、根因或图；采样cohort分母分离、CPU kHz、jank ns与Trace同轴、缺测不等于零的总体原则正确。没有实际Trace材料，本片写模式读取凭证不适用。

## 3. 系统上下文与完成通道缺口

过程日志`run-1.logs/codrax-20260923-015933-000-36146.log`第1873行携带可完整解析的静态目录：21个view、41个metric族、8种输入格式，45902字节，`static_only=true/evidence=false`。Finalizer获得完整typed材料及禁止伪引用教学；第969行只是2000字节日志预览截断，不是模型只拿到这段。上述单位/前提错误不能归为核心目录缺失，也不能凭一次回放证明纯模型波动。

确定性系统gap仍成立：第1079/1194/1279行要求2–5维度的源码操作席位，静态工具说明因此被推去寻找并不存在的目标仓源码。5次完成调用实际是1次装饰成员降级、3次源码席位降级、最后因4次无进展强制完成，不是5次hard reject。三次虚构工具名文件来源尝试均被拒，最终`citations=[]`，没有伪文件引用落地；不能通过放松源码引用校验解决。

第2268行仍要求至少2个citation，第2273行软接受。完整报告第77–88行随后附加无关源码定位状态、内部枚举`auxiliary_only/localization_auxiliary_only`与“部分锚点未校验”警告，而最终没有源码引用。该问题归原§166说明域完成通道及01.3/16.4/18.4，不再造重复父任务；需要统一适用性并保留混合源码/明确Trace窗的独立义务，不宜继续叠提示词追本例。

## 4. 本批结论

读取版本基础代码`45823f0d3`完成公开红绿、独立边界/race及完整全仓55744（正式exit0，87测试包/13无测试包/零FAIL）。它不开放source-free PTO、执行或行为证明，不能解决本次全部人工失败。下一步必须实现原生登记完整链路及静态说明域完整通道；任务仍79总项、14已交付、65开放，旧FAIL不倒签。推送与正式封存收据见统一账本§168。
