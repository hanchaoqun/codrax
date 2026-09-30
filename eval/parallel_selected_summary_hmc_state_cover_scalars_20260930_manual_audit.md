# HMC §201：状态普查去重与 SQLite 原始字段人工审计

- date: 2026-09-30T09:11:01Z
- sweep_start_ts: 20260930-021059
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定62633，恰好2例并行×1，无第三例。live快照`.codrax/tmp/codrax-selected-20260930-021059`，生产`b503127d3`；后续`b172b43cd`仅契约测试。机器2/2 PASS，完整人工0/2，原始答案/日志/机器判定不改写。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_existing_sqlite_hisys_scalars | PASS | eval/results/trace_existing_sqlite_hisys_scalars-20260930-021101 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 102s | 38 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 8行表正确；完整性推断、TID0解释及坏时间行披露错误 |
| 1 | trace_query_sleep_dependencies | PASS | eval/results/trace_query_sleep_dependencies-20260930-021101 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 244s | 38 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=7,inv=1/0,fin_reject=6,unavail=0,prune=0 | FAIL | 21.8ms总数正确；四段明细错、缺真实终点箭头、同线程多生命线 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 1. 睡眠依赖：确定性修复与live验收分开

公开双PID查询回归复现21.8ms总体与10/6/5ms局部合成42.8ms，并验证新覆盖凭证修复。**live没有执行该双目标查询组合，不能据正文21.8ms就称该聚合分支已获live验收。** 第一次wakeup_chain未带目标PID，输出窗口普查/状态提示；第二次无时间限制的sched_wakeup检索得4条事件。未得到完整递归链，事件串不能自动升格完整因果树。analyzer2轮、finalizer8轮、6次拒绝/修补；产品wall242秒，最大上下文75493/200000。

原始调度行给出dep2四段睡眠：[5.002,5.012)10ms、[5.014,5.020)6ms、[5.022,5.027)5ms、[5.028,5.0288)0.8ms，第三段缺唤醒。答案写10/8/9/11ms并称末段未闭合/自醒。actual finalizer有总体、四唤醒及可运行段，缺完整目标状态逐段展开，探索摘要亦有错误状态/层数；不是已证明的纯模型波动。

图的两处系统接缝挂12.5/16.4/04.5 P1：

- 第4事件dep2→app@5.040000在用户窗内，但producer将观察包络5.012..5.040写成QueryWindow，消费者按半开右界删掉最后点，图权威/候选仅3条。需精确区分选择器和观察包络，不扩用户窗、不加epsilon。
- 防借证的事件身份又被当作生命线身份；最终11个participant，dep2重复4次、原5个声明留空，仅3条箭头。需分离事件凭证与实体显示身份，在修补时复用已有可证节点；不能凭同名合并跨源/跨代次实体。

内置Mermaid.js+本地Chrome原图解析/渲染PASS（SVG29376字节），截图`/tmp/codrax-hmc201-sleep-diagram.png`目视确认上述缺失和重复。语法PASS≠关系/可读性PASS。下批以无界/单点/显式右界、同实体多事件与跨源负控复现后统一修provider，不增加模型必填、不豁免未证箭头。

## 2. SQLite：表格正确，质量交接不完整

源表10行、8行合法时间，另ts=NULL/TEXT两行留SQL fidelity，不能放0点或判断属于所选窗。8行表保纳秒时间、域/事件名/内容。源TID有2个合法INTEGER（41007与0）、1个NULL、5个非法存储类/范围；源值0不授真实线程/CPU/进程身份。正文却称仅第1有效、其余7不可用/非法，又凭0断言内核角色。产品wall99秒，最大上下文75609/200000；finalizer一次发射零拒绝。

系统上下文缺口与模型误述分别挂01.3/16.4/17.7：

1. manifest `.codrax/trace-input-a58d3628aabbbc6a45dd1bcbd279f753/capture.tracebundle.json`明确rows_read=10/emitted=8、invalid_timestamp=2、invalid_source_tid=5，actual finalizer未收到坏时间统计。`traceBundleCoverageCaveats`保前24行及固定priority类，resolver/缺表清单在前，相关业务表被压缩。应按typed查询所用表/事件族优先投递覆盖和异常，不以原问句关键词硬选，不无界塞全部表。
2. `augmentPerfBundleWithTimeSemantics`仅由ftrace行生成“whole attached excerpt spans 0..0”，漏HiSys timed observation。应共享已验证时标并区分附件/片段/有效事件范围，不以合成哨兵冒完整采集范围。
3. 模型凭session1..8连续断言前70ms完整，又凭末10ms无事件断言采集不完整。无事件≠丢采，内容序号≠采集完整性。先补质量证据交接，保原FAIL，不做正文关键词门。

自然QUESTION未泄露系统防护规则。新scalar能力可用，不冒称全部表/17.7父项完成；不追加第三例追绿。

## 3. 原始证据指纹

| 原件 | SHA-256 |
| --- | --- |
| sleep primary | `a322285d1e803dbf1dfaa5bdf00d883f23dd065ccf1969dd75ef4bba6d24a93f` |
| sleep完整日志 | `bf2cef1b628b1bd470a2c2c02b9deb7e975937a9d1b92a0110f6aa8f0be0bda0` |
| HiSys primary | `f016c98623900348ea1408953df980a45ea2c1f7ace2448abe489bc8b601928f` |
| HiSys完整日志 | `f7498394a773398d1e48c831e133d70ce1bd025dc297187c67ac34f13864bf7b` |
| 浏览器审计JSON | `25059e7087947ad4f0157576f5b442c43666c648fe745b1ae758410ecdd4d9d7` |
| 原图截图 | `49fb88292a5c978891ccb047d2b9b9498d61781df6db0eaa21e5e286f0d56f48` |

79稳定任务、16完整实现、63开放、重复0；本批两组子能力/修复，完整人工残留2份，5稳定验收父项仍开放。实现/全仓/推送收据见主账本§201。
