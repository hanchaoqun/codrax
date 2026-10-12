# §231 Selected Eval Manual Audit

- date: 2026-10-12T12:49:27Z
- sweep_start_ts: 20261012-054925
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_native_binding_20261012

主审与独立只读审计检查了原题、独立oracle、真实工具参数及载荷、最终上下文、完整正文/图/表和投影旁路。冻结live版本为`2920917e3c88`，每例一次、并行2；机器2/2 PASS，完整人工0/2。验收后修复不得倒签这两份FAIL，不追加第三例追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_lossless_sampling_tree | PASS | eval/results/hmc_sampling_native_binding_20261012/trace_lossless_sampling_tree-20261012-054927 | successful_tool_result,trace_attachment,answer_contains | perf_triage+trace_query | 174s | 38 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 正确原生树被具名目标投影移除；正文拆字面帧、合producer分母、跨单位推密度 |
| 2 | trace_business_io_sequence | PASS | eval/results/hmc_sampling_native_binding_20261012/trace_business_io_sequence-20261012-054927 | successful_tool_result,typed_trace_projection_count,trace_attachment | perf_triage+trace_query | 708s | 90 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=14,inv=2/0,fin_reject=16,unavail=0,prune=0 | FAIL | 主文背景IO升根因、业务窗混尺；图重复6泳道且未绑定Note倒序，投影正确不能替主文免责 |

## 1. 采样路径

`run-1.principal.md:1,11–15`把`sys;root`一个字面符号拆成三层，并给缺精确栈的5权重造路径；第3及21–23行跨producer合cycles=42、A=22/52.4%，丢掉raw35=精确30+未知5和simpleperf独立7。第29行由instructions100与cycles20推指令密度、第31行推单次执行周期/内联原因，均无配置或执行证据；局部免责声明不能修正主文。

实际两次调用均`perf_sampling_tree`、[1,2)、PID11；第一带pattern得到明确unavailable，第二模型主动去掉额外pattern后返回5样本3cohort，未由引擎静默扩范围。`trace-query-result-f689d737.json:116–123`保raw cycles3/35/30/5及未知原因，131–194保`sys;root`单帧与A/B不同库/IP，210起保simpleperf7。自然选择到树已经命中；旧perf_stats自动携树在本次live未覆盖，不能伪称覆盖。

accepted `emit_analysis-params-b6a21b2d.json`为[1,2)、`bounded_fact_set/count_or_duration`和用户线程document-worker；附件包络1.1..3不是提交的用户窗。最终日志2088–2096仅保错误预分析，2115–2117仅保两条树引用，没有原生树阅读段。精确根因是共享具名目标投影只按subject识别，而已校验聚合选择入口只实现process profile；树被删后没有deterministic事实替换旧预分析。此为系统交接缺口，不可归为已证模型随机波动。

## 2. 业务IO与时序图

正文保OpenDocument50/5/44/1ms和完成闭合等待31ms，但又把document-worker运行说成3.5ms；表中同一业务明写8ms却把宽窗窗尾5.5ms sleep加进40ms业务窗。背景backup47ms被两次排为“根因第一”，又无证宣称加剧存储争用；该事件不在目标唤醒链。35ms请求驻留与31ms线程等待虽各自出现，不能支持正文把所有IO统称同一根因。

最终Mermaid共6个participant：3个原生长ID加Main/Worker/Storage3个重复主体；两条已证唤醒相互先后正确，但1.040010唤醒放在OpenDocument开始前，业务Note仍挂另一组主体。图中“主线程发起LoadDocumentIndex”及一些运行/等待注释没有相应原生绑定。Trace因果投影仍在且把backup列为背景，不能用旁路的正确边界抵消主文的错误根因。

此例16次发射拒绝/16次patch，耗时708s，ctx峰值90%。图中被证实事件顺序的拒绝有真实命中（合并日志5242、5323、5641），但最终仅把该消息移到前面；未绑定业务注释仍不具备可校验时间权。不能通过扫描Note数字做硬门，也不能宣称新注释绑定已经live验收。

补充完整旁路审计：303–349行两张原生业务表各只出现一次，完整保50=5+1+44、40=8+1+31；不是业务量测未交付。351–384行的宽窗时序也标明自己的范围和尾段未知终点。仍有系统展示接缝：证据索引294–295行在“背景”后又写“根因排序第1/2位、IO观测时长口径未明确”；428–429行系统补充又把请求驻留写成“根因观测、IO等待”。217–223行E4尾段[1.0455,1.051)误挂早于该段的1.040010上游唤醒点，与376行实际终点未知不一致。这些是已出货旁路缺陷，挂既有12.5/16.4/18.4并提升共享事件实例/因果资格展示优先级，不当作主文模型波动，也不破坏已有正确Trace因果投影来绕过。

本次alias教学实际进入finalizer（合并日志4032–4033），原始17份emit/patch参数中from/to_native_participant、runtime_notes_v1、diagram_participant_edits均为0；也未选择native measurement，完整业务表为系统原生保留，不是模型主动复用。日志未保留实际provider完整tool schema，因此只能证明教学可见、选择未用及代码/公开回归可提供，不能声称具体注释选项的完整schema实投已验。新绑定功能的自然易用性仍欠验收。

实际alias失败诊断（合并日志5196、5556）仍只教使用技术endpoint或producer-listed ID，没有就地提示已存在的显式可读绑定出口；模型随后新增原生长ID泳道。这是可定位的协议教学接缝，不是已证模型波动。验收后修正该诊断及公开回归，仍不倒签当前图。其余16次拒绝包含表头4883、未证图边4956–4957、租约5085/5508、placement5163/5793、边界操作5384、重建锚丢失5420/5713、阶段carrier5594、旧addition_ref5843；正常第17次patch在5891–5892接受。principal与primary内容SHA相同；完整transcript仅一张Mermaid图，无恢复/降级或第二图附件绕过。alt/par/loop、跨source/window负控和原生注释选择未由本次live覆盖。

## 3. 计账与边界

固定两例原题/trace/oracle六份SHA前后相同。原始机器/人工结果及出厂产物保留在结果根；源payload与验证日志按SHA-256另归档。后继修复先用真实public路径反例证明，再统一末版全仓；不更改本次live版本/结论，不把确定性PASS包装成修后live PASS。其余任务状态与末版发布收据见[统一账本§231](../docs/design/hmosperf_capability_gap_audit_20260917.md)。

验收后补救已冻结于`9cd10ae0c1a6`并普通推送：有限具名目标原生树交接、peer角色及逐物理来源引用、精确alias修复提示和重复教学预算。公开RED→GREEN、相关race及末版全仓90包PASS/13无测试包/零FAIL，构建通过；初轮全仓的3个根FAIL保留。该版本没有再次运行live，两份人工FAIL状态不变。原始载荷、出厂产物及各版本验证凭证的路径/来源/SHA见结果根`evidence-sha256-index.txt`。
