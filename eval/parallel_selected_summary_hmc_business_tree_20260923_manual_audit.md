# 业务父子树与 IO 在途固定双例人工审计

- date: 2026-09-24T01:54:57Z
- sweep_start_ts: 20260923-185457
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

固定二进制 b18c9540b8f2，恰好2并行×1，61135正式exit0，不追加第三例。机器1/2，完整人工0/2；机器PASS不代签业务正确性。阅读两份最终主答案、最终交接/拒绝/修复日志、原始fixture、HTML及root-causes旁路后记录如下。后续代码修复不回写原verdict。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_business_tree | PASS | eval/results/trace_query_business_tree-20260923-185457 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 253s | 31 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 同步三层数字正确；AsyncPrefetch错归background-900；合法包含图缺证据接面，修复删除图，未围栏文本树在HTML塌成一段；重复数表 |
| 2 | trace_query_io_inflight | FAIL | eval/results/trace_query_io_inflight-20260923-185457 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 831s | 43 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=3,inv=4/1,fin_reject=1,unavail=0,prune=0 | fail | 正确四组原生统计已到最终上下文，答案仍串设备/层、错均值与面积、错误跨窗/成员/完整性说明 |

## 1. 业务树：能力命中不等于完整答案通过

- `run-1.primary.md`三层inclusive=10/5/1ms、自身=5/4/1ms；LoadIndex自身运行3.5、就绪0.2、睡眠0.3ms正确，不把业务耗时当CPU或根因，缺调度不等于零的说明保留。
- 原始`eval/fixtures/hmosperf_marker_tree/events.systrace`第5/14行的AsyncPrefetch由business-700发出，5.0025–5.010、7.5ms。主答案第20/32行却归background-900；后者只有同名OpenDocument。最终上下文日志2274已有正确异步归属，不能一概归咎缺证据。题目复合句可能增加歧义，但不能覆盖原始证据或放行错误归属。
- 探索只调thread_timeline/event_search，从有界raw preview自行误配B/E；系统在日志1591–1599自动补window_stats，完整结果含4个树节点。最终独立交接2279–2286按现有目标范围保3个business节点；background的5.5ms与未知状态并未在此独立卡展示。比较对象的结构化范围与实例绑定仍留04.2/01.3/16.4，不能按题目词面硬放行其它线程。
- 预分析错误B/E/数值被emit接收，现有Authority投影已隔离非validator摘要，不是自由文本直接透传；候选时间数值仍可误导查询单位。后续修复仅保行号/片段坐标、保原审计bundle，确定性工具数字不受影响。
- 第一稿OD→LI→DR关系图缺edge_anchors，2530–2548拒绝；现有证据池也没有业务ParentID转包含关系的接口，仅填锚还不够。2579局部patch删除d1，退成未围栏字符树；输出HTML `20260923-185908.004-64833.html` 第311–314行成为单个段落，不能称图形验收通过。这是实例关系供应/修复接缝，不是Mermaid库子集问题。
- 主答案再次附同一数表，增加噪声。旁路`20260923-185908.004-64833.root-causes.json`存在、131字节、schema_version=2、空根因且原因trace_root_cause_contract_not_active，符合该非根因问题；没有虚构链上结论。

## 2. IO：已修共享交接实际命中，剩余是总体/成员及答案绑定

- 此例最终family为generic（日志2803），新公共交接2974及2981–2984仍完整提供4组数值、完整分组键及曲线。原§176漏交接故障本轮未再现，不能重复记为“没有给模型统计”。
- `eval/fixtures/hmosperf_io_inflight/expected.json`及原生结果一致：RQ/8,0/R峰值2、均值1.4、繁忙10ms、请求面积14request-ms、窗内发起6/接受配对4；RQ/8,0/W为1/0.6/6/6/2；RQ/8,1/R为1/0.4/4/4/1；BIO/8,0/R为1/1/10/10/1。配对另有未闭合1、歧义队列1、拒配2。
- 主答案却把第一组写为3/2.1/10/21/3，写组变成1/0.8/8/8/2（自列区间也只加到6ms），又合成跨层峰值5/均值2.5/面积45/发起7；不能由无跨层关联的RQ和BIO去重或相加成设备队列。成员借入其它设备的other-reader-70，歧义9000及未完成10000也影响解释。
- 原始1000请求0.998→1.004、4000请求1.008→1.012，答案却否认窗前发起/窗后完成。发起计数独立于完整配对总体，不能把窗后完成说成不计发起；未闭合请求实际在1.009发起，也被写成窗口前。可观测8个完成配对不证明完整采集、物理设备队列或目标等待。
- 本次唯一最终拒绝是member_set写成复合字符串的结构/覆盖问题，重发后接受，不代表这些数值经过精确事实绑定。后续高ROI方案挂08.3/08.4/16.4/18.4：原生接纳成员见证、独立有界曲线、结构化度量/实例绑定；不扫描原文数字或添加重复提示硬门。本次没有证明只是模型波动，不标“已解决”。
- 输出`20260923-190846.916-64832.{md,html,root-causes.json}`正常产生；旁路同为131字节空非根因合同。跨组错误仍使正文FAIL，不用旁路存在掩盖答案错误。

## 3. 状态与下批边界

HMC-04.2验收中，08.3/08.4与模型实例归属/图表/总体成员旧债均保留；79唯一任务仍14已交付+65开放。先修通用端点查询导航、候选坐标投影、同实例包含图证据与精确局部修复，再恢复总体/成员能力。后续定向/完整回归只证明代码，不代签本批0/2人工结果；不追加第三例追绿。
