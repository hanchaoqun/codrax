# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T10:55:20Z
- sweep_start_ts: 20260805-035518
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260805-035520 | log_regex,answer_regex | none | 155s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `{"ids":["u1","u3"]}`；两份必需材料、3 条规则、5 条决策、2 条贡献、reconcile 与 final projection 均有 typed 闭合。过程有 3 次 repair：先补材料 DAG，再拒绝互相冲突的 filter aliases，最后拒绝 unsupported `include`；每次失败都给出精确参数/阶段信息并收敛，未发现错误结论或系统代写。 |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260805-035520 | write_apply,answer_regex | none | 350s | 19 | read=18,repo_map=4,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-safe | 首次 `checked_sub(n+1)` 被 repository checker 正确打红并触发 replan；第二版改成 `(current_length.saturating_sub(1)).checked_sub(n)`，虽然 Python checker 通过，但空 iterator 上可得到 index 0 并访问 `items[0]`，补丁仍错误。Codrax 因 Rust runner 缺失且 Make 未声明三个 changed paths 而正确终止为 `unverified`，没有假签绿。确定性缺口位于 eval oracle/声明，不在产品 verification fail-closed。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
