# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T10:55:48Z
- sweep_start_ts: 20260820-035546
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double | FAIL | eval/results/github_issue_nlohmann_long_double-20260820-035548 | write_apply,answer_regex | none | 149s | 26 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 两份目标头文件均正确改为 `%.*Lg`，测试文件未改，`make check` 两次 exit=0，changed-path coverage 也完整；但累计复核生成 `project_test_assertion_not_observed` 后，终验域仍为 required_typed_contracts=0。该不可关闭义务把正确交付误判为 verification_proof_incomplete，属于确定性写状态机合同接线 P0。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-035548 | answer_regex,answer_contains | none | 233s | 32 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 12 个生产实现类型、文件与 Observe 定义均完整且图方向正确；但首稿 reader label=`实现` 与 body=`implements` 不一致被拒后，模型通过删除 visible_label 让 Mermaid 直接显示内部枚举 `type_relation`，同时系统仍判通过。读者语言约束可被删除绕过，图虽结构正确但客户表达不合格；末尾还出现泛化的“部分验收未达标”提示。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
