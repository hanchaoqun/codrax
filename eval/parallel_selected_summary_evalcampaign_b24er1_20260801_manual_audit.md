# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T19:45:18Z
- sweep_start_ts: 20260801-124517
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-124518 | write_apply,write_patch_oracle,answer_contains | none | 99s | 19 | read=1,repo_map=0,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 完成；applied tree 仍只有 `main.c` 的 `retrun`→`return` 一行。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-124518 | answer_regex,answer_contains | none | 207s | 25 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=3/2,fin_reject=2,unavail=0,prune=0 | fail | `required_mechanism_anchors=3` 且 `gate.RunWith` 不再满足 `gate.Run`；但 endpoint miss 仍以 `principal_support_member_omitted` soft advisory 出厂，最终没有单独裁定 `gate.Run`，机器 substring oracle 继续误报 PASS。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- endpoint 编译和 exact matching 已在真实复放生效：`required_mechanism_anchors` 从 0 恢复到 3，缺失集合明确包含 `gate.Run`、`analyzer.go`，`gate.RunWith` 没有再冒充 `gate.Run`。
- 仍人工 FAIL 的唯一 P1 原因是权限：pre/post required-anchor 复用了 soft-by-default 的 `principal_support_member_omitted`，日志明确显示 “accepted as soft advisory”。
- `EVAL-B24-ENDPOINT1` 从“编译/匹配缺失”收敛为“typed publication authority 缺失”；下一批新增只适用于 `QFCallChain` 的 `call_chain_endpoint_omitted` High/finalizer-local hard kind。
- read 耗时进一步降到 207s / 2 rejects；diagram authority 和旧附件清理继续通过。
