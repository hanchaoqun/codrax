# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T12:13:12Z
- sweep_start_ts: 20260829-051310
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260829-051312 | write_apply,write_patch_oracle,answer_contains | none | 104s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | ChangePlan 只含 main.go 一处 patch，实际 applied commit 为 1 insertion/1 deletion，`retrun` 精确改为 `return`；测试、changed-path coverage、fingerprint、recovery ref 与 clean worktree audit 闭合。write analyzer 首稿误造指向生产文件的 preserve_regression_test 约束，第二稿删除后通过，列低优先教学心智观察。 |
| 1 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-051312 | log_attachment,answer_contains | log_triage | 325s | 26 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=4/0,fin_reject=0,unavail=0,prune=0 | partial | B1455 生产转正：analyzer 首次即 generic+enumeration+bounded fact，零结构拒绝。前置路由却把 artifact-only 误设 current source required，Explorer 读取无关 cangjie_minimal fixture，并把日志 Bridge.cj:18 与样例定义 Bridge.cj:9 并置进终稿，制造假关联且使时长由 140s 增至 325s；确认新的上下文权威 P1。最终跨栈关系仍明确未证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
