# HMC §204：HiSys原始行身份与运行时图要求人工审计

- date: 2026-10-08T02:25:26Z
- sweep_start_ts: 20261007-192524
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_hisys_graph_20261007

冻结代码`850a03cf2`（含HiSys `5ae5808da`）。恰好2并行×1，没有第三例。机器1 PASS / 1 FAIL；完整人工0 PASS / 2 FAIL。SQL/README真值不进入模型上下文；原始日志、JSON、失败结果不覆盖，不用确定性回归替原live签绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_existing_sqlite_hisys_row_identity | FAIL | eval/results/hmc_hisys_graph_20261007/trace_existing_sqlite_hisys_row_identity-20261007-192526 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 121s | 41 | read=1,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 6行精确ID到finalizer；成文漏源行号，JSON恢复丢列名 |
| 2 | trace_query_sleep_dependencies | PASS | eval/results/hmc_hisys_graph_20261007/trace_query_sleep_dependencies-20261007-192526 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 216s | 44 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 时间缺边正确，最终无图；范围/优先级资格仍有系统问题 |

## 1. HiSys：供给准确，成文与结构恢复未过

独立oracle为`eval/fixtures/hmosperf_hisys_row_identity/{capture.sql,README.md}`。窗口[2,2.08)内六行：2.010秒四条同内容MEDIA_START，hidden rowid依次为-3、0、9007199254740993、9223372036854775807；2.040秒CAPTURE_DONE为23，2.060秒为11。声明id重复，不是唯一物理身份；窗外1.9/2.1两行不计入。

该例`run-1.logs.all.log:1186,1194–1202`为成功查询；blob `.codrax/blob/20261007-192528-000-87330/trace-query-result-2b9eae08.json`保留全部精确值，最终typed display_rows见日志2022。日志2019已有精确数值字符串教学，故不能称parser/handoff丢失。Explorer在1309把负值、零、大整数解释为溢出/无法回表；最终`run-1.primary.md:3–19`只给`capture.systrace:14–19`且称原始记录。六条计数、时间、域、名、内容正确，无合并、伪CPU/TID或因果根因；未把枚举完成说成采集完整。

日志2142外层只有`blocks`，其值是畸形JSON字符串，尾部出现不归任何block的`columns`。`repairBlocksAsStringDetailed`按花括号恢复两个block，Lossless只比block数量；拖尾字段恢复不覆盖columns，接受后表头变“项目/列2…列6”。不是schema要求root columns，也不是本例root未知metadata删除。通用方案应检查可见载荷所有权，唯一结构关系才归并，歧义保草稿并请求局部修补；不能按最后一个表格猜父对象。

范围提示也有系统矛盾：同receipt source.query_window_known=true/[2,2.08]，日志2029–2035又说实际查询范围未知；1973–1977用blob读406/437行描述枚举不完整，与同窗6/6 rows_complete应分层。需统一原表、查询成员、显示页、采集完整性的结构化范围。

源DB运行前后Git blob均`36eb47c1bb4de242dfca5d0100fafd4832a68ea8`，SHA-256为`a1c4efd2ba3e4712eb8b19f061e82bf48c68f3a651958027b1100532290b5e1e`，未修改。行号误述已有充分供给证据，保留失败但不加本例特定prompt，不凭单次运行断言纯模型波动。

## 2. Sleep：时间误述改善，缺图和上下文资格未过

真值为5.012 dep3→dep2、5.020 dep4→dep2、5.0288 dep5→dep2、5.040 dep2→app。app睡眠40ms；dep2四段10/6/5/0.8ms，第三段结束为5.027 sched_switch、没有对应唤醒。正文正确保这些量和缺边，不再借dep5给第三段，但整体仍FAIL。

- 日志829/833–834接受no_named_target；1330–1331等8次查询都显式给thread，本批缺选择器修复**未在此live触发**。查询全部扩到4.998–5.045，不是用户[5,5.041)；2070–2071以no_typed_target跳过补证。
- app链JSON `trace-query-result-5cf1f5c7.json`仅3条边，dep2链仅2条；默认MinDurationMs=1排除0.8ms分支。census有该事件，但未用event_search取得独立事件凭证，裁剪不能算不存在。
- 日志2637仍为Diagram Preference，2774没有合格事件对；2934无锚初稿被拒，3043 remove d1，3048接受。最终0图，不能因被拒初稿有mermaid修复标志就宣称图可渲染。本批共享图支持公开回归通过，不等于本例上游供证已补齐。
- `run-1.answer-transcript.md:19`仅凭低优先级关系称dep3/4反转候选，又称dep2低CFS→app高RT“正常、无反转”，缺runnable/执行供给证据。生产链边当前会仅因lower_priority_waker发`priority_inversion_candidate=true`，是系统诱因，不能归为纯偶发措辞。
- `run-1.answer-transcript.md:30`把“未捕获”写成窗外行为，还称dep3/4仅作唤醒方；fixture实际有它们sched_switch。缺记录不能推出窗外。
- 上下文日志2703说省略95条越窗观测，2723起Coverage和Requested Fact Authority又带同批扩窗观察，多入口范围/资格未统一。

root旁路`.codrax/output/20261007-192859.942-87329.root-causes.json`为`root_causes=[]`、`trace_root_cause_contract_not_active`，正确未启rank；没有selection unavailable或超时降级。正文问题不能用旁路正确掩盖。

## 3. 回账与后续

保63稳定开放、5验收父项；本批2份完整答案FAIL挂原16.4/12.5/04.5/17.7/18.2，不新建重复任务。下一能力轨优先08.5 CPU状态×频率联合区间，随后03.3完整native栈、02.3逐工件索引，避免17.7单个字段长期占用能力轨。下一系统轨优先统一范围/资格投影和可见字段恢复所有权；优先级反转候选需已有链上调度/供给证据，不增加用户问题约束或扫描散文硬门。

源时间与链资格、JSON结构、模型误述分开记。不降低图硬门、不把近邻当因果、不自动改用户窗口，不为此两例补第三次求绿。
