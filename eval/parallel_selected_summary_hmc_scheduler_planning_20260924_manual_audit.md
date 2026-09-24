# 调度完整分布与测量规划：固定双例人工审计

- date: 2026-09-24T10:12:46Z
- sweep_start_ts: 20260924-031244
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

构建revision `490992289eab`，build time `2026-09-24T10:12:20Z`，快照`.codrax/tmp/codrax-selected-20260924-031244`。严格2并行×1，未追加第三例。机器原始结果不回写；人工判定两例均FAIL。内部metrics244/419秒与外层246/421秒口径不同。自然问题不提供防错清单，不要求答案背诵防护句；旧长问句及旧失败保留，不能跨不同问句声称A/B提速。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_scheduler_concurrency_distribution | PASS | eval/results/trace_query_scheduler_concurrency_distribution-20260924-031246 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 246s | 40 | read=1,repo_map=1,list=1,trace=1,source_lens=1 | midloop=5,inv=4/0,fin_reject=0,unavail=0,prune=0 | FAIL | 漏传3ms；错误成员和旧开放尾投影冲突 |
| 2 | trace_query_io_activity | FAIL | eval/results/trace_query_io_activity-20260924-031246 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 421s | 53 | read=0,repo_map=0,list=0,trace=20,source_lens=0 | midloop=2,inv=3/2,fin_reject=0,unavail=0,prune=0 | FAIL | 原生统计正确到场，正文错桶/量尺/范围并漏速率 |

## 1. 调度：数据、交接和终稿分开判断

目录见上表；日志行号指`run-1.logs/codrax-20260924-031249-000-14136.log`，答案行号指`run-1.primary.md`。独立oracle `eval/fixtures/hmosperf_scheduler_concurrency/distribution.expected.json`没有交给模型。

- `[1,1.01)`秒内原生Runnable峰值2、均值0.8、忙6ms、线程时间8ms；Running峰值2、均值0.5、忙4ms、线程时间5ms。深度0/1/2持续4/4/2ms与6/3/1ms，分布/分位数正确。
- 用户要3ms，唯一实际query（1161）未传`bucket_ms`。原生`trace-query-result-4ec107bd.json:1017`为默认100ms，实际finalizer2337–2341/2357–2361同样给100ms/1桶，非引擎不会分3ms或交接截断。
- 终稿24–26/44–46却写三段“每3ms”，末段合并成4ms、漏最后1ms零桶；Running第二桶0漏掉decode在1.004–1.005运行，Runnable第二桶1既非峰值2也非均值4/3。正确3ms峰值应Runnable `[2,2,1,0]`、Running `[1,1,2,0]`。
- 真实closed成员2329/2349分别是render101/decode102/upload103及4个Running闭合区间；两状态×四视图共8个selector，2361预览17/128行，无成员省略。2527 emit全部普通table/section，无`runtime_measurement`，2603只补facet。终稿67把未闭合104补成1.009–1.010/1ms，与69排除声明矛盾。accepted `aggregate_facts`在1517已把upload换成unfinished，2390继续保留错误reason。
- **确定供给矛盾，不能预归模型波动**：旧`state_drilldown`把开放尾投成有限1ms（原生blob2326–2347）；最终消息2314仍是`value=1ms, interval=1.009..1.010`且无逐行closure标识，2302却称exact finite fact family。新closed总体正确不代表旧旁路不误导，需统一端点/闭合状态展示权限。
- 来源合同冲突：54 route `current_source=optional`；1292/1377完成门仍要求源码，驱动1323 repo_map→1412 list_files→1445 README→1482 emit_evidence；最终2272却明确allowed_optional/current_source_required=false。1239还把运行时线程装饰名当代码identifier成员门。不是用户应补“不要读源码”。

输出`.codrax/output/20260924-031650.517-14136.{md,html,root-causes.json}`存在。HTML349–351/385–387复制错桶，478–482复制开放尾伪闭合、487又声明排除。11个答案块无diagram，表结构可用，失败是语义；未作浏览器截图视觉验收。旁路schema2/空roots/`trace_root_cause_contract_not_active`，纯测量题正常，无伪根因。

## 2. IO自然问题：原生8组正确，完整答案仍FAIL

日志行号指该例`run-1.logs.all.log`，答案行号指`run-1.primary.md`。独立oracle为`eval/fixtures/hmosperf_io_activity/expected.json`。

