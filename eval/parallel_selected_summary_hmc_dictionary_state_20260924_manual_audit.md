# 引用字典与累计状态口径：固定双例人工审计

- date: 2026-09-24T13:33:25Z
- sweep_start_ts: 20260924-063323
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

构建revision `e15b107514ef-dirty`，时间`2026-09-24T13:32:37Z`；dirty仅为待汇总文档，Go输入已提交冻结。快照`.codrax/tmp/codrax-selected-20260924-063323`，36450严格2并行×1正式exit0，机器1/2。不追加第三例，不回写原机器判定；全文审计与供给准确性分别记录。独立oracle/SQL均未交给模型，未要求用户背诵系统防错清单。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260924-063325 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 250s | 42 | read=2,repo_map=1,list=1,trace=1,source_lens=0 | midloop=8,inv=7/4,fin_reject=1,unavail=0,prune=0 | FAIL | 时间/已解析名称正确；未知启动名及未解析系统记录未披露，消费者限制未进入查询/最终上下文 |
| 2 | trace_query_scheduler_concurrency_distribution | PASS | eval/results/trace_query_scheduler_concurrency_distribution-20260924-063325 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 293s | 42 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 数值/3ms桶/开放尾显著改善；仍漏upload就绪段并把已确认总体零贡献扩称全局无运行 |

## 1. 调度：原系统口径缺口修复，完整成文仍未通过

行号以`run-1.logs/codrax-20260924-063327-000-30030.log`及`run-1.primary.md`为准。对照`eval/fixtures/hmosperf_scheduler_concurrency/distribution.expected.json`全量核查，而非仅检查0.8/0.5字面。

- 1189第一次查询真实传`bucket_ms=3`，窗为1..1.01；其后1779重查漏该参数而返回默认100ms。两套query身份/范围/桶规格均保留，没有把后一次默认结果覆写成3ms。10次查询、2次探索；不是只看最终一次query判断全无3ms供给。
- 终稿13–18四个桶、最后1ms尾桶均正确；Runnable峰值/均值2/0.8、Running2/0.5以及深度分布4/4/2ms和6/3/1ms均正确。相比§182本轮数字改善，但不是受控A/B、不能把效果全部归因本次修复。
- 最终上下文2694–2702明确每条state_drilldown累计量及开放尾，render运行是2段/3ms且起点裁剪1；unfinished104是开放尾1段/1ms，measurement_scope非实际端点。2823–2832短卡优先保累计/非连续语义。原closed成员和四视图保留，2734/2774完整包含upload的1.006..1.007就绪1ms。
- 3001普通块提交、3067补普通成员表，未选择`runtime_measurement`。终稿48–56只列upload运行/睡眠，遗漏其已确认就绪段；供给并未丢失，不能再归工具缺成员。
- 终稿40断言“CPU利用率偏低：60%的时间没有任何线程在运行”，将已确认闭合区间的零贡献扩成全系统无运行，与3/42行自身的未知覆盖说明冲突。没有根因加冕，但仍是统计解释越界，完整答案FAIL；不靠是否背诵防护句判错。
- 终稿开放尾明确标记，未像§182那样纳入闭合总体。这部分系统修复真实到场并有成文改善；原§182人工FAIL不倒签，08.4仍保持验收中。

输出`.codrax/output/20260924-063816.308-30030.{md,html,root-causes.json}`完整。HTML复制普通表/文字；无diagram，未作浏览器截图视觉验收。schema2旁路空roots、`trace_root_cause_contract_not_active`，符合纯测量问题，不因旁路存在就签正文PASS。600/300/600及活跃流保护未改，无4分钟空正文主动降级。

## 2. SQLite：原始名称身份不确定性未随观察传递

机器250秒FAIL，原正则结果保留。新场景复现源码可选却被合成源码清单的独立系统缺陷：887模型没有source_inventory_profile，888系统自动补全，938/1247又判none/allowed_optional；1371、1624完成硬门却强制源码。1552/1553两个跨夹具目录find均被1555/1558范围门拒绝，未返回文件列表；成功只读隔离仓库README与目录信息。未见expected.json、capture.sql或夹具说明内容入模。该缺陷与旧scheduler同源，升为下一批高ROI修复；不得通过在自然问题中补“不要读源码”规避。

