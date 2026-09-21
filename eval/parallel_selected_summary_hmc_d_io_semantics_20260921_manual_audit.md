# D/IO双层语义固定双例人工审计（2026-09-21）

冻结构建：`b55ca5d1809f`，buildTime `2026-09-21T13:00:58Z`。两例同时开始于06:01:42，2并行×1；runner56191正式exit0，但这只表示跑批结束。机器1/2，完整人工0/2；不追加第三次追绿，不修改case/oracle，不把单测通过抵销旧人工FAIL。

| case | 机器 | 完整人工 | 实际成本/修补 | 原因 |
|---|---|---|---|---|
| real_trace_c2_dstate_iowait | PASS | FAIL | 197秒；analyze1/explore2/finalize1；trace5/read5；一次展示修补失败 | 三段IO次数/端点/0.635ms正确，但主句错误否定原生D；无证据把小占比等同影响可忽略 |
| trace_query_wakeup_causal_io_chain | FAIL | FAIL | 196秒；analyze1/explore1/finalize1；trace5；一次展示修补 | 明确20ms窗口、目标S和三条唤醒边保留；链上11ms IO与各1ms就绪等待未进入主文，因果投影0/1；另有线程优先级和业务机制误述 |

## C2：正确晚期上下文已到，但早期清单入口存在投递缺口

结果目录：`eval/results/hmc_d_io_semantics_20260921/real_trace_c2_dstate_iowait-20260921-060142`。完整日志 `run-1.logs/codrax-20260921-060144-000-7834.log`，正文 `run-1.principal.md`。

原始fixture目标59566的三条`prev_state=D`，经配对kernel iowait标记分入互斥`io_wait`桶；三段0.138/0.147/0.350ms，合计0.635ms。非IO D桶为0，不等于原生D不存在。当前最终输入2242/2798/2814有新的共享定义，2824–2827有完整计数、三段端点；本片教学确实命中。模型仍首句宣称从未进入D，段内又说“没有非IO D”；二者不能混同。`sync_buffer_read_wi`可以保留为阻塞点，不能仅靠名称和整窗占比就确认系统调用类别、业务影响可忽略。独立Binder闭合0.524ms与三段IO端点不同，数值和边界保留。

首次无窗`thread_timeline`已产生243段，但payload `trace-query-result-76ccb4ec.json`没有`TargetWindowStates`；摘要只显示前12段，第三段IO位于省略部分。等待清单预览和共享语义都依赖该账号，故早期查询没有得到它们，后续翻查完整结果文件。这不是trace原始数据缺失，也不能仅靠再加终答教学闭环。下一片应复用已生成的精确时间线，通过既有单一builder发布清单；保实际窗口、线程身份、取消/完整性、半端输入、S+IO和独立Binder边界，不另扫全仓或升级full-artifact权限。

JSON过程另留：2911首次emit已带`facet_ids:["member_set"]`，展示覆盖软提示仍要求独立承载；2956模型选择`add_facet_id`，2959实际dispatch拒绝`field_not_published`，事务未落地且原正文保留。不是JSON畸形或成功自愈。独立复核固定版发布/执行代码：原子分支要求恰好一个required member_set且唯一合规未绑定列表/表格，本例两维度、已绑定summary均不满足；hint也明确发布时才使用，未证新合同矛盾。恢复提示应区分缺绑定和缺承载/数量，不放宽schema。日志没有完整wire schema，不冒称已读取。最终输出非空，旁路`.codrax/output/20260921-060456.003-7834.root-causes.json`存在，schema2、空数组、`trace_root_cause_contract_not_active`；有限清单问题不应为填旁路强制选根因。

## 明确窗口因果链：最终成文缺已采集的链上事实

结果目录：`eval/results/hmc_d_io_semantics_20260921/trace_query_wakeup_causal_io_chain-20260921-060142`。日志 `run-1.logs/codrax-20260921-060144-000-7822.log`。

原生目标app100在2.000–2.020为20ms S；threadpool400在2.003–2.014为11ms D侧IO，2.004是blocked-reason记录时刻而非入D时刻；threadpool/network/cookie各有1ms链上runnable。三依赖均CFS20，app为平台RT52，不能称threadpool为RT。`fscache_page_wait_on_page_bit`是已采集的阻塞点。不能从network/cookie名称推导网络请求派发、网络IO完成或cookie上下文职责。

该例不只是模型忽略完整终答输入：analyzer把“主要阻塞原因＋相关链路”收窄为`bounded_effect_verdict`；实际Finalizer只收到40条观察中的6条，集中于app自身，依赖的11ms/阻塞点未在成文前提供；finite教学同时限制根因/rank表述。已接受completion中有相关事实，但不能把其整段自由说明直接升级权威，其中也有错误推断。应审计typed问题范围、已选链上事实投递和报告形态的一致性；不得扫用户原文强制切换合同，不得为主因恢复而将背景入链。

原始trace及wakeup census有irq→threadpool边，因果展开本身止于终端IO节点；没有证据说明本批图解析器删边，主文却称threadpool为整条链起点，表述越界。最终系统附录保有部分11ms等事实，不抵销主文遗漏及投影0/1。旁路`.codrax/output/20260921-060454.579-7822.root-causes.json`存在但schema2空数组、`trace_root_cause_contract_not_active`；文件必产正常，因果合同未激活的上游原因继续开放。展示归属patch成功，但新增了内部回执术语。

进一步源码审查限定：接受的finite＋target_effect_verdict tuple内部自洽，relation_path并不授因果权限；所以不能仅据缺11ms就删除finite过滤器或定性它违反已授合同。先用同一fixture对照coherent causal与finite两套真实交接，区分分析模型误分类和系统实现故障；不存在合法因果授权时不能从查询结果/自由说明强造授权。本次用户需求仍未满足，但“系统确定性丢弃合法causal事实”尚未证实。

## 处置顺序与不可倒签

1. 窄教学/原始桶标签修复独立验收；模型答案的D错误继续留债。
2. 已计算时间线→等待清单的早期投递缺口，优先复用既有builder补齐，无重复扫描。
3. 复核展示patch教学与本轮dispatch一致性；复核有限判断与链上根因事实/投影的组合交接，先公开反例再改。
4. 已接受业务实例局部自动补齐、声明/观测原生pair、业务同卡分尺与B2–B6仍开放。父账13/79交付、66开放不变。

本批未再次live跑写模式、JSON畸形或Mermaid语法自愈，不冒充覆盖；600/300/600秒及4ms活跃流由本批独立回归保护，生产实现未改。
