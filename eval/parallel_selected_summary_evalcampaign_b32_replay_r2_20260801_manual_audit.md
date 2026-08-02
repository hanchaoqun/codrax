# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T01:45:43Z
- sweep_start_ts: 20260801-184542
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260801-184543 | log_regex,answer_regex | none | 49s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 再次经历首批漏读 instructions→第二批补读；修复后 live state 为 coverage/answer/decision=complete，旧失败只在 terminal `last_nonterminal_error` 与历史事件中保留。最终严格 JSON 正确。 |
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260801-184543 | log_regex,answer_regex | none | 137s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终 30、5 条决策、2 条贡献及 reconcile 30=30 均正确；但最终 result 已使 ledger/output 全部 satisfied 时，评估前状态卡仍发布上一结果的 `continue_data` 和“贡献/对账缺失”理由。模型再次自行纠正，暴露 stale evaluation current-authority gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed findings

### `data_json_strict_ids`

这是 `EVAL-B32-DATASTATE1` 的同型正证。运行仍真实经历两轮：第一批没有消费
`instructions.md`，执行器拒绝；第二批补上 `read_text` 后消费两份材料。修复后 evaluator
只收到空 current violation set 与 `decision.status=complete`，不再需要从冲突字段推断哪一
面可信。terminal 仍记录 `last_nonterminal_error`，所以修复没有删除审计证据。

### `data_join_entity_reconcile`

不同拓扑的业务结果完整：exact join 将两个 Alpha 别名映射到 canonical Alpha，逐行计算
`qty*unit_price`，最终 5 条决策、2 条贡献、对账 `expected=30/actual=30/pass`，纯单行输出
`30`。但最后一批 `assemble_answer` 已生成新 result 后，构建 live state 的
`latestDataTaskEvaluation` 仍无条件向后取上一批 evaluation，导致用户可见评估状态卡仍为
`continue_data`，reason 还声称 decisions/contributions/reconcile missing；同一 typed state
中的 ledger/output graph 实际已全部 satisfied。模型读取新图后正确发出 complete，因此
runner 未发现过程矛盾。

通用修复 `fc533bfb8` 新增 `ActiveEvaluationFromRecords`：evaluation 只在尚无更新执行结果
或失败时拥有 current decision 权限；新 outcome 到达后旧 judgment 退为历史，直到该 outcome
附着自己的 evaluation。判断仅消费 `Result/Err/Violations/Evaluation` 的结构字段，不读取
任何请求、错误、reason 或答案文本。answer-face 的 sticky repair contest 继续由专门的
open/clear authority 维护，不走这条普通 current-evaluation 投影。
