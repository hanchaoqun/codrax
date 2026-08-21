# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T17:22:07Z
- sweep_start_ts: 20260821-102205
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260821-102207 | typed_inventory_rowset,answer_contains | none | 300s | 33 | read=11,repo_map=2,list=1,trace=0,source_lens=2 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 4 个 @Entry 与 2 个 @Builder 的成员、路径和源码行全部正确；B1300 旧有跨行 row-id 拒绝为 0，未恢复旧稿。探索仍有 3 次 dispatch/17 iteration，但没有影响完整性。 |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260821-102207 | log_regex,answer_regex | none | 369s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B1301 已让未确权 reference candidate 返回 typed planner，模型与账本最终恢复正确 17,0,5；但终批调度器漏遍历 typed artifact 子级来源 lineage，把前三份已在前序 extract_records 消费的 CSV 误判为未调度，耗尽 6 次修复并以 failed 终止。B1302。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

1. ArkTS 最终答案完整列出 `Index`、`ParentComponent`、`StyledPage`、`ListPage`、`defaultHeader` 和 `GlobalCard`，逐项路径与声明行均可复核；
   `EntryAbility` 明确作为非装饰器入口排除。finalizer 0 reject，r817 中 `@component row-id outside allowed @builder` 的确定性合同冲突未再出现，
   B1300 获得生产正证。
2. 数据例不是模型波动。前五批已经生成 9 条 rule coverage、10 条 decision、4 条 contribution，reconcile 为 pass；模型后续明确读出 reference 顺序
   `GroupA, GroupX, GroupC` 及实际贡献 `GroupA=17, GroupC=5`，并形成正确目标 `17,0,5`。B1301 的“未确权 candidate 让位给模型”接线生效，
   没有再次被系统默认 present-groups 投影成 `17,4,5`。
3. `B1302-CUMULATIVEMATERIALLINEAGE1/P1` 是本轮失败根因。终批 `required_material_scheduling` 虽读取历史 Result，却只收集顶层 artifact；
   顶层 `labels_data/observations_data/targets_data` 携带 staged absolute source path，和合同中的相对路径不相等。精确相对来源凭证位于各自
   `*.csv#records` 子 artifact 的 `source_paths`，旧实现没有递归，因此把已完成的 typed 消费伪装成当前终批缺失。
4. B1302 根修递归消费 typed artifact lineage；`material_inventory` 仍只证明发现候选，明确禁止递归其 children 取得消费资格。没有 basename/suffix
   模糊匹配，也没有读取请求、模型 thinking、最终答案或错误 prose。回归同时钉住“前序 extract_records 子来源可累计闭环”和“inventory child
   不得签绿”，原 authoritative literal-script 未读材料拒绝继续通过。

状态：

`r818=runner-pass-1/2,human-arkts-pass+data-system-gap`；
`B1300=production-positive/core-closed`；
`B1301=decision-handoff-production-positive/core-closed`；
`B1302=implemented/dataworkflow+repl-full-suite-pass/pending-replay`；
`required-material-coverage=workflow-cumulative+typed-lineage-recursive`；
`material-inventory=discovery-only/no-consumption-authority`；
`reference-scope=model-owned`；
`request/model/final-prose-hard-scan=none`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/unchanged`。
