# HMC §197：进程概览与混合源码任务完整人工审计

- date: 2026-09-30T02:38:28Z
- sweep_start_ts: 20260929-193827
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

代码冻结于`a15920630`，二进制revision同源（dirty仅为待汇总文档）；81542恰好两例并行×1，runner正式exit0，无第三例。机器0/2，完整人工0/2。后续`4e3198648`参数修复/目录去重只跑公开回归及全仓，不倒签此次答案，也不重跑追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | FAIL | eval/results/read_combo_command_current_source_explanation-20260929-193828 | answer_regex | none | 86s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 计数391正确；源码未读，操作receipt说明代替源码链路；无标准answer_surface收据，不将机器dynamic-binding失败误记为数值错误 |
| 1 | trace_query_process_profile | FAIL | eval/results/trace_query_process_profile-20260929-193828 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 174s | 37 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 源TID未传；调用不可计量被误写成原始数据不足；状态17/15ms正确，但Load热点和late线程丢失，等待被称业务耗时 |

## 1. Trace：从参数遗漏到错误缺证结论

已读取[完整主文](results/trace_query_process_profile-20260929-193828/run-1.primary.md)、[日志](results/trace_query_process_profile-20260929-193828/run-1.logs/codrax-20260929-193831-000-61180.log)、5个查询实际入参/输出、成文初始上下文与patch拒绝。

- 源事实：半开窗口[1,1.02)内原生TGID10的已观测成员10/11/12；13只在右边界出现。10运行17ms、睡眠2ms、等调度1ms；11运行15ms、已证IO4ms、等调度1ms；12有即时标记但调度状态未知20ms。10/11各自Load包含耗时8/7ms，不能跨线程或与CPU时间混算。
- analyzer日志795/830行把“ui（线程10）所在进程”判为`no_named_target/runtime_targets=[]`，理由是进程概览不是唯一诊断线程。这暴露**查找入口身份和诊断焦点身份混用**：系统已有目标自动补齐没有可用typed输入，不是补齐函数忽略了正确TID。不从原始问句关键词补硬规则。
- explorer1243行确实发现并调用新`process_profile`，但只传path/window，不传pid。工具返回`unique_source_thread_required`且不可计量；随后1285–1288行查10/11/12/13的thread_timeline，而未修复原调用或查询业务区间。新profile完整成员/热点没有实际传入finalizer，不能称为模型拿到正确完整profile仍说错。
- 成文中17/15ms状态正确，却只列10/11，称二者是否同进程未知；把自己的参数问题说成trace缺成员，把running区间和submit_bio等待叫“业务片段”，遗漏Load 8/7ms。还无证断言无阻塞Binder/无明显压力。预分析已把右边界13计入进程组4线程，且一度把ui唤醒时刻从1.007写成1.008；实际确定性状态纠正了时长，但身份/热点仍未闭环。
- JSON修补提出`add_facet_id`这个未发布分支并被拒；保留原FAIL。不扩大合法patch面，不扫描原文替模型修答案。
- 修复`4e3198648`：缺源选择器在已有typed目标继承之后返回明确参数错误及同源同窗retry提示，无零人口/缺证observation；pid/thread各自schema说明前提。不可计量展示不再列零成员。公开测试验证合法明确/继承目标继续成功。**仍需后续真实验收**；身份角色混用及最终答案义务分配留04.5/01.3/16.4，不把修复当父项关闭。

## 2. Mixed：路由按主次丢义务，短命令绕过完成评估

已读取[完整输出](results/read_combo_command_current_source_explanation-20260929-193828/run-1.out)、[日志](results/read_combo_command_current_source_explanation-20260929-193828/run-1.logs/codrax-20260929-193831-000-61163.log)。日志45行仍为`route=operation/needs_repo=false/current_source=optional/source=current_message`，reason明确称源码说明次要，尽管新教学已要求保留所有交付。分析/探索/提取/成文阶段均0；不能宣称路由已修好，也不足以归为一次偶发。

第一次find参数漏`-name`等失败；第二次`find internal/tool -name '*.go' ! -name '*_test.go' -type f | wc -l`正确返回391。新增生产文件只有一个tool文件，动态oracle亦391；机器FAIL是没有标准答案面收据，非数值错误。最终未读源码，却声称解释command_measurement链路，实际上只讲命令输出/退出码；还把相对路径输出称为绝对路径、8字节带填充输出写成`391\n`。这些解释不能作为当前源码依据。

追加代码审计：`commandOperationShouldRunMaterialEvaluator`只在执行结果带payload引用时触发；这次短输出无payload，CLI直接通向最终报告，新完成评估教学并未执行。因此下一高影响修复应建立**独立、可检查的交付义务载体**贯穿router/plan/短输出完成/报告，区分命令执行成功与整体目标完成；不能只继续叠prompt、凭工作目录一律强制源码、或把普通pwd等单一操作都无条件增加一轮重模型。原标准answer_surface操作收据问题也单列未完。

## 3. 校验、时间与失败保留

日志两例均为首响应600秒/静默300秒/非流式600秒；没有“流活跃但未成文就按短时限降级”。Trace174秒、mixed86秒，不是超时失败。无写模式动作，不代签写模式验收；无新增根因链、反转、算力供给或跨窗投影规则。

| 原件 | SHA-256 |
| --- | --- |
| Trace日志 | `7e6bbe953f6c931d784dbc2cde9d54d96096b56b1768d51e651bbdec740773cc` |
| Trace主文 | `bc397ff3acbc28f49ec9e15c2337d5b2746af1bf5bfa6741ff82dd5ecf77295b` |
| Mixed完整输出 | `b730b7d6586a78a91c87f377118fd967b267f1d25af5829bf0771ad6dd1c1e39` |
| Mixed日志 | `9bcff414561967e27d37176c455def9290e97b8d29e6eee19667ed2ab04ae129` |

## 审计口径

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
