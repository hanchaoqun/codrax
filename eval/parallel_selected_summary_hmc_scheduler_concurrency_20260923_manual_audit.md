# Selected Eval Manual Audit — scheduler concurrency / IO in-flight

- date: 2026-09-23T16:11:08Z
- sweep_start_ts: 20260923-091107
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

固定二进制为 `bf4680bc4c78`（buildTime `2026-09-23T16:10:36Z`），恰好2并行×1，无第三例。已阅读完整最终答案、原生查询JSON、实际阶段日志、JSON修复记录及旁路。机器0/2、完整人工0/2；后续修复不回写本轮原判。该版本不包含随后发现的上下文去重或IO跨分类交接修复。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_scheduler_concurrency | FAIL | eval/results/trace_query_scheduler_concurrency-20260923-091108 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 468s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=2/0,fin_reject=2,unavail=0,prune=0 | FAIL | 新统计值正确，闭合成员、跨窗、累计量和时序解释错误 |
| 2 | trace_query_io_inflight | FAIL | eval/results/trace_query_io_inflight-20260923-091108 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 929s | 49 | read=1,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=5/5*,fin_reject=0,unavail=0,prune=0 | FAIL | 原生四组正确但最终上下文丢占用账户，答案重算错组/错量/错单位 |

## 1. 调度并发：正确数值没有形成正确完整答案

原生 `trace-query-result-f5c38640.json`：在[1,1.01)秒内，Runnable峰值2/全窗均值0.8/忙时6ms/面积8 thread·ms，Running为2/0.5/4/5。3条闭合Runnable来自101、102、103；104只有唤醒无闭合，不能补到窗末。Running的101为1ms+2ms=3ms，原始窗前入口0.998有实际记录。

最终 `run-1.principal.md` 的核心两组指标正确，但把104列入闭合总体并漏103；同篇又说开放区间排除。把101的3ms写4ms，把已知跨窗入口当缺失，同刻半开处理误说依事件顺序，Running仅列前4段却未披露3段省略，还写“全部让出CPU”。不能把闭合总体的零贡献直接解释为系统空闲。机器因未匹配“未闭合”等词面失败，人工依据上述实体/口径错误独立判FAIL，不调整oracle掩盖语义错误。

实际日志 `run-1.logs/codrax-20260923-091120-000-61535.log`：3320起最终新typed卡有6行正确指标/coverage；3340旧TopRunnable有104的窗尾桶，接纳口径不同。新DTO没有接纳成员见证，不能凭同来源/同窗借旧Top成员。3327时序note明确`omitted=3`，原生JSON和首次工具摘要完整；这是上下文有界展示加模型漏披露，不是算术缺陷。预阶段错误自由文本已降导航，未验证为事实污染。event_search的1.0105为披露的lookup宽容窗，不改变1..1.01统计。

3488首轮blocks为JSON字符串，有损恢复拒绝；3530原生数组受理；3572第三轮patch删必需summary被拒，保上一受理答案。不能把`member_set`格式维度认作系统明确要求逐线程：它也可呈现两个状态组，模型选错成员层级仍负责。原生数值已在场的3→4算错不归缺数据。

## 2. IO在途：实际最终上下文跨分类遗漏

原生 `trace-query-result-889b3c3b.json` 四组（峰值/全窗均值/忙时ms/面积request·ms/窗内发起）对照如下：

| 分组 | 原生正确值 | 最终报告 |
| --- | --- | --- |
| RQ 8,0 R | 2 / 1.4 / 10 / 14 / 6 | 3 / 2.3 / 7 / 23 / 4 |
| RQ 8,0 W | 1 / 0.6 / 6 / 6 / 2 | 1 / 0.4 / 4 / 4 / 1 |
| BIO 8,0 R | 1 / 1 / 10 / 10 / 1 | 前四数字对，发起0，面积错写ms² |
| RQ 8,1 R | 1 / 0.4 / 4 / 4 / 1 | 数字对，面积错写ms² |

`run-1.primary.md:11,27–31,51–65`：请求数×时间并非ms²；未完成不能只进面积不进同一积分均值；歧义队列数不能与请求数相加。BIO实际1.000发起、1.010完成，却被说窗前开始且结束未知；编造1.0035→1.0075，8,1成员串入8,0，漏写请求；“不同设备无竞争关系”无证据。carry-in0.998→1.004与carry-out1.008→1.012原始端点已完整存在，不是Trace缺失。

实际日志 `run-1.logs/codrax-20260923-091120-000-61536.log`：1253原生window_stats、1771 event_search；1358受理交接已猜配歧义sector9000并漏跨窗成员，2013后续交接继续混配。2599起实际finalizer初始消息没有任何io_inflight行，2741仅旧IO耗时10行。分析器合法选择bounded_fact_set/count_or_duration，而独立在途展示只挂causal入口，旧公开fixture固定ResourcePressure未覆盖本次分类。这是已定位的可泛化系统缺口，不归模型随机波动；修复公共提层，不改原生算术、不扫用户/输出、不加模型字段。

`*` runner metrics显示inv=5/5是计数噪声：真实5次发射中1329/1946/1989拒绝、1365/2020受理，即3拒绝2受理。此记录保原机器栏并纠正人工口径，不用误计数作结论。最终2913一次受理、3009–3015完整写出，退出0，case 927秒/runner929秒，无旧map崩溃、无活跃流提前降级。

## 3. 展示、旁路与后续

两例都没有Mermaid，不拿本批验收图语法；调度文本列表、IO的HTML8列表格沿用各自报告，表格能生成不代表数值正确。两例MD/HTML/answer-surfaces/必选root-causes文件均存在；`20260923-091854.482-61535`及`20260923-092635.568-61536`旁路均schema_version=2、root_causes=[]、unavailable/trace_root_cause_contract_not_active，符合有限统计未启用根因合同，不是选择文件生成失败。

账本§176继续挂08.3/08.4及01.3/16.4/18.4，数量仍79=14交付+65开放。本批后修IO跨分类交接、逐字重复矩阵/矛盾源码提示；另将“总体身份+有界成员见证+时序详情预算”列成共享后续片，原Top桶合法保留但不代替新总体。04.2业务父子树、08.2规模/速率与08.4剩余桶分布仍保队列，不追加第三例追绿，不回签历史人工FAIL。
