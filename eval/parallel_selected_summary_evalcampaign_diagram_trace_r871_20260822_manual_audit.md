# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T18:14:40Z
- sweep_start_ts: 20260822-111440
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-111440 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 精确窗、四态、四跳链及逐跳 CPU、11ms 链上 IO 第一席、三个独立 1ms 调度/优先级候选、实际/可消双账户、业务下钻和完整 Trace 因果投影均在；背景没有升主因，活动流没有固定时长降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260822-111440 | answer_regex,answer_contains,mermaid_edge_count | none | 408s | 39 | read=23,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=5,unavail=1,prune=3 | partial | B1355 无回归但自然正臂未触发：模型把字段读取误发成 assignment，typed 分类始终是 no_incident/source_operation_missing，第三次相同 blocker 正常收敛。正文责任较准，最终图只剩三条阶段 precedence，共享载体仍断开；首次 joint repair 还暴露了 decorated requested participant 被错发为可删除孤点的 B1356 自冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. Trace 人工通过。最终 `Trace 因果投影` 明确加冕 threadpool-400 iowait 11.000ms，保留四跳唤醒链、逐跳 CPU、三个不可相加的 1.000ms 候选、真实占时/规则可消双账户和链外背景隔离。模型对缓存页机理仍略有扩写，但同时披露等待对象、持有者和后端未证；不据单案扫描或改写 prose。
2. B1355 本轮为生产无回归、不是自然正证。模型在 extract_work.go 发出的三行都把“读取字段/调用 getter”标为 `anchor_kind=assignment`，且 subject/object 不是源码赋值两端；系统正确没有把它们升级为请求范围关系。最终 typed 状态仍为 `no_incident_typed_relation=[Mutable BusContext]`、`source_operation_missing=[BusContext]`，所以没有发生 `operation_missing -> local_operation_available` 阶段跃迁；第二个 Explorer dispatch 的第三次相同 blocker 收敛符合有界合同。
3. read 最终人工 partial。四阶段职责和三条 precedence 基本准确，Mermaid 语法合法；BusContext/Mutable 以显式 unproven 分组保留，但没有接入阶段，用户要求的数据流仍不足。正文末尾声称“图中以虚线表示其关联”，最终图已无虚线，是模型在删边后没有同步修改说明；属于可见答案漂移，暂不增加原文扫描硬门。
4. 新确认 `B1356-DECORATEDREQUESTEDORPHAN1/P1`。首次 draft 中 `bc["BusContext\\ninternal/types/context.go:7593"]` 是请求参与者 BusContext 的精确第一行身份，后续行只是展示元数据；participant coverage 能识别它。孤点候选 producer/patch executor 却对完整 label 做整串相等，故把 bc 发布为 `optional_orphan_cleanups`。模型按 typed joint repair 删除五条失败边并删除 bc 后，兄弟 participant gate 又以 `boundary_participant_not_visible` 拒绝 BusContext，形成同一结构事实的确定性自冲突并多耗至少一轮。
5. B1356 最优修复统一为共享的“第一行精确身份投影”：只裁掉 `<br/>`、`<br>`、Mermaid `\\n` 或真实换行后的展示后缀，并仅剥一层完整合法 inline-code 包装。producer 与 executor 都用该投影保护 request/boundary participant；它不做前缀、token、同义词或 prose 匹配，不证明关系，也不选择保留/删除。`MutableState` 不会被模糊当成 `Mutable`。
6. 五次 finalizer reject 中，未证 call/无 anchor 边、Mutable 主身份缺失和后续陈腐 participant edit 均是正确 fail-closed；B1356 只确认首次候选清单的结构自冲突。后续 `bc_sub/ms_sub/o` 已在不可变 base 中断开却被模型自行再次提交清理，executor 正确拒绝，不能把该轮也归因于 producer。
