# 47babb574 固定双例人工审计

- 日期：2026-09-21；批次：20260921-012200。干净二进制 revision `47babb574d4f`，built `2026-09-21T08:18:32Z`；runner 副本 `.codrax/tmp/codrax-selected-20260921-012200`。
- 每例一次、并行2、单例上限1800秒；runner正式exit0（session36566）表示批次完成，不代表两例通过。没有追加第三例或改case/oracle追绿。
- 结果根 `eval/results/hmc_coverage_state_crossmode_20260921`。机器 **1/2 PASS**，完整人工 **0/2 PASS**。功能、证据保护与整份答案/交付分账。

| 场景 | 机器 | 完整人工 | 已正确的部分 | 未通过的部分 |
|---|---|---|---|---|
| 明确窗唤醒链与链外长等待 | PASS，217s | FAIL | IO11ms、链外logger19.5ms隔离、投影/旁路、新覆盖/等待措辞 | 正文错用依赖分析窗为睡眠起止、混写唤醒与入核运行、机理/内部词面；末尾系统提示仍统称work |
| C++ long double双头修复并应用 | FAIL，208s | FAIL | 两头各一行修复、严格编译/运行、隔离交付、不授缺失证明 | 精确断言能力未齐，所绑现有测试不覆盖普通浮点合同 |

## 1. Trace：显示修复真实生效，完整答案仍FAIL

目录：[trace_query_wakeup_background_demotion-20260921-012200](results/hmc_coverage_state_crossmode_20260921/trace_query_wakeup_background_demotion-20260921-012200/)。核对该目录的`run-1.answer-transcript.md`、`run-1.answer-surfaces.json`及`run-1.logs.all.log`。上下文峰值46%，6次trace_query，无成文硬拒。

已正确：最终全文68–163行保留因果投影及明确2.000–2.020s窗；76–78行“链路覆盖不等于原因已全部查明”，113行“等待症状；沿唤醒/阻塞依赖下钻”实际命中。链上IO11.000ms、三项1.000ms调度供给候选分别保留；logger19.500ms在106–108/158–162背景段，不因更长而成为目标根因。方向threadpool→network→cookie→app不变。必选`.codrax/output/20260921-012534.583-74320.root-causes.json`正常生成，schema_version=2/status=available，仅threadpool的IO0.011秒根因，明确窗/来源/链深3一致，没有把logger或优先级候选塞入旁路。

未通过及来源区分：

1. 全文47行把network依赖分析范围2.001–2.018s当14ms睡眠，把cookie依赖范围2.000–2.020s当17ms睡眠并称覆盖全窗。真实cookie睡眠2.001–2.018s、network睡眠2.002–2.016s。最终输入日志2551/2553已明确`locator_role=dependency analysis window (not a continuous state interval)`，真实状态事实也已供给；不是查询丢证据，不通过扫描正文改写起止。
2. 全文15行把2.020000s唤醒与CPU1入核运行合写，实际sched-in为2.020020s，在选择窗外20µs。输入2541–2547已有精确窗口与wakeup/sched-in区分。40行“不直接阻塞”将未知关系写成否定；42行“根本源头”及后文重叠推非独立亦超过独立机理证据。函数名不能单独授资源身份。
3. 正文泄漏`depth=2`、`schema ... runtime_work_relation`。analyzer宽泛声明了业务关系需求，日志554已教独立业务子问才设置；输入2138又用同样内部词提示缺失，容易回声。保留模型过宽声明与上下文表达债，不加关键词硬门。
4. **确定系统教学缺口**：日志2574–2575的末尾`final_answer_mechanism_scope`对IO与sleep行仍统一要求描述为`on-chain work`。owner为`answer_document_final_decision_boundary.go::renderTraceFinalLeaderMechanismCeiling`；通用phase handoff亦有同类措辞。需与新工具状态限定统一，保机理上限和独立已证等待，不能让模型消解系统矛盾。
5. **确定系统显示缺口**：全文318–319/327–328的邻近E11/E12同时说“邻近支撑(无直接唤醒边)”又显示精确上游唤醒点。owner只按`row.Kind=Adjacent`选词；行未获链上计量资格不等于线程无已知唤醒边。需修显示权限含义，不能靠把邻近晋升链上消除矛盾。cookie/network原始占用镜像重复归既有去重债，不按名字或同值强并。

边界：HTRACE入口确为Harmony语义，52高于20有来源，候选不等于已证锁关系；不能套Linux优先级误判。D/IO根停递归不是要求把IRQ再画成主根因。初始analyzer一次fact_families冲突拒绝符合日志554已有正确教学，按提示只修字段后成功；与成文零拒绝分开。此次JSON/答案完整，无活跃流提前降级，不代验全部畸形JSON/超时场景。

## 2. C++：功能改对，不伪签完整行为证明

目录：[github_issue_nlohmann_long_double_symptom-20260921-012200](results/hmc_coverage_state_crossmode_20260921/github_issue_nlohmann_long_double_symptom-20260921-012200/)。核对`run-1.plan.json`、`plan-1789979033714548000-74339.report.json`、同ID的`final.json`、`run-1.materialization.json`和真实应用树/日志。上下文峰值28%。

- 仅普通头和single_include头第10行`%.*lg`→`%.*Lg`，测试与Makefile字节未改；应用提交`414d3bc`。合并日志2731、3125行起两次`make check`均含`-std=c++17 -Wall -Wextra -Wformat -Werror`真实编译和二进制执行，exit0。不是计划检查或降告警。
- 交付resolved，唯一保留计划上述ID，原仓HEAD仍seed `e4778bdcfab1be64311d9be69eed9ec52e3038dd`，隔离正确。
- 报告只有aggregate `make-test/check`，无assertion-scoped witness；`project_test_assertion_not_observed`保留`float-not-regressed`缺证。最终回执8–12行为`unverified / verification_proof_incomplete`，正文亦披露未完全验证。保守终态合理，不能放宽为aggregate=合同证明。
- 模型计划67行起把既存测试`main`绑定给`float-not-regressed`；实际测试只对两入口传`1.25L`并检查非空，没有double/float输入。因此未来新增C++断言执行生产者也不能机械授予该语义合同。B2–B6、生产者能力与模型错误绑定三者分开。
- hard-required comparator引用只有long-double签名的源码却称`snprintf_float(double)`；另一planning-only expected仍写旧`%.*lg`。这些模型合同错误未获权威，不由系统猜测/代写expected。
- 一次不可用grep在write_analyzer成功读窗关闭后；planner日志1960行`failure_rounds=0/2`，成功读取后正常收窗，未命中本片失败计轮修复，不能宣称该例证明重试下降。

## 3. 后续与销账边界

先消除实际命中的末尾提示和图关系自矛盾，再推进已有原生断言补登记B2–B6及精确执行生产者。正确证据已给但模型未遵循的区间/词汇问题保持人工FAIL，不反复加单例规则或重跑求绿。不倒签§70及更早业务/明确窗FAIL，不关闭HMC父任务；仍13/79实现已交付、66开放。
