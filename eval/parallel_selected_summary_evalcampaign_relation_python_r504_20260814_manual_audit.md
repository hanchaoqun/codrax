# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T19:00:21Z
- sweep_start_ts: 20260814-120019
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260814-120021 | answer_regex,answer_contains | none | 346s | 30 | read=17,repo_map=5,list=0,trace=0,source_lens=4 | midloop=6,inv=5/1,fin_reject=3,unavail=0,prune=0 | fail | 机器只校验类型名/文件名，未校验关系图的边。系统已有 12 条 typed `implements` 权威，validator 也正确要求实现类型→接口；finalizer 却只看到 lookup 形 `source=LoopController/member=实现类型`，模型把 12 条箭头及 anchors 全部画反。三次正确拒绝后模型误判为“Go 没有 parser 级 implements 证据”，最终删除所有边，图退化为 13 个孤立节点。B819 内联 link-text 修复已生效，本轮没有 synthetic node；新 GAP 是 typed lookup 方向到显示方向缺少 copy-ready 翻译。 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260814-120021 | write_apply,write_patch_oracle | none | 773s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 探索已定位 `relativedelta.py` 和测试，typed run state 为 `ready_to_plan`。但 write exploration 子流未把 TaskState stage 切到 Explore，explorer 产生的 file-coverage RetryHint 被误标成 WriteController 所有；该 hint 又自相矛盾（1/1、100% coverage，却要求继续读）。Controller 同时只看到 context pack 前 16 项和 `+67`，没有紧凑 handoff receipt，在 `plan_batch`/`explore_code` 间重复自辩。上游字节持续活跃，系统没有在 4m 固定年龄降级；11m25s 后 provider 以 `finish_reason=length` 截断 67 万余 transport bytes/252100 content bytes，零 tool call，required-tool stage 被 soft-stop 直接终止，源码未改。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generalized gaps

1. `B821-TYPEDRELATIONLOOKUPTODISPLAYDIRECTION1/P1`：typed roster 的 `source/member` 是查询集合方向，
   `type_relation` 的 canonical 显示/校验方向则是 subtype/implementer → supertype/interface。两者没有一个
   schema-native copy-ready 翻译层；模型必须自行翻转，容易在 JSON 中反向并在拒绝后删边。不能放宽 validator，
   也不能由系统代画；应把 typed 行生成明确的 advisory body-edge + `edge_anchors` 配方供模型选择复制。
2. `B822-WRITEEXPLORESTAGEOWNERSHIP1/P0`：write controller 调用 read explorer 时只把 stage 作为 dispatch 参数，
   没同步 BusContext 的 PipelineStage/TaskState.Stage。`applyStageOutput` 因而把 explorer RetryHint 盖上旧的
   WriteController owner，造成跨 agent 合同污染；StageReport 也存在同源错标风险。
3. `B823-EXPLORERREADINESSFALLBACK1/P1`：没有显式 completion signal、但 readiness 各面已满足时，retry builder
   落入 file-coverage 默认臂，能生成“1/1=100% 仍继续读”的逻辑矛盾。应使用 typed readiness 发
   `structured_completion_missing`，要求复用现有证据完成结构化 closure，不扩大读取面。
4. `B824-REQUIREDToolLENGTHNOTOOLRECOVERY1/P0`：write_controller 是必调工具阶段，但非空 no-tool 响应不走
   protocol controller；`finish_reason=length` 且零 tool call 时也被普通 soft-stop 接受。应隔离巨量草稿，给一次
   schema-only 纠错轮，只要求从当前 typed state/action enum 选择并调用 `emit_write_workflow_decision`；不得从草稿
   文字推断决定，也不得按固定流时长中断活跃流。
5. `B825-WRITECONTROLLERHANDOFFRECEIPT1/P1`：WriteExplorationHandoff 虽已落盘，controller 只通过优先级 pack
   间接看到，重要 target/evidence 可能落在 `+N` 后。应在 Typed write artifacts 中显式给出有界 presence/count/
   target receipt；它只降低心智负担，不替模型选择 plan 内容。

## 施工复核（2026-08-14）

- B822/B823/B824/B825 已在同一写控制面批次落地并通过 `internal/agent`、`internal/orchestrator` 包测试。
- B823 冷读纠正：r504 不只是“缺 completion”；普通源码 `QFRootCauseTrace` 被误套 bounded Trace endpoint 合同也是
  隐藏 blocker。现由 typed runtime profiles 区分源码根因与真正运行时 Trace，并将剩余 readiness 缺口分席提示。
- 活跃 stream 的时间边界、Trace 显式窗/因果投影/自动补齐、模型对 plan/答案/图的作者权均未改变。
