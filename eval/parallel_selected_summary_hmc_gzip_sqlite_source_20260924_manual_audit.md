# §184 压缩SQLite与源码义务双例人工审计

- date: 2026-09-24T14:54:53Z
- sweep_start_ts: 20260924-075450
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定2并行×1，session56103正式exit0；快照`.codrax/tmp/codrax-selected-20260924-075450`来自冻结`8da8d75b43bd`，构建dirty仅文档。机器原判1/2通过不修改；完整人工0/2。接入和源码完成门的真实过程验证通过，不等于全部答案通过。未追加第三例，也不倒签§183的失败。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_gzip_sqlite_jank_inventory | PASS | eval/results/trace_gzip_sqlite_jank_inventory-20260924-075453 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 136s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 数表完整正确；单位/字段来源解释错误，供给已正确，独立留账不无限追跑 |
| 2 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260924-075453 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 192s | 40 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 四阶段数值正确；域/名称错绑，未知名称和两条未解析系统事件未披露 |

## 1. 压缩SQLite自然卡顿清单

输入仅为`hmosperf_gzip_sqlite/capture.transport`，独立README/oracle未给模型，隔离源码仓仅stub。真实路径识别压缩内SQLite，不调用转换器。最终表完整准确：2条/appid27599；2帧、1020000000→1040000000ns、20ms；4帧、1100000000→1160000000ns、60ms。未把打点写出时刻1.041/1.161当实际卡顿端点，也未把背景业务/状态升为根因。后两次真实查询(log1988/2038)已带time_start=1/time_end=1.2；首轮使用原生payload起止/帧数筛选，不能只凭首轮称整个过程没有显式窗。此夹具不能单独验不同窗口语义分歧，覆盖限制不当本例答案错误。

完整人工FAIL来自primary第1/12行、正式MD和answer-surfaces中的字段来源误述：称毫秒耗时直接来自`reported_duration_ns`且无需换算。原生值为20000000/60000000纳秒，20/60毫秒须除以1000000；起止值独立来自start/end，duration也是parser从两者差值得出。log2962的模型emit已写错，渲染未改义。最终上下文log2813明确差值→毫秒，2815/2816两条完整结构化供给无省略，故不是系统没给数据。该供给充分下的成文误述及`runtime_work_relation`内部词面记录于16.4/18.4，不为它追加局部关键词硬门/第3次live；单次收据不证明统计意义上的模型波动。

源码可选门真实跳过(log1275/1276)，forced_reads=0；最后scope与root sidecar正常。HTML真实tbody两数据行与MD/JSON一致，无diagram，未做浏览器截图验收。

## 2. 原启动／系统事件问题复测

四段1.010–1.018/1.022–1.026/1.030–1.034/1.040–1.045及8/4/4/5ms正确。机器8ms/5ms两正则未识别表头单位，属于测量误报；缺未知身份说明则真实失败，保机器原FAIL不改签。

旧缺口继续：两个`startup`均被当作恢复出的阶段名，未披露NULL/重复字典引用导致未知；1.050/1.052两条原始HiSys身份不全事件仍未披露。11是当前语义导出行数（8个B/E+3系统事件），不能代表原数据库窗口完整事件人口。报告把SQLite app_startup合成B/E说成“应用在主线程注入”，没有原生注入机制证据。

新证据确认语义交接精度丢失：原生JSON `trace-query-result-ada83898.json`第78–79/230–231/250–251行准确给出plugin_domain=APP_LAUNCH和独立plugin_event_name；但`types/trace_event_search_inventory.go:81`及`tool/trace_query_event_search_inventory.go:73`只保通用字段与Jank专臂。实际最终模型上下文log2074因此只有print、comm和raw，要求模型重新拆原文；最终正式MD38–40把comm=`<hisysevent>`填作域，事件名填成域/名拼接值。属于可证结构化handoff缺口叠加模型错绑，不可全部称模型偶发。并入01.3/16.4最高缺陷，下一批统一审各语义族、容量/单位/原始与合成来源，不只加两字段补丁。

本批源码门真实过程PASS：log59 optional，803接受分类无库存义务，823/826跳过源码图，1169 allowed_optional/none，1348–1349跳过完成阶段强制读取；read_file/repo_map/list_files/source_lens均0，仅一次[1,1.08]原生查询，无README、无扩窗/因果分析。不能用此过程通过代替整份答案通过。

## 3. 原始产物与SHA-256

原始模型日志与机器结果保持在表中目录。正式输出分别位于`.codrax/output/20260924-075706.516-56460.*`、`.codrax/output/20260924-075803.448-56459.*`，主代理与独立代理复核MD、answer-surfaces、HTML和root旁路，无修改原始回答。

| 产物 | gzip卡顿例SHA-256 | 字典例SHA-256 |
| --- | --- | --- |
| Markdown | `e2ee5c680e1d1b56a4496e684820785df88c66fa570f5a73947c2dd55ee67df5` | `5aad65b6ad097b7b861bdc70ef4fb255e4192e4a63976b1f5e2d782ab93bf852` |
| HTML | `c454653c246c887dce32787a6542ada3ec8185a505a74f76f98776b4f538d8c4` | `32da90ea0e8b526c00e1f2d1f7147ed902e6c5ce419adfc5dea14515066c81c4` |
| answer-surfaces.json | `5caebcb4c469667b9b7fd8852f2adba4f8d6f403a13a2503ca9141c726baa581` | `2ee00ab199c7500ad1387bb30ff367958a51c2b904b76a2258e5387d7805de04` |
| 模型日志 | `2748dc16956a56586dff2e81e56e0b23591de0dc71c4e1d270c7e8fbb60c1f57` | `d952a5d31775e0468b45d3ad1db140b58aacbca2174b726bc663aae9190c5227` |
| root-causes.json | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |

两份根因旁路都131字节、schema_version=2、root_causes=[]、reason_code=`trace_root_cause_contract_not_active`。这是正常非因果答案，不是`valid_model_root_cause_selection_unavailable`。没有提前降级或超时取消，600/300/600与活跃流保护未改。全部未通过保持开放；供给充分的次要成文错误不无限阻塞下一高ROI能力。
