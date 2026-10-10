# §219 共享事实资格与摘要生命周期：真实双例人工审计

- date: 2026-10-10T04:06:53Z
- sweep_start_ts: 20261009-210652
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_fact_authority_20261009

冻结源码 `aced23666e8a`，构建的 dirty 仅文档。两个自然问题不变，各运行一次、并行2，runner 正式 exit0；机器2/2、完整人工0/2。主代理与独立审计分别读取原件、工具载荷、实际最终消息及终稿；不追加第三例求绿，不修改原始答案，不倒签§216–218。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_measurement_records | PASS | eval/results/hmc_fact_authority_20261009/trace_measurement_records-20261009-210653 | log_regex,trace_attachment | perf_triage+trace_query | 139s | 39 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 33行到场且原生资格生效；正文猜单位/状态、未知终点延长、分组计数错误 |
| 1 | log_shared_sources | PASS | eval/results/hmc_fact_authority_20261009/log_shared_sources-20261009-210653 | log_regex,log_attachment | log_triage+log_query | 182s | 30 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 9事件到场并获原生引用；正文仅列6，借邻近进程身份、无证跨时钟排序 |

## 1. 日志：来源与原值交接通过，整答仍失败

原件基准见 `eval/fixtures/hmosperf_log_sources/README.md`：app 6条/7物理行，kernel 3条/4物理行，共9条/11行。app L4坏日期、L5孤立续行、L6未知文本不能消失或继承邻行身份；kernel L3只有level=3、boot纳秒9007199254741001及消息，无pid/tid/comm/wall。原件与§218 SHA相同，没有更换fixture。

过程日志 `log_shared_sources-20261009-210653/run-1.logs/codrax-20261009-210655-000-2859.log`：1290真实查询matched=9/omitted=0；2115–2123九个完整JSON均投递，包含malformed/orphan/unknown、精确boot纳秒和unknown身份；2160–2163原生支持名册列全9个ID。来源元数据及私有定位凭证正常；不是本轮又截断字段或漏kernel源。2126后的closure状态不复播模型正文。

独审确认1304–1317的explorer还逐项列全9条，遗漏发生在最终成文；1900已明确隔离旧triage summary/subject，1904–1913只保原文。不能把本轮误述归因于旧summary仍直接灌入；882还正确清除了虚构declared_count=5。

835/877本次analyzer主动发`is_granularity_question=false`，不再要求上一批的per_item肯定结论；这说明此例没有再次触发错误合同，不把它冒称新的精确重复quote软化分支live命中。该分支已由真实analyzer→合同→finalizer→emit公开回归验证。

最终 `run-1.primary.md`：

- 3–8仅列六事件，遗漏app L4–6；12又称app4/kernel2，实际所列是app3/kernel3。还称全部围绕rx17，而observer没有该ID。
- 5虽写无pid/tid，仍称“storage进程完成查找”；没有证据把该匿名记录归给邻行storage。
- 4无证计算跨日志“开始后1ms”；14、26、28确认跨源先后和壁钟对齐。年份/时区未明且无校准，kernel L3无wall，不能确认其在app缺失事件之前。各源文件顺序和source-local boot顺序可单独保留，不等于共享时钟。
- 正文保住“共享request不是因果证明”和peer边界，但不能撤销上述身份与时序越权。

上游triage首稿虚构cause被拒，后续删除是正常修正，不单独计整题失败；最终仍存在的问题以上述终稿为准。完整人工FAIL，挂16.4/18.4以及既有05.2时钟关系验收债，不另增稳定任务ID。

## 2. 量测：33行及资格完整，模型手写副本仍越权

基准见 `eval/fixtures/hmosperf_measurements/README.md`。请求窗[1,2)，13记录/7组；5个唯一引用过滤器与2条未解析记录分开。filter10与20在同一来源内同名，但属于不同过滤器序列；row5右边界排除，row14起点NULL无法定位。真实工具 `.codrax/blob/20261009-210655-000-2860/trace-query-result-1b640ad3.json` 42–43为窗口，501/668为未知持续，672–674为13/0/1覆盖计数。

过程日志 `trace_measurement_records-20261009-210653/run-1.logs/codrax-20261009-210655-000-2860.log`：2101/2106/2110/2114为7摘要+13成员+13时间线，33/128全到场、omitted=0；2102–2113明确单位未知、NULL/负时长终点未知、原始区间与交集及1条无法定位。2170–2174原生支持通道真实生效。2121–2130的closure状态不复播错误正文，既有runtime-only安全抑制继续生效，本轮无旧closure文本污染。2086–2091/2147明确无需源码，实际read_file/repo_map/list_files全0；入口required/mixed不能误报成最终仍强制源码。

最终 `run-1.primary.md`：

- 14–15把NULL/-1持续的时间点延至“窗口末”；61又说“不延长区间”，正文与免责声明矛盾。
- 27–29/57把整数1/0和文本"1"译为活跃/空闲，缺少状态码协议；未知不是肯定状态。
- 9/19推Hz，51/59仍断言334–800MHz GPU频点；51还把filter20的800000000混入filter10范围。单位与硬件身份未知，不能这么换算或归组。
- 5错误称4确认过滤器+3不明；37–41将已有唯一过滤器引用的vendor.sample归为来源不明，混淆资源语义未知和引用未知。
- 未披露1条无法定位时间的记录及右边界排除。首值科学计数3.342000005e+08与334200000.5等值，不计为精度错误；精确大整数、BLOB、NULL和文本原值仍保留。

2265/2270最终只emit summary+caveat，没有runtime_measurement，仍由模型手写表格。已提供充分正确数据和边界但未复用原生展示，与“工具不支持量测”“handoff缺行”不同。完整人工FAIL，继续挂16.4/18.4，不倒签旧失败、不称已证明偶发波动。

## 3. 本批结论与后继边界

确定性交接子能力通过：所选源优先的元数据预算；原生事实支持名册及typed窗外引用限制；过期closure独立审计/当前摘要单次advisory；精确重复请求义务软化及单项问题可答否。真实live验证部分实际路径，不用未触发分支冒充全覆盖；40/64来源预算、mixed摘要与局部投影边界由公开正反验证。

两例没有源码读取或最终格式拒绝，终稿事实错误仍未被机器判定捕获。下一共享缺陷应面向“原生数据默认展示复用、模型解释另列、精确引用与成员覆盖”，而非继续增加GPU/日志关键词禁令；主能力轨推进10.1双侧原生测量包。多源异常链的occurrence见证另留05.3/16.4，需要公开RED后再实施，不把静态怀疑算已发生客户故障。

原始结果目录、工具blob和验证日志归档于本批results目录；失败不覆盖。完整父项新增0、累计20/79、59开放，五稳定验收父项不与两例FAIL重复相加。
