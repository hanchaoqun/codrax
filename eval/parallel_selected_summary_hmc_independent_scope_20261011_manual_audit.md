# HMC §225：独立来源与窗口真实评测人工审计

- date: 2026-10-12T02:49:29Z
- sweep_start_ts: 20261011-194928
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_independent_scope_20261011

代码/二进制版本`ce5a8afeb1d0`，两例并行、各一次。机器1/2，完整人工0/2。审计核对最终接受稿、实际工具结果、分类IR、独立oracle和原始夹具；确定性回归与同批后续修复不能给本次答案倒签。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_dual_measurement_unbounded_partial | FAIL | eval/results/hmc_independent_scope_20261011/trace_dual_measurement_unbounded_partial-20261011-194929 | successful_tool_result | none | 97s | 30 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 正确双侧声明在范围修复中丢失；坏基线中止健康侧，未到取数/最终答案 |
| 2 | trace_dual_source_window_timeline | PASS | eval/results/hmc_independent_scope_20261011/trace_dual_source_window_timeline-20261011-194929 | successful_tool_result | none | 265s | 46 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=4,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | 接受pooled窗而未建立来源归属；最终正文上CPU时间错、频率效果越权，图泄漏技术ID且不展示完整过程 |

## 1. 无时间窗的双侧原始量测

实际日志`trace_dual_measurement_unbounded_partial-20261011-194929/run-1.logs/codrax-20261011-194933-000-24804.log`：525、611行两次都有正确baseline/current路径及连续局部quote，没有捏造时间端点。530、614行因full_artifact缺**外层**source_quote拒绝，却把错误归到comparison父对象。652行改成两个0..0 pooled窗口再次被拒；699行删除comparison后被接收，738行空基线在旧原子准入中止整个流程。Analyzer4轮；Explorer/Extractor/Finalizer均0、trace_query0、answer_len0。

已证系统问题而非纯随机误述：通用scope教学要求无窄边界用full_artifact，双侧教学要求无端点用unspecified；单源多窗教学又未限定capture而要求only time_windows。不同章节/schema存在相互竞争的模板，缺失叶字段修复未保留有效双侧结构。普通非比较full_artifact的未锚定quote原本已有安全降为unspecified的处置，comparison分支却新设整次硬拒，不一致导致已有合法来源身份丢失。

此例9条可定位原生记录+1条未知时间记录55、四个独立序列、NULL/文字0/大整数精度、末条原始50ms、逐侧状态表均**未命中真实验收**，不能用事前公开工具测试代签。缺失记录不能记为已证不存在。

## 2. 双源唤醒、调度与CPU时序

最终接受稿见`trace_dual_source_window_timeline-20261011-194929/run-1.out`的“已撰写最终答案”之后，非首轮草稿。实际执行两侧wakeup_chain，原始载荷已给出wakerA-201→target-41、wakerB-301→target-41、runnable4/8ms与running4/8ms；系统状态账与跨源边界有用且应保留。机器PASS仅说明真实查询成功，不代表成文正确。

最终问题：

- 基线写“等待4ms后在1.002秒上CPU”，把唤醒时刻当调度进入；原始事件为1.006秒。当前上CPU应为4.012秒，最终稿没有明确列出。
- 最终“总调度延迟约增加6ms”无对应数据，实际wake-to-run从4ms到8ms，增加4ms。两次所选窗口长度不同不证明相同业务操作总耗时翻倍。
- 虽未查询cpu_state_frequency，已有wakeup_chain载荷中的window_stats.cpu[].frequency_residency正确记录了1→2GHz（1.005秒）和1.5→0.75GHz（4.010秒）；不是工具完全未取到频率。正文/交接未保初始段与转换点，专用CPU idle/frequency联合时间线仍未产出；不能以目标线程非running时间代替整个CPU的idle统计。正文又把降低频率推成“执行速度大幅下降”，没有等量工作/可比性依据。
- 最终图保住两条真实有向唤醒关系，但可见标签是长rt_thread哈希；另保留与箭头脱节的S1/S2、CPU、WA/WB和TID 41节点。图未展示4/8ms runnable等待、实际上CPU点及空闲/频率过程，语法通过不等于可用时序图。
- 接受的分析仍是pooled time_windows，无comparison，因此本批来源所属窗口防护**没有实际命中**；未触发对侧高值不等于证明该防护通过。requested_work_relation又被误置，引入用户没要求的业务工作量缺证说明。

细证据位置：`run-1.logs/codrax-20261011-194933-000-24803.log`687行接受分类、690–693行丢掉两个未逐字锚定的唤醒关系维度；1089行调查aggregate把2GHz/.75GHz写成2.000MHz/.750MHz，最终单位虽恢复，错误比较推论仍留；2530行接受末次patch。`run-1.principal.md`31/43行定位正文错误，3行起定位最终图。原生CPU空闲6/12ms和线程running4/8ms在本夹具数值正确，不记数值错误；仍不能以这次碰巧同值证明任何线程统计都能替CPU统计。真实的两侧waker、4/8ms等待、状态账4/4/2与8/8/4ms均正确，701/801干扰未进入终稿。

首稿中已被门拦下的假调度箭头不再计为最终问题；最终稿仍有上述独立错误。日志/上下文暴露高负担：分类约5.5万估算tokens，修复多轮后最终上下文峰值46%；后继应统一声明教学和高价值事实交接，不扫描答案关键词添加场景门。

## 3. 记账与后继

两份人工FAIL挂原10.1/16.4/18.4及图关系任务，和59个开放ID交叉，不额外加两项或删除旧失败。本批最小补救只处理已证scope教学/叶字段处置不一致，公开正反测试验证；不追加第三例，不把确定性GREEN写成真实答案已通过。完整来源声明、原生CPU取数/展示复用、可读的图身份与精确时序继续保持待验收。

后续补救`a1bef9838429`已统一单源/双源scope教学，并将合法无窗comparison缺外层全量quote的处置与普通scope对齐：仅降unspecified，保各侧来源和局部quote，不授全量/数值窗权限。新增公开正负和12种实际Run入口、相邻契约及race均通过；该提交未再跑live，因此上表机器/人工结果完全保留，不推断新版本模型答案已经通过。完整验证和发布收据见[统一账本§225](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
