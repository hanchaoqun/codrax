# 来源与归因分量双例完整人工审计

- date: 2026-09-22T08:13:40Z
- sweep_start_ts: 20260922-011336
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_origin_component_20260922

冻结构建 `ea2365916f01`，buildTime `2026-09-22T08:06:14Z`。runner55480正式exit0；恰好2并行×1，无第三次追跑。机器2/2通过，完整人工0/2通过；不冲减79=13+66父账，不倒签历史FAIL。全仓74551正式exit0/87测试包、默认超时/活跃流保护27189正式exit0；窄修复的确定性验收与最终答案正确性分别记账。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_background_demotion | PASS | eval/results/hmc_origin_component_20260922/trace_query_wakeup_background_demotion-20260922-011340 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 47 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | FAIL | 分量已实到、链外仍背景；wake/Running、等待起点与机理误述；系统caller来源混淆 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/hmc_origin_component_20260922/read_combo_trace_current_source_explanation-20260922-011340 | trace_attachment,answer_regex | perf_triage+trace_query | 421s | 56 | read=12,repo_map=0,list=1,trace=2,source_lens=0 | midloop=7,inv=3/2,fin_reject=1,unavail=0,prune=2 | FAIL | 86.111ms与双来源保留；函数职责/正常关闭流程错误、证据ID引用错配、未实搜的否定结论 |

## 1. 链上因果与链外背景

完整读取 `.codrax/output/20260922-011704.157-97894.md` 441行、同名root-causes.json121行、17行fixture和 `run-1.logs/codrax-20260922-011341-000-97894.log`。主审复核正文/原始事件/旁路，独立审计全文同判FAIL。

保住的能力：显式2.000..2.020秒、目标app-100、20ms睡眠、threadpool D+IO11ms、三个低优先级依赖候选各1ms、四节点三条跨核唤醒均在；logger-900的19.5ms等待仅背景。Trace因果投影与完整文本图存在，不把无Mermaid视为失败。schema2旁路available，模型选4项，IO0.011秒、三PIC各0.001秒，PIC均保runnable0.001/deficit0的拆分和未证明反转的限定。§119两摘要在最终模型消息2599–2601/2614实际到场，每行对应自身identity；旁路描述明确runnable1ms，未把sleep17ms改成归因17ms。本片有live命中，但不外推为整份答案通过。

答案错误：md15把2.020唤醒写成切回运行；实际sched-in在2.020020窗外。把S状态直接称阻塞原因，md17把四节点写成四跳；md25把network的14ms pre_wakeup_wait起点借给threadpool（后者2.003→唤醒network2.016为13ms）；md27称app等待cookie完成工作20ms，记录只支持等待状态及唤醒链，不支持完成机理。md21/43将logger D写成D/iowait，而该线程没有iowait确认记录。完整系统图与末尾限定并未修正这些模型自有陈述，不新增原文关键词门或系统代写正文。

独立系统问题：§104“已证最大可消/11ms可消”仍在；占用表cookie/network各重复行仍属原身份归并债，不能按同名同值硬去重。新增P1是源码未解析污染运行时调用点：预分诊将caller填入stalls[].file（log274），源码未解析denial随后把真实raw caller及上下文调用点改成`<unverified-external-source>`（1199、2064–2066），但最终摘要2613、图及旁路仍保原caller。末尾md441又称`fscache_page_wait_on_page_bit`尚未被当前证据确认。原始trace第8行已经观测该caller，源码机理未证不等于调用点未观测。先沿typed运行时来源修双轴，不放宽源码/机理证明。

已排除的假gap：event_search显式2..2.02却返回2.020020导航行，JSON `trace-query-result-e2caf1e0.json`明确`lookup_only_boundary_tolerance`、requested/matching范围不同；对应工具有独立导航容差，统计/因果窗口仍严格2..2.02。不能把合法导航返回当成计算窗外溢，也不能替模型wake=Running开脱。

## 2. Trace＋当前源码组合

主审完整读取 `.codrax/output/20260922-012039.308-97893.md` 63行及空schema2旁路、全部两行B/E事件、查询结果与关键调用/拒绝/修补日志 `run-1.logs/codrax-20260922-011341-000-97893.log`。原生span_window在1000.100000..1000.186111上确实给出86.111ms；未硬塞源码标量。有限范围无可选链上根因，旁路`no_selectable_typed_on_chain_candidates`正确，不应强造因果投影或根因。源码函数定义、运行时B/E和单位说明均在，但不等于解释正确。

人工FAIL依据：

- md27/38把`durationOrderObservations`说成B/E时长计算主体；该函数实际构造顺序审计的open/close观察，并不相减时间戳。实际span_window配对在`query.go:10861`起的栈push/pop、`traceSpanFromEvents`等路径。
- md28把`resetTraceMarkSyncPairingState`说成正常E端点关闭；实际调用在生命周期重置或坏marker路径（query.go10222/10250和10816/10846），正常E只弹出一个栈层，不清空该线程全部栈。
- md23/24/27的模型原文路径与追加的证据引用分别错配到directMarkerNameGoverned、traceMarkRawCandidate、resetTraceMarkSyncPairingState。原模型选择的evidence IDs已错；源码定位附录能正确指出durationOrderObservations:467，不抵销主文引用错配。需要审计通用引用/owner与关系上下文，不加语义关键词硬拒。
- md19/40称当前仓无帧时预算/阈值，却没有执行针对这些量的实际否定搜索；可见grep仅搜Java外部符号、marker和span duration。不能用泛搜失败证明不存在，更不能把60Hz举例写成设备已知刷新率。给出86.111ms作为待核查线索可以成立，已知设备标准/最终性能裁定不成立。
- `resolved_files=0`、post-normalization/schema/wire等内部术语及末尾追加大段摘要重复，增加阅读负担；是展示/上下文债，不是JSON损坏。

过程：analyzer首次拿artifact线程片段当current-request source_quote被精确拒绝（787）；第二次bounded_effect缺verdict维度被拒绝（825），第三次接受。当前源码请求真实存在，保留源码调查是正确的，不应援用纯runtime豁免。第一次成文漏summary被拒绝（5187），随后patch只添加summary而未修正旧正文；答案未消失。一次completion负搜索缺query/pattern被拒绝（3400），补齐结构后接受，结构合法不能代替实际搜索证明。日志请求仍600/300/600秒，无4ms活跃流降级。

## 3. 后续优先级与边界

1. §104统一计量语义：链上有效归因、规则估算潜力、实施后收益分开；同时处理“区间重叠即可证明修其一另一项缩小”的反向教学。保数值/排序/链资格及模型正文。
2. §117完成接缝：公开流程核实已接受current_source_lane回执，精准减少无效重开，保真实mixed/precise与其它来源义务；不能删除所有soft要求。
3. 本次caller两来源污染：按typed runtime caller身份保观察事实，source未解析只限定源码/机理；保非法源码path/line反例。
4. §94业务局部补齐/容量、声明与观测原生pair、B2–B6继续；源码职责/引用解释和旧模型机理FAIL不冒称已证随机波动。

§118来源保护的5×8公开红绿是主要验收证据；本双例未声明错误qualified field profile，因此不能声称live触发旧兼容绕过。参考实现的member→JSON→field绑定及wait/supply/load分离已经落到本批共享边界，不照搬缺失填零、启发式主因或文字扫描裁决。
