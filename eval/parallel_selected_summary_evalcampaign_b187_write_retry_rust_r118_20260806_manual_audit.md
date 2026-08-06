# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T19:17:23Z
- sweep_start_ts: 20260806-121722
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_trait_impls | PASS | eval/results/sr_rust_trait_impls-20260806-121724 | answer_regex | none | 86s | 20 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正确枚举 LiteralMatcher/RegexLikeMatcher，并给出 `fixed=true/false` 的选择条件和 struct/impl 行号；0 成文重试。显式引用清单只选了两个 struct 行，没有选择分支与 impl 行，正文表格虽有精确位置但 claim-to-citation 映射偏弱，登记通用引用覆盖观察项，不以 Rust/type 关键词硬门。 |
| 1 | github_issue_libgit2_foreach_worktree | PASS | eval/results/github_issue_libgit2_foreach_worktree-20260806-121724 | write_apply,write_patch_oracle | none | 126s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划和实际 patch 均精确保持第一处分支 `(error = cb_result) != 0`，第二处为 `(error = lookup_result) < 0`；三测试及 runner oracle 通过，一代完成、无 replan。write analyzer 首稿合格，因此本轮证明客户可见回归闭环，但未现场命中 S19 retry 臂；该臂另以编排级精确 pin 覆盖。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
