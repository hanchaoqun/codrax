# pytest 调用归属修复后的固定双例人工审计

- 日期：2026-09-22；冻结生产提交 `a182682dd`。
- 构建：`a182682dda3a-dirty`，`2026-09-22T13:00:09Z`；dirty仅文档，Go输入已提交。其后只迁移结构审计函数登记，不改变本轮生产二进制。
- runner95668正式exit0；并行2、各1次、每例1800秒；无第三例追绿。
- [机器收据](parallel_selected_summary_hmc_pytest_invocation_20260922.md)保持原始结果。机器1/2、完整人工0/2；局部功能通过不代销最终答案/验收缺口。

| case | 机器 | 完整人工 | 已通过的部分 | 仍未闭合 |
|---|---|---|---|---|
| trace_query_jank_field_inventory | PASS / 337s | FAIL | 精确匹配3条、7/4/2排序、>2^53纳秒整数、70/40/20ms、全附件覆盖、时钟/应用身份不可直接授调度因果 | 线程/标记进程身份混用，IO/优先级反转的必要证据误述，反向减法文字、内部词汇 |
| github_issue_dateutil_relativedelta_float | FAIL / 133s | FAIL；功能补丁PASS | 仅源码修改，原4个unittest两轮均过，整数值浮点规整、非整数值浮点异常 | required行为合同没有精确项目断言绑定；最终诚实unverified，不是假绿 |

## 1. Trace：正确数表不等于完整答案正确

结果目录：`eval/results/hmc_pytest_invocation_20260922/trace_query_jank_field_inventory-20260922-060044`。审计完整`run-1.answer-transcript.md`、`run-1.primary.md`、`run-1.logs.all.log`，以及原`eval/fixtures/trace_query_jank_field_inventory/events.systrace`。本题是字段清单，不是调度根因题；不以无因果投影或无图判失败。

最终清单是3条：头行5.040/5.050/5.010秒，原始起点9007199255740993/9007199256740993/9007199254740993，对应终点9007199325740993/9007199296740993/9007199274740993；差值70/40/20ms，jank_frames为7/4/2。相邻阈值1、其它appid621、名称前缀诱饵99、非法帧数字段均未进入最终清单。全域扫描16行、14事件、三条完整枚举；最终未把匹配端点误说成采集缺损。

最终错误仍有：

1. 把`marker_pid=201`与`emitter_tid=101`都归为写入进程writer。原记录和最终行集明确`emitter_tgid=101`，marker PID201是独立载荷身份，没有201属于writer的证据。也不能把appid620自动变成线程ID。
2. 把“IO等待需要D-state或iowait标记”和“优先级反转需要优先级继承标记”说成必要条件。这会排除已有独立完成→提交线程唤醒证据支持的S状态IO等待，并混淆优先级反转与其缓解机制；不能把这些说明作为合法排查通道的硬前置。
3. 附注写成`start_ts−end_ts`与正时长一致，方向反了；主表及最前换算正确，不能因此忽略附注矛盾。
4. `artifact`、`scope_complete=true`、`time_domain_status: unverified`等内部表示直接进入业务答案。

过程分账：早期宽查包含7条，失败的完成声明曾选入99帧并错算时长；后续精确AND字段查询修正为3条，不能把该草稿错计为最终数表错误。最终首emit缺summary种类被拒；孤立claim_uses已由既有容错并入前块，不是该拒绝原因。随后2次patch只修结构，以上最终语义问题仍在。两条非法可选negative_observation被丢弃，但原生完整行集与时钟/身份边界仍送达最终上下文，不把它们误记成证据丢失根因。

上下文审计：最终消息保完整三条原生行、原始字节及精确身份/时钟；IO释义已有S/D完成闭合和状态分桶边界。另一方面，最终读者事实卡出现“目标线程的调度器标记等待清单、已记录的内核或工具原因、事件发生时间”，并不准确描述本题jank字段清单。应追查结构化分类→教学的职责接缝；尚不能证明该卡直接导致身份或必要条件错误，也不能按一次回放称为纯模型波动。不得扫描模型正文替换答案或增加样例关键词硬门。

## 2. Python：功能通过，但required行为证明缺口再次出现

结果目录：`eval/results/hmc_pytest_invocation_20260922/github_issue_dateutil_relativedelta_float-20260922-060044`。审计`run-1.out`、plan、report、final JSON、完整规划/应用日志、种子与应用树diff及测试原字节。

应用提交`614c04fde1e365c847aa1dedbe25c3da8cfb64df`，原repo仍为`34576b73d1162ac57da2a1d6fa6def9005eeedcc`。只修改relativedelta.py；`_normalize_int_float`将整数值float转int，其余float抛ValueError，原整数保持。测试文件原仓/应用树/夹具SHA256均为`8e7ebfe79df13b1c19d25d7ba7b9d5042b4dac4c05bef45e43710114f767c484`；README未改。

应用日志`codrax-20260922-060154-000-37097.log:661–685、1081–1104`：两轮实际执行1个plain Python probe及`python3 -m unittest discover -v`，原4测试均通过。报告5条是4原生断言+1探针，并非5原生测试。异常probe实际调用months=1.5，不抛则raise AssertionError，只捕获ValueError；工具观察到helper第20行异常路径，未执行的第19行是整数分支，不是漏测raise。

`non-integer-float-raises`为required，但计划未声明`project_test_observations`。whole-probe通过和目标执行行覆盖不是逐合同断言凭证；报告保留4个缺证明项，最终`unverified / verification_proof_incomplete`。控制器两次all_verified请求分别转为再次验证和accept_unverified，最终答复明确“未完全验证”。不把该安全终态签成完整验收，也不把缺凭证说成已确认功能错误。

规划消息已经提供准确PTO和plain-probe权威边界；分析器读过完整测试，但planner只重新读实现，并获取三条调用点grep，未读断言方法便提交计划。未出现工具不可用、预算耗尽或JSON修补失败。当前累计复核重用既有计划，重复跑测试仍不能补出缺少的声明；下一步优先审计“实际断言结果→有界只读补绑定→同交付重新验证”的通用路径，而不是从测试名字、成功日志或散文推断合同对应关系。

## 3. 修复命中、保护与未验收边界

- §130 pytest独占报告路径：本轮写例运行unittest，**未live命中**；有真实协议子进程的公开红绿及文件系统/并发边界回归，但host没有pytest及pytest-json-report，不冒称原生插件实测。
- §127精确既有测试执行意图/强收据/分析补读：本题只要求保护测试文件，未明确要求执行指定文件；这些新分支**未命中**，不自动制造执行义务。原4测试实际执行来自原有默认runner。
- 本批未改Trace引擎/窗口/根因准入/旁路或模型超时策略。600/300/600秒及活跃流保护专项64650正式exit0，不能把4ms作为答案降级期限。
- 旧人审FAIL、caller观察/源码双轴、业务局部补齐/容量、原生声明/观测pair与B2–B6继续开放。79=13已交付+66开放保持，子缺陷修复不冲减父项。
