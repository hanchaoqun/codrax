# HMOS gap batch 1：生产双例人工审计

- date: 2026-09-18T04:05:14Z
- sweep_start_ts: 20260917-210512
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

冻结版本42159453f069，严格2路、各1次。机器FAIL 2/2保持；人工已读取原始夹具、最终答案及相关工具/成文上下文。两例都有非空答案，均不是超时、畸形JSON或Mermaid失败；最终成文没有硬拒循环。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_native_resource_metadata | FAIL | eval/results/trace_query_native_resource_metadata-20260917-210514 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 186s | 28 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 3个瞬时操作+3个计数器观测误写6次操作；精确整数仍在但无据解释成浮点编码；0/MaxInt64被过度解释。 |
| 2 | trace_query_business_marker_io_chain | FAIL | eval/results/trace_query_business_marker_io_chain-20260917-210514 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 273s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 投影保留，但54ms分析窗/52ms状态账替代50ms业务窗；35ms请求驻留与业务阶段缺失，链上IO误作纯背景，唤醒前后表述倒置。 |

## 资源记录：数据交付正确，解释失败

- 实际查询使用0.0005–0.0035s，NativeHook命中3条且完整，FD在窗外；后续分别查HeapSize两条和MmapSize一条。源整数9007199254740993/9007199254740995/9223372036854775807与NULL/0/-1均完整到达工具结果与最终表，不是转换浮点丢精度。
- 最终正文仍把所有6行说成6次资源操作；将整数称“双精度浮点编码”、将0解释为释放成功、将MaxInt64解释未知或尚未结束，均缺源合同。MmapSize累计快照被描述“新增4096”，也不是观测本身能保证的delta。
- 用户窗口0.0005–0.0035s为3ms，记录首末0.001–0.003s为2ms；最终多次写“窗口跨度仅2.5ms”，两种口径均不支持。原始边界已交付，不归咎于源字段丢失。
- perf_triage确实缺少新增资源字段说明，已确认同源教学供给缺口HMC-16c；其错误提取有advisory隔离，不因此把全部最终错误都归系统。模型在正确查询结果已可见后仍误释的部分保留为解释质量问题，不加输出扫描硬门或系统改写答案。
- 资源总数oracle失败与人工事实一致，不改原机评、不扩大正则接受错误6次。此次只能证明元信息可交付，不能宣称资源分析生产质量已通过。

## 业务响应：保住投影，不等于回答完整

- 真实公共fixture测试已证明OpenDocument为1.000–1.050s/50ms；文档IO请求驻留35ms、S态完成闭合等待31ms、唤醒后调度1ms；47ms备份未连接目标链。原文B/E与IO记录在输入中，不是采集缺失。
- 模型只查询两条线程timeline、wakeup_chain和window_stats，均采用0.998–1.052s。最终把54ms查询窗及52ms已归账状态当完整业务响应；“未归账2ms在窗口之外”也错误，未归账不是窗外。
- Trace因果投影及storage-irq→document-worker→app-main方向保留；正文却说storage-irq唤醒“触发”前面的31ms等待，颠倒先后。31ms、1ms虽存在，不代表35ms请求驻留、50ms业务窗与LoadDocumentIndex业务归属已回答。
- 进一步逐条对照工具结果确认：window_stats已经产出OpenDocument/LoadDocumentIndex两个业务片段，并发布35ms请求驻留、completion_woke_issuer及31ms等待；不能归因于模型没有查询这些证据。普通业务片段只进入原始结果，语义片段发布器会跳过没有优化分类的业务名称；已发布IO行又未进入本次最终成文选出的10/114条观察。两处交接缺口均已公共复现，后续修复收据见下节，不倒改本次结论。
- 此轮验证HMC-02/04组合分析和业务窗口定位仍未闭环。请求/等待测量应保留为有来源的事实，不能因为未选根因就消失，也不能反向把窗口背景或业务标签提升为链上根因。原始查询/答案保留，不靠改样本、补写业务结论或追加第三例追绿。

## 跨两例的系统问题及边界

- HMC-16b：实际perf_triage教学一边禁无工具计量、一边要求时间差/阈值计算，确定性可复现的提示合同自冲突，进入独立修复；未复现同一声明硬门必带必拒。
- 大整数/时钟/状态权限均应由原文或typed测量支撑，资源新语义在预阶段与query侧统一供给，不能把前阶段误读变成权威。
- 上下文峰值资源28%、业务36%（200k窗）；业务analyzer5轮，最终1次补块，无成文硬拒循环。阶段能力与长prompt的成本另列观察，不据一个模型样本裁剪验证红线。
- 本轮模型流活跃，10分钟首响应/5分钟静默配置保持；没有4ms/4分钟无答案而降级。后续修复不改变此冻结版本的机器结果。

## 后续修复与原始验收分离

同源资源教学与预阶段无工具计量冲突已以`857920fa7`推送。业务片段、因果诊断IO计量交接已从真实`TraceQuery.Execute`到完整最终提示构建先红后绿；测试必须保留typed PerfTrace载体，不能用非紧凑上下文假绿。冷审另外发现并修正同一物理IO请求跨两个明确窗口被去重吞一窗的问题；重复同窗仍去重，不将双窗见证当两次请求。

这些都是修后确定性数据流回归，不是第二次live eval。本批仍为机器FAIL 2/2、人工FAIL 2/2；业务窗自主定位、最终模型解释及其它领域能力仍须后续异构生产回放。详细实施、测试状态及剩余范围统一见[主账本§11](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
