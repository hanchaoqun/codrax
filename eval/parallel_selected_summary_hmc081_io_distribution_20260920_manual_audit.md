# HMC-08.1 生产回放人工审计

- date: 2026-09-20T02:57:02Z
- sweep_start_ts: 20260919-195700
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc081_io_distribution_20260920

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_business_marker_io_chain | PASS | eval/results/hmc081_io_distribution_20260920/trace_query_business_marker_io_chain-20260919-195702 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 272s | 40 | read=0,repo_map=0,list=0,trace=15,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 原始50ms业务区间与56ms查询窗混用；把CPU执行叫作请求耗时，见下文 |
| 1 | trace_query_io_request_latency_distribution | FAIL | eval/results/hmc081_io_distribution_20260920/trace_query_io_request_latency_distribution-20260919-195702 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 485s | 42 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=7/4,fin_reject=1,unavail=0,prune=0 | FAIL | 已完成分支的正确查询结果未交接；最终统计错误、遗漏写组、猜FIFO，见下文 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 批次范围与原始收据

代码/构建固定为 `2887fb8ae324`，严格2并行×1，无第三例追跑；外层1800秒只是eval上限，产品600/300/600秒及活跃流策略未改。机器1 PASS / 1 FAIL，人工0 PASS / 2 FAIL。原自动summary/verdict、答案与fixture oracle不改。

- 分布报告：`.codrax/output/20260919-200505.060-1687.md`及同名HTML、root-causes.json。日志：上述结果目录`run-1.logs.all.log`；blob session `20260919-195703-000-1687`。
- 业务报告：`.codrax/output/20260919-200131.965-1688.md`及同名HTML、root-causes.json。日志：上述结果目录`run-1.logs.all.log`；blob session `20260919-195703-000-1688`。
- 两例均实际产生非空回答和旁路文件；旁路均`schema_version=2`、空`root_causes`，状态`unavailable`、原因`no_selectable_typed_on_chain_candidates`。不是JSON语法失败或文件未落盘。
- 分布问题明确不要图，答案没有Mermaid；其表格虽可渲染，却只有“列1…列8”，不标组名/指标名，人工不通过。业务答案有1个Trace因果投影文本树，不是投影丢失；此批不宣称验证了Mermaid渲染能力。

## 分布例：系统交接缺口与模型错误分别记账

独立夹具期望与实际确定性`window_stats` blob一致：RQ/R n=11，min/max/mean=1/11/6ms，P50/P90/P95/P99=6/10/10.5/10.9ms；RQ/W n=3，5/25/15ms，15/23/24/24.8ms；BIO/R n=3，2/6/4ms，4/5.6/5.8/5.96ms。RQ/R还明确1条缺完成、1个歧义组和2条抑制配对。查询scope 1..14s，未混入窗外499/1000ms请求。统计既不裁为窗口交集，也不取展示Top8计算。

实际工具教学已进入system和dynamic消息；日志2712–2718显示第二路模型当时读到了正确三组数值。因此不是“引擎统计错误”或“教学未注入”。但日志2964–2968显示该路被其他分支完成信号取消，ParseOutput在取消检查处返回，最终只有早先event_search的证据进入成文。源码还把非胜出分支整体跳过，连已成功工具产物也不合并；这是可确定复现的系统丢证，修复跟踪主账本§23.3。

模型也存在独立错误：第一路在数量超限重发过程中把歧义同扇区并发请求猜成FIFO；最终写RQ=17、BIO=5，P99=2/8ms，遗漏写操作组，把13.999s称为1..14窗外；无基准却断言设备正常/稳定。这些错误不能通过系统重写答案或放松配对规则来“修复”。

否证记录：曾怀疑成文压缩还会丢三组分位数字段；独立公开`TraceQuery.Execute → TurnA → BuildInitialInstruction`复核发现，既有generic ledger和priority inventory已保留三组全部八项数值、身份、来源、窗口及非阻塞边界。旧测试只认`io_request_p99_ms=`而没认等价`p99=`，不足以证明掉数。试验性新增提示段已撤回，不增加重复上下文；保留按数值/身份语义检查的公开正向回归，不冒称此回归是旧显示代码红转绿。此次生产主因是整份已完成工具结果未交接。修后生产效果尚未回放，不倒签本次FAIL。

## 业务例：机器烟测通过不足以验收

正确保留：OpenDocument/LoadDocumentIndex线索；35ms单请求、31ms提交者S态等待、1ms唤醒后调度延迟；47ms后台备份未直接加进响应，35与31没有相加。

错误：正文称OpenDocument 1.000..1.051“约50ms”，后文又给1.050结束；原始`span_window`结果`trace-query-result-7a4cb41b.json`已给出1.000..1.050=50ms。将0.999..1.055的56ms查询窗统计7+1+44=52ms和4ms缺口套到50ms业务窗；把7ms CPU执行改叫请求本身耗时；把字节数32768当扇区（实际923339752）；将“等worker”症状称直接根因；背景缺依赖证明被说成已证独立。

自动补齐日志3152–3155明确`no_typed_target`，root-cause查询0次，旁路无可选链上候选。不能把探索查询的PID直接当用户授权目标、也不能取消身份门；通用业务实例→目标/窗口的显式结构绑定仍归HMC-04.3/02.4。业务结束端点/状态分区口径与模型量纲解释归HMC-16/18持续验收。该case仍人工FAIL，不因projection存在而销账。

## 两例相同patch拒绝的审计

两例各1次post-emit建议、1次patch拒绝，之后保留已接受第一稿；不是结构化答案全丢，也不是活跃流超时。模型都提交了`block_field_edits_v1.field="facet_ids"`，而实际教学明示当前schema只在发布时允许`add_facet_id`，数组要完整`replace_blocks`。业务例同时replace同块且replacement已含facets，非法edit冗余且另有跨操作冲突。

当前schema构造、每轮刷新、发送和执行校验同源，未发现“同一声明必带又必拒”。日志没有完整wire schema，只能声明日志教学与源码路径一致，不能冒称抓包逐字核验。此处不放开任意数组edit、不自动改名、不增加修补轮。后续可从当前schema直接给本轮可用动作，减少条件式教学心智，归HMC-01.2/16.4，不为单次模型错误增加硬门。
