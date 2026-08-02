# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T01:57:15Z
- sweep_start_ts: 20260801-185714
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260801-185716 | log_regex,answer_regex | none | 163s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终 `17,0,5`、4 条贡献、10 条实体解析与 reconcile pass 正确；但首个 assemble 只输出已有 GroupA/B/C 且格式不合规时，live OutputProjectionGraph/decision 仍报 satisfied/complete，后置 completion gate 才以 0/3 item reference gap 打回。 |
| 1 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260801-185716 | log_regex,answer_regex | none | 205s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 不同规划路径下仍产出 30、5 条决策、2 条贡献和 reconcile pass；最终 live decision 直接为 complete，旧 evaluation 未跨越新 result。3 个失败 action/1 次 repair 属过程效率观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed findings

### `data_join_entity_reconcile`

本轮先完成规则，再覆盖两个 CSV，随后逐批 join、derive、filter、contribution、reconcile、
assemble；顺序与 r2 不同。最终状态的 ledger/output/current decision 一致 complete，证明
`EVAL-B32-DATASTATE2` 不依赖固定批次形状。terminal 仍保留历史规划失败。耗时与失败
action 数高于 r2，但业务值、贡献和对账均正确，暂按模型/规划效率 watch，不设硬门。

### `data_multifile_reference_projection`

最终结果完整且可复算：inactive 被排除，10 条 entity-resolution 记录覆盖映射/应用，4 条
贡献形成 GroupA=17、GroupB=4、GroupC=5；按 targets 顺序补 GroupX=0 后输出
`17,0,5`，reconcile pass。

首个 assemble result 却是标签化的 GroupA/B/C roster，既非纯逗号数字，也缺 target
universe 中的 GroupX。模型正确返回 repair，后置 completion gate 也用 typed reference
candidate 精确拒绝并生成 zero-fill projection；但在调用 evaluator 之前，live state 的简化
OutputGraph 只看 answer/reconcile/projection artifact presence，未携带已有的 reference gap、
key count 和 answer item count，因此先发布 satisfied/complete。这是同一事实的双权威。

通用修复 `62c279edf` 将 completion authority 已有的精确
`OutputProjectionGraphInput` 接入 live reducer；`BuildWorkflowDecision` 同时固定 graph
blocker 优先于 `NextStage=complete` 的默认推导。系统只发布 typed incomplete-reference
事实和合法 `assemble_answer` 下一步，不修改或替模型生成答案。
