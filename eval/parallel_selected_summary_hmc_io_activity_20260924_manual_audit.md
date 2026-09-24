# IO 活动与混合说明问题：完整人工审计

- date: 2026-09-24T06:29:12Z
- sweep_start_ts: 20260923-232912
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

正式结论：机器1/2、人工0/2。恰好2并行×1，未追加第三例。代码为`f0ce8f6cb05a`；构建带dirty标记是文档未提交，运行前Go输入已提交且被runner核验。两份原始报告、日志、结构化旁路均保留，不用随后确定性修复倒签旧答案。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_capability_and_window_analysis | PASS | eval/results/trace_capability_and_window_analysis-20260923-232912 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 436s | 65 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 35/31/1ms数字与链保留；唤醒角色、起止解释和旁路负载范围错误，系统附录亦有计量口径问题 |
| 1 | trace_query_io_activity | FAIL | eval/results/trace_query_io_activity-20260923-232912 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 731s | 48 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原生8组数值正确；最终6组汇总被预览前缀预算挤掉，模型手填错数、混配对总体与时间边界 |

## 1. 独立IO端点统计

原始路径：`eval/results/trace_query_io_activity-20260923-232912/`，正式`.codrax/output/20260923-234121.246-63852.{md,html,answer-surfaces.json,root-causes.json}`，首原生结果`.codrax/blob/20260923-232915-000-63852/trace-query-result-48d73c43.json`。独立oracle为`eval/fixtures/hmosperf_io_activity/expected.json`，实际源行在同目录`events.systrace`。

- 实际原生查询[2,2.25)有8组、19个端点、0拒绝/0来源未知。RQ发起7次/37888B/28次每秒/151552B每秒，完成6次/28672B/24次每秒/114688B每秒；BIO两端各1/4096B；MMC请求2048B与实际完成1024B；F2FS两端各1次、大小未知，不补0。源行4/11/12/13/16/20/21是RQ7个起点，跨入完成、歧义与无完成端点均按独立总体计入。
- 原生8组数值、单位和缺测正确。最终手填表却成RQ6/5次、28672/12288B、24/20次每秒；把2.045、2.145、2.230都称超过2.250，把F2FS未知写0，比例和桶数亦错。正文把BIO的20ms请求称“有效归因”，没有等待或响应因果依据；错误不能归为格式问题。
- 原生结果真实进入首explorer（日志1239起、其1269附近复述了多个正确数值）；该轮仍出现已供数据后的错误运算/成员解释。后续有11次event_search，RQ/BIO使用归一化事件类产生更广集合，MMC原始事件名查询为0后再用pattern；行优先查询亦覆盖窗外行。不能让这些不同范围的明细替代首窗的独立汇总。
- 最终INIT日志2806–2981保留42个选择器/14组，但8组预览被6个旧在途组+2个BIO活动组占满；2945–2977的RQ/F2FS/MMC六组`rows=null`，其未知/比例Notes也未投递。普通观察账只有10/106条旧延迟，Known Facts仅512字节参数头，TraceCoverage是Top5旧量，原trace未重贴且extract跳过；不能声称“完整原始数据已经到最终模型”。这是真实共享交接缺口，不是统计引擎缺数。
- 最终错误aggregate未直接作为权威搬运；首emit日志3228手写3张表、未选择`runtime_measurement`，3284的patch只补facet仍保错数。发布选择入口正确不代表模型已经使用。修复应保证名册内摘要数值公平可见，并使用同源表输出省略细节，不加问句/答案关键词硬门，不让统计抢占链上根因账。
- MD/JSON/HTML工件均产生；HTML普通表载体与错误正文一致。旁路131字节、schema2、空根因、`trace_root_cause_contract_not_active`符合纯统计无因果合同；不授新根因。仅做HTML源码/载体核查，未声称浏览器截图视觉验收。

原MD SHA256 `dd55ce022a512a15e9391dd7634816ccc243fed5051386699a9d82709342187e`；原生JSON `48d73c438ae0797e5090e3a96991c055ab87385727f7d8f4bba8517bb6cab771`；完整日志 `1ace359e5d5430d25cdf7438089d75d96a8fd7e3d87bdcafb52258b004cdae7e`。归08.2实现验收/16.4交接与18.5真实答案记录；确定性修复不改本例FAIL。

## 2. 公开工具说明＋真实文档响应

原始路径：`eval/results/trace_capability_and_window_analysis-20260923-232912/`；正式`.codrax/output/20260923-233626.549-63851.*`；独立源`eval/fixtures/hmosperf_business_io_chain/events.systrace`。runner墙钟436秒，实际pipeline约434秒。

- 真正调用公开能力说明和6次指定窗查询，不用说明替代现场；35ms请求、31ms线程等待、1ms唤醒后调度等待保留。图保留OpenDocument/LoadDocumentIndex、31ms链上首因、backup47ms独立背景，没有把最长后台IO晋升主因。
- 正式MD:33把`pre_wakeup_wait`讲成唤醒方等待，而日志2605–2607已明确它属于被唤醒线程。MD:60把issue后、进入睡眠前的1.005–1.009010讲成“issue之前”，等待结束应是1.040010唤醒而非1.040000完成；:72又称整个读IO期间都在等待。日志2939已提供35/31ms独立时间尺及禁止跨尺相加，不能归为缺上下文。
- MD:52称backup completion“未发出唤醒”，但日志2938已说明false仅表示未形成该证明，不证明没有唤醒。旁路:46把worker全窗10.5ms负载搬到main的1ms调度等待“期间”，且来自另一CPU。旁路结构合法（schema2/available/3条链上选择、31/1/1ms和窗口正确）仍不能代签自由描述正确。
- 系统侧展示亦有缺口：MD:185/330/334将已知双尺降为“口径未明确”，系统附录:373/383将backup请求墙钟47ms写成“IO等待”。保留在原16.4/18.4口径统一任务，不误归模型波动；不在本批额外扩成第三条实施主线。
- 最终能力合同、唤醒角色、双尺已准确送达；剩余模型错误没有稳定系统原因证据，不新增关键词硬门，也不追跑第三例。正式MD、JSON、必选旁路、HTML数表及专用Trace图载体均核查；本地浏览器预览被安全策略拒绝，未绕过，不声称截图视觉通过。

原MD SHA256 `545d4662c7eba71893f4405ce6cb2fb750afc06b94ef7f424f564c9426eda97a`；旁路 `1a6bbf05c6e1e7956d70c5cf672817a42b3f2bbd174b7e327b02695c9121d9be`。原机器PASS/人工FAIL独立保留，不反写此前任何回放。
