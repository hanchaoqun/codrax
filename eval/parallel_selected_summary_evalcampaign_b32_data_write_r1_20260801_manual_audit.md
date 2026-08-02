# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T01:30:52Z
- sweep_start_ts: 20260801-183050
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260801-183052 | log_regex,answer_regex | none | 44s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终 JSON `{"ids":["u1","u3"]}` 正确且第二轮真实消费两份材料；但修复前 live workflow state 同时发布 coverage=true、answer present 与历史 `required_material_not_consumed`/blocked，当前态与审计历史串线。runner 只校验最终值而漏报。 |
| 2 | github_issue_libgit2_foreach_worktree | PASS | eval/results/github_issue_libgit2_foreach_worktree-20260801-183052 | write_apply,write_patch_oracle | none | 163s | 19 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 隔离 worktree 只修改 `repository.c` 两处括号，保留 `-42/-7` 原错误码；`make check` 在 `-Wall -Wextra -Werror` 下编译并执行 1/1 通过，未修改测试、未走 unverified/fallback。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed findings

### `data_json_strict_ids`

第一批脚本没有调用 `read_text("instructions.md")`，执行器正确拒绝并登记
`required_material_not_consumed`。第二批脚本补读该文件，结果明确记录
`ConsumedPaths=[instructions.md, users.json]`，材料覆盖完整，输出投影满足且最终 JSON
正确。问题在于 evaluator 收到的 live `workflow_state_json` 仍复用了第一批执行失败：
它同时显示 `material_coverage_sufficient=true`、`has_answer=true`、缺失材料为零，又显示
`decision_status=blocked / required_material_not_consumed`。模型识别出这是历史状态后仍正确
完成；终态 journal 也正确将它降为 `last_nonterminal_error`，但 live current-state
projection 不应要求模型自行消解这种矛盾。

通用修复 `bab0fe0b4` 让当前 reducer 只消费“最近一次结构化成功进展之后”的执行违规；
`WorkflowViolationsFromRecordExecution` 仍保留全历史供 reasoning/journal 审计。成功进展边界
只判断 `Result != nil && Err == ""`，不读取错误文本、动作名、用户原文、模型 rationale
或最终答案。单元层与 REPL 接线层均固定：旧失败仍可审计，但不得继续出现在当前
`WorkflowViolations`、blocked action graph 或 current decision 中。

### `github_issue_libgit2_foreach_worktree`

最终 patch 精确为：

- `if ((error = cb_result) != 0)`；
- `if ((error = lookup_result) < 0)`。

写前 analyzer 为构造可验证的 behavior contract 进行了四轮，planner 又先尝试了当前不支持
的 C verification probe 后自行移除。这是低优先级效率观察，不影响变更范围、风险门、
隔离 worktree 或验证真实性；单次出现不立生产硬门，也不按 C/case 名拟合。