真值：RQ发起7次/37888B/28次每秒/151552B每秒，完成6次/28672B/24次每秒；发起读5次21504B、写1次16384B、其他1次0B，读写次数分母6、字节分母37888。100ms发起桶4/1/2次、21504/0/16384B，末桶50ms。BIO双端各1次4096B；MMC请求2048B、完成报告1024B；F2FS双端各1次、字节未知。未知不补零，各层各端不相加。

- 20次trace_query，1274/2068两次window_stats都是原请求窗2..2.25，默认100ms本题正确。原生`.codrax/blob/20260924-031249-000-14135/trace-query-result-7ab463ae.json`的8组正确。实际finalizer3151完成组、3170起发起组、3208 F2FS及3227/3246 MMC双端summary齐备，42种表选择（6在途×3+8活动×3）可用。
- 3170–3184包含正确7/37888/28/151552、读写比例和分布；distribution/timeline只预览4行但标明总行/省略，选择受信表可展开全部保留行。3429模型仅提交summary+caveat自由正文，3482保持原文，未选任何受信表；不是渲染丢了已选表。
- accepted交接2245既有正确7个发起成员，又写W=2、写49152B、读22528B、时序5/1/3，混入2.250右界；reason“7次=5R+2W+1FS”自相矛盾。查找宽容右缘8项与半开统计7项均带范围说明，但未经量尺绑定的模型聚合仍并列交接。引用真实query不能使自由算术自动成为原生计算。
- 新规划分支**未命中**：777模型显式声明4子题，读写占比是required comparison_axis，804/831按4项调度。规则有意保留显式子题/比较义务；不是旧8个相似名称自动派生。公开回归通过不等于本轮已验证提速。1790–1793又用未支持的`event_types=[block_io]`得空，后续去掉筛选再查；工具选路/枚举教学仍留账，不能把不支持值静默当全量。
- 终稿5行事件数/读写字节正确，但7行把发起比例5/6叫“有效完成”；11行把1024B放进4–16KB，缺写请求分布。29–31行写4/0/2次、24576/4096/0B，错把完成量混入发起桶；实际2.100零字节发起仍为1次，末桶写发起16384B。RQ全窗速率和BIO/F2FS事件率未回答，机器缺28而FAIL，人工失败不只靠词面。
- 19行双端2048/1024正确，但不能由此证明硬件“传输效率50%”；33行将有限已配对观察扩称“之后无并发/无竞争”，对未配对/歧义请求不能作全局否定。35行把窗内2.200写与2.250另一写混同，实际成员端点仍完整到场。F2FS未知字节本次正确，这一局部进步不等于全答通过。

输出`.codrax/output/20260924-031945.736-14135.{md,html,root-causes.json}`存在。旁路同样schema2/空roots/contract_not_active。无diagram，未作浏览器截图验收；421秒正常完成，没有4分钟主动降级。600/300/600与活跃流保护未改。

## 3. 留账和证据摘要

两份人工FAIL继续挂08.4/08.3及01.3/16.4/18.4/18.5，不倒签、不追第三例、不扫问句/答案原文作门。剩余系统问题是结构化统计粒度→实际查询→同源/窗/总体可信交付，以及旧新投影的闭合状态和可选源码门一致性。原生正确后的模型误述独立记录；供给确有矛盾时不能先称偶发波动。

下一完整能力优先17.7按引用有界解析共享字典；以上高影响接缝择一为缺陷轨道，不同时铺开所有旧提示词优化。只读测试登记已交付范围保持，write当前阶段投影债仍在队列。全仓及后续修复收据见统一账本§182，不能用它替换这次真实模型快照。

| 工件 | SHA-256 |
| --- | --- |
| 调度primary | `a781a003f78987a86f1d8b167b8a3f7c47ae3ebb1a68bb268c2eafd1b34e83b5` |
| 调度日志 | `49aef617ec4c43d411a04b0d54e0ed19bdaaf04fc613fc229576d98c731c0b69` |
| 调度HTML | `4197b5a1ca04be529f41b52395c3721150c609d5c21ea5a17ef5889e8e594343` |
| IO primary | `e81fcb3463d5f3c72ad52b64b57f5ca5db59392137f71bb795ef164d740ad97d` |
| IO日志 | `050a45f9fcd7e7614e1baa84463657af3e04508e807254fb1bfc98ff4a76f789` |
| IO HTML | `1d44cb6027f1ef6f364e603058600fa6d85b221e316be559dd3345ec455c847f` |
| 两例root-causes（内容相同） | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
