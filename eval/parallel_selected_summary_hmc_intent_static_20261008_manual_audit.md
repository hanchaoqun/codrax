# HMC §209 — 请求意图保留与动态库初始化人工审计

- date: 2026-10-08T12:11:41Z
- sweep_start_ts: 20261008-051140
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_intent_static_20261008

固定构建 `ef1a030ac320`，恰好两例并行、各一次。机器2/2，完整人工0/2。两份采集均为构造协议夹具，不冒充设备录制。独立oracle/建库SQL不在模型工作仓；不修改自然问题追绿，不追加第三例。末版零owner去重边界补丁在live后通过确定性回归，不倒签这两份输出。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_catalog_objects | PASS | eval/results/hmc_intent_static_20261008/trace_catalog_objects-20261008-051141 | log_regex | none | 174s | 47 | read=16,repo_map=0,list=2,trace=8,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=3 | FAIL | 窗口声明保住；4次literal搜索+4次window_stats，无资源栈查询，错判无分配/释放 |
| 2 | trace_static_initialize | PASS | eval/results/hmc_intent_static_20261008/trace_static_initialize-20261008-051141 | log_regex,trace_attachment | perf_triage+trace_query | 230s | 40 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 6/15ms正确；库名被改写，真实CPU0被误称占位，Gantt坐标/时长/单位不一致 |

## A. 目录逐采集、逐线程资源事实

原件：`trace_catalog_objects-20261008-051141/run-1.primary.md`；完整日志`run-1.logs/codrax-20261008-051146-000-73613.log`；预期见`eval/cases/trace_catalog_objects.oracle.md`。

- 本批新分支真实命中：log511的明确窗口10..10.05与question=not_applicable冲突，514拒绝；552重发bounded_fact_set，555成功。没有旧版“缺附件carrier→抹掉请求范围”行为。八次查询都保留显式10..10.05，不把这项正确性签成整答通过。
- target=no_named_target是模型主动声明，理由误将采集中的线程排除出运行时主体，并非系统后处理抹除。工具调用仍手填101/202；完整目标名单未被正确建模，归16.4留案。
- 872目录发现保留两份同名文件，但没有登记expected queries，本批目录计划缺窗补齐分支未live命中。900–903的四个event_search把`malloc|alloc|free|kfree|kmalloc|realloc`当正则串，而该参数是literal；1008–1011转window_stats，始终没有resource_stack。
- 对照原生资料，四组合应为3事件/8帧、1/3、1/3、0/0。终稿却称各组合没有分配/释放；把全源资源载体/rename计数21或5说成线程调度活动。观测末点与完整查询范围也未清楚区分。最后加“未匹配不等于从未分配”的caveat不能修复主要错误。
- 已查询结果提示有resource_stack，完整能力教学也存在；通用零匹配导航仍未利用同源可用能力，不能全部归随机波动。后续以确定能力、查询用途和对象/窗口驱动软路由，保留其他等价工具路径，不能扫描最终原文强行改答案。

## B. 动态库初始化与CPU可知性

原件：`trace_static_initialize-20261008-051141/run-1.primary.md`；完整日志`run-1.logs/codrax-20261008-051146-000-73605.log`；预期见`eval/cases/trace_static_initialize.oracle.md`。

- 默认SQLite准备和真实event_search已保两段：libimage.so，10.002..10.008，6ms，起止都有CPU0 Running见证；librender.so，10.030..10.045，15ms，CPU未知。没有CPU时未丢真实区间。背景线程202和窗后liblater没有进入终稿，正向子能力成立。
- 模型三次查询都主动扩到10.051，交接已明示“用户窗10..10.05、补充查询窗10..10.051，不能代替指定范围独立账户”；本例未实际多计窗外库，但不得把显式扩窗说成本批自动补齐覆盖。
- 终稿6/15ms与先后顺序正确，却将librender.so写成libreader.so，并生成一个源中不存在的Base64串；又把两段CPU都判未知、真实标准ftrace `[000]`说成占位。未知CPU标记只属于第二段，不能跨成员传播。
- actual finalizer的Trace Observation Coverage（日志2140–2145）对第一段提供标准原始B/E行，对第二段主要提供编码协议原行；后续摘要又截断name字段。已经解析好的可读名称和逐成员CPU状态没有形成紧凑完整成文表，需优先修通用协议可读交接，而不是让模型手工解码。原始payload/正确raw行存在，不代表模型上下文已精准、低噪、易用；也不能归咎源表信息缺失。
- 确定性接缝：`answerDocBoundedRuntimeObservationPromptRecordAllowed`依赖顶层Subject，过滤掉Subject为空的已解析event_search_inventory，仍保留原行evidence_pack。不能直接恢复整个清单：当前库存无条件复制event.CPU，而未知CPU解析事件该整数默认0、真正不可用状态在PluginFields。下一修复需先贯通逐行CPU可知性，再按确切来源/owner/窗口投递可读字段，不把模型member_notes当事实权威，也不连带放进背景记录。
- 终稿Gantt使用`dateFormat X`，却把任务写成`0, 6`与`30, 15`，并用`axisFormat %10.3f s`。没有明确毫秒单位，首段起点不是实际相对2ms，图的时间坐标/持续时间不可信；不得只因围栏被接受就签图正确。该关系/时序表达残余归12.5/16.4，需统一量尺与结构化数据绑定，不写SoInit专属规则。

## 保留与后续

两run、原始查询载荷`payloads/<session>/`及验证/失败日志`validation/`均保留在本批结果目录。源范围保留、数据接入正确性、最终答案验收独立记账；五个稳定验收父项及全部旧FAIL不改签。本批完整父任务新增0，累计18/79、61开放；静态初始化来源准入为17.7子能力，普通viewer/完整启动树/持续写入/实机仍开放。