对照独立`expected.json`和真实输入/manifest/query/最终上下文/MD/HTML逐层核验：

- 四段时间/耗时8、4、4、5毫秒与三条已解析HiSys的时间、域、名称、内容都正确，无窗外混入。8ms/5ms正则未考虑毫秒已在表头，属于两个机器误报；不改原 verdict，也不以此抹去以下真实FAIL。
- 报告24–25行把两个`startup`兜底标签当阶段名，未交代分别来自NULL引用和重复字典身份。没有错选`FirstDraw/CacheWarmup`，但“名字未知”丢失。系统事件表没有披露1.050、1.052秒另外两条身份未解析记录；原SQL保真systrace物理36行仍保存原始字段/内容。
- 完整`capture.tracebundle.json`1043行保`start_name_null_reference=1,start_name_unresolved_reference=1`，1188行保`domain_id_null_reference=1,event_name_id_unresolved_reference=1`。但`traceBundleCoverageCaveats`（parse.go:2821）只取前24条及2条优先记录，2901优先分类不涵盖消费者身份缺口。因此一般字典拒绝计数到场、与当前事件直接相关的名称限制未进原生查询或finalizer；日志2557虽完整11/11事件行、零省略，仍缺上述身份信息。应沿消费者观察传递精确身份状态，而不是只提全局caveat上限、加提示或扫答案关键词；不能预归模型波动。
- 两条未解析HiSys虽然保真，却没按真实行时间进入本次查询人口；不可借SQL载体外层时间伪造事件时间。继续归05.1/17.7未知名称可逆编码，01.3/16.4接消费者状态交接，不新增重复任务。
- `event_search_coverage.scope_time_*`仍写匹配包络1.010–1.058而非请求1.000–1.080；本次正文区别正确，不等于typed范围合同已闭环。保留01.3/16.4窗口交接待修。
- 只调用`event_search`，耗时由模型成文后人工核对正确，未命中原生业务树配对；不能称04.2/04.3确定性配对交付验收。2737最终patch因未发布`add_facet_id`被拒、保留首稿，原过程失败不删。

HTML正文与MD一致，无额外渲染损坏，无diagram且未作浏览器截图。schema2空根因为`trace_root_cause_contract_not_active`，符合信息整理问题；原输入SHA与准备收据相同、既有SQLite入口明确不调用转换器。本批能力的确定性测试通过与自然答案完整FAIL分别登记，人工最终0/2，不追第三例。

## 3. 证据摘要

| 工件 | SHA-256 |
| --- | --- |
| 调度primary | `0fb17e7ece046899dd9bf177a1690a088b36b66059a48db697a7690f49b00fe1` |
| 调度日志 | `54648ce8db615ae5f304fc64e0fc2d01eb25a7743d9851d7c0e3e1a895b78b06` |
| 调度HTML | `a13b201486f22c3f8b3f2713ec08d8c35121c28a591f46603091421273f695da` |
| 调度root-causes | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
| SQLite原输入capture.data | `91ba8e876d409ddbde61e2dfe701f6051d0a41c69b00bbfe609c421ff08fc756` |
| SQLite独立oracle | `a60603f1e9422d33a94f79b2ae9fc49385285f79d1b59c4642604a774c703a1e` |
| SQLite报告MD | `dd6e68dbc7f08e8c99d919e373c4c56b80166e25bb28bfa0d2cfd3d7d5670ca2` |
| SQLite报告HTML | `86d68614f3cb4f8485e4d480c3d7b4ff3e3a722b4d700514cdedad05fc3a05dd` |
| SQLite根因旁路 | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
| SQLite准备manifest | `5b665e25a2d734d7cab47c2b7ad00b1d3a7b833fcaea2da43754f3637f498de6` |
| SQLite原生查询JSON | `8443399e7f2398fc6680d4481b0c28e9285179a2056dc5a946b950a5fe5efeea` |
