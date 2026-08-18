# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T03:01:40Z
- sweep_start_ts: 20260817-200138
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-200140 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 346s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 精确窗、角色 CPU/prio、sleep→wakeup、worker 链上 8.300ms、双轴与 Trace 因果投影齐全；背景 pressure 未晋升主因，未把候选写成锁持有已证。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-200140 | answer_regex,answer_contains,mermaid_edge_count | none | 361s | 39 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=5/0,fin_reject=1,unavail=0,prune=1 | uncertain | B1038 生效：四阶段 spine 与 BusContext/Mutable local-only 使用同一 typed 分类，局部事实未冒充完整 flow，两个未证边界可见；最终图诚实但未回答共享载体如何贯通各阶段，故只判 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### Trace

- 最终答案只以 typed on-chain `worker-200` 为主根因席；`unknown-thread` supply pressure 保持背景，
  `worker-200` 的相邻 sleep 保持邻近。实际占用与规则可消除量分账，8.300ms 没有与 10ms sleep 或
  3.500ms 背景压力相加。
- 模型调查推理中一度使用了过强的“confirmed inversion”措辞，但最终成文按 typed caliber 收敛为
  “优先级反转候选”，并明确无锁持有/同步阻塞证据。这是上下文校准成功，不需要扫描或改写模型思维原文。
- 无 final reject、空答案、旧稿恢复、畸形 JSON 或 active-stream 固定 4ms 降级。

### Code relation diagram

- Finalizer 收到精确同源分类：`request_scoped_incident=[analyzer explorer extractor finalizer]`、
  `local_typed_incident_only=[Mutable BusContext]`、`requested_relation_spine_status=unproven`，并生成两个
  boundary recipe。strict validator 不再把独立 local pair 当作完整 request scope。
- 第一稿包含未证 `Orchestrator→Analyzer`、无锚 `BusContext→Mutable` 以及 metadata/body 错绑的
  `BusContext→Extractor`；validator 一次拒绝后，模型只替换 diagram，保留六个责任说明块。最终保留
  三条 verified stage precedence 边、BusContext/Mutable no-arrow ownership group 与两个可见未证边界，
  没有伪造 bridge。
- 用户请求的是“各阶段与共享载体之间的数据流”，最终图只证明阶段顺序，未画出共享载体的真实局部
  operation，也没有完整 carrier spine；因此 runner PASS 只能判结构通过，人工为 partial/uncertain。
  现有 prompt 已提供 local operation binding，模型在 repair 中主动删去可选局部边；暂按模型 authoring
  波动观察，不用硬门强制留边或由系统代画。
- Explorer 仍在 `flow_participant_coverage` 连续三次无进展后 force-complete，并把
  `result_kind=resolved` 与“required relation remains unproven”同时发布。好的一面是 precise debt 已完整
  进入 Finalizer 且变成可见边界；未闭环的是内部完成状态语义与 stale repair 清理，归 B1039 单独修。
