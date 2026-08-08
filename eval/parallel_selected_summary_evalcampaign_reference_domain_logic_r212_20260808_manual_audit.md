# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T13:10:28Z
- sweep_start_ts: 20260808-061027
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260808-061028 | log_regex,answer_regex | none | 182s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格纯文本为 `17,0,5`；4 条 contribution 分别为 GroupA=10+7、GroupB=4、GroupC=5，inactive r3 与 unmapped r6 未进入贡献，reconcile 与 complete-reference grounding 均 pass，GroupX 正确补零。共 10 data rounds、2 repairs、5 action failures，主要仍是 DAG rank/输入依赖规划噪声。本轮从一开始就显式选择 `reference_key_field=canonical_label`，没有发生 `reference_ledger_domain_mismatch`，故 S37cb 的 `reference_domain_field_candidates` 正向修复臂未被生产命中；B349 也没有 foreign-param/schema-invalid witness。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-061028 | answer_regex,answer_contains | none | 236s | 38 | read=11,repo_map=5,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=2 | fail | Mermaid 经窄语法修复把节点换行转为 `<br/>`，可渲染且未改语义；四阶段成员和大部分职责正确。但用户明确要求的 data flow 出现核心错误：图把 Finalizer 的整个 `StageOutput`（含 `FinalAnswer`）画成经 `applyStageOutput` 写入 `BusContext`，而源码明确说明 `applyStageOutput` 不捕获 `FinalAnswer`，scheduler 直接消费后写入 task result；正文又把 `StageBinding.Terminal=true` 误述成写入 `BusContext`。Analyzer 把“各组件责任”误判为单一 role lookup，系统于是强制一个无关 scalar/单引用合同；Finalizer 虽拿到字段 roster 与源码池，却没有 field-level transfer/exclusion 权威，presentation-only 边逃过关系校验。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Exactly two cases ran concurrently; no third case was launched.
- Human result: `data=PASS`, `read-architecture=FAIL`; runner 的 `2/2 PASS` 只证明字面 oracle，不证明数据流语义正确。
- `EVAL-B351/S37cb` 本轮为 production no-witness：模型直接选择了正确的 `canonical_label`，没有 domain mismatch，自然不会出现 soft candidate。
- 新立 `EVAL-B353-ARCHROLEFLOWAUTH1=P1`：概念架构/工作流的逐组件职责与数据流不等于“从线索中选一个单一角色”。错误 role profile 会强制 scalar 合同；同时 field roster 只证明字段存在，不能证明所有字段都经同一 merge path 落到目标 carrier，更不能覆盖源码中的显式 exclusion。
- 最优修复分两层且保持模型所有权：(a) Analyzer 软教学把 component/stage roster + responsibilities/data-flow 与 single-winner role lookup 分开；(b) Explorer/Finalizer 软教学要求 data-flow 边由 producer/merge/consumer 或 typed relation/flow 证据支撑，并保留显式不流转字段。不得扫描用户/答案关键词设硬门，也不得由系统重画或替换模型图。
- 本批没有 Trace 输入或 Trace 代码变更；显式时间窗、因果投影、自动补齐、唤醒链、根因排序、真实占时/规则可消双轴保持隔离。Trace 主因仍只能来自 typed on-chain 席，邻近/背景只能支撑额外排查方向。
