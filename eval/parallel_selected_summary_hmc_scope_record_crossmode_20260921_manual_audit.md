# 固定双例人工审计：非根测试身份与状态记录计数

- date: 2026-09-21T09:55:23Z
- sweep_start_ts: 20260921-025523
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_scope_record_crossmode_20260921

固定二进制3b2a41eaa0f3，2并行×1，runner session6160正式exit0。自动2/2 PASS；完整答案功能人审1/2 PASS。新实现的公开回归、全仓通过不等于本轮live命中，以下逐项区分。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | eval/results/hmc_scope_record_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-025523 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 101s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 数值/双窗/旁路正确；状态占比被升级为恢复正常；§79未live命中 |
| 2 | nested_python_increment | PASS | eval/results/hmc_scope_record_crossmode_20260921/nested_python_increment-20260921-025523 | write_apply,write_patch_oracle,answer_contains | none | 198s | 28 | read=8,repo_map=1,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 实际单行源码修复+3原生测试通过；scope join未成功命中；cwd coupling系统债 |

## 1. 真实双时间窗分析

[正文](results/hmc_scope_record_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-025523/run-1.primary.md)第7行称“有意义的恢复”“正常获得CPU”，但本轮只有两个窗口的状态账，没有健康基线、性能效果或等待机制证据；“完全阻塞”也过度概括S观测。因此整体保留人工FAIL。实际最终输入日志1666–1668已明确状态不证明原因/S不证明具体等待，1751限定bounded事实，未发现新的系统供给缺失；不为本轮模型越界加原文硬门或追跑第三例。

A=34579.472865–34579.475857，2.992ms，running/runnable/sleep=0/.014/2.978ms；B=34579.475857–34579.505857，30ms，3.414/.780/25.806ms，0%和约11.4%计算正确。B CPU1=1.971ms，其余CPU2/3/5合计1.443ms，未说单线程多核并行。表格IO零值未充分说明是调度标記统计口径，但正文没有明确宣称排除所有IO机制，另留表达观察项。

首轮scalar与time_windows混用、缺pid/thread被现有精确信号拒绝后修复；4次查询成功，最终一次发射。该题为bounded_fact_set/count_or_duration，比较表合法、无需因果树；必选schema2旁路已生成root_causes=[]、trace_root_cause_contract_not_active。最终Markdown/receipt与eval副本逐字一致，primary/principal哈希与receipt相符。只有4条状态来源记录、两窗，且未物化状态附录，所以§79遗漏计数句未命中；无背景尺图，§76也不计live命中。

## 2. 子项目源码修复

[计划](results/hmc_scope_record_crossmode_20260921/nested_python_increment-20260921-025523/run-1.plan.json)、[报告](results/hmc_scope_record_crossmode_20260921/nested_python_increment-20260921-025523/plan-1789984688082690000-55993.report.json)与交付树核对：seed3cdd2b06f4b387ee9c84b033945d4c8e7df796b0→applied526d9b9e只改widget.py:2的return value→value+1，1入1删。原checkout HEAD未变，无主干合并；tests/config未改，测试SHA256三面均504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322。

真实unittest在packages/widget执行精确tests/test_widget.py，exit0，3条assertion_scope结果覆盖负/零/正（含±2**64）。另1条probe有changed源第2行的真实target_execution；并非第4条原生测试，也不升格成行为合同证明。最终答案准确交代隔离工作树、4条验证结果及自然验收不等于逐项证明；current-plan post_apply_verify=true、final verified，功能PASS。

初版与最终PTO仍有身份错误：最后suite写TestWidget，已读源码及真实输出均是IncrementTest；id缺非根前缀。真实suite/id带python/unittest@packages/widget::，所以§78没有成功消费live见证。四项合同都是planning_only，required=0，未冒发per-contract proof，不销B2–B6；教学/成功身份可见性仍开放。

新增确定系统gap：同一个working_dir=packages/widget、from widget import increment在planner_probe已真实执行到实现并因缺+1失败（不是ImportError），但静态coupling要求repo-root模块packages.widget.widget，七次emit被拒。最后模型改用repo-root导入才通过。原生测试执行与这次planner_probe须区分，不能把后者当unittest。此缺陷优先按每个probe实际有效cwd解析改动模块修复，避免兄弟同名模块串证；禁止放松真实changed-source耦合。
