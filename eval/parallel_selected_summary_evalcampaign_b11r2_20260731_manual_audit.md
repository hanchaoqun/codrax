# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T20:52:09Z
- sweep_start_ts: 20260731-135208
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-135209 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 191s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | AB2 的 pacing occurrence 窗已正确；AB3 把 on_chain/adjacent 两张榜合并为 duplicate_rank，正文继续自排错误榜位。Binder 1.409ms 值正确，但正文把 transaction 阶段窗 13762.834345–13762.835754 当成真实等待窗；typed root-cause 等待窗为 13762.835861–13762.837270。 |
| 1 | github_issue_libgit2_foreach_worktree_symptom | FAIL | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-135209 | write_apply,write_patch_oracle | none | 343s | 18 | read=17,repo_map=3,list=4,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 两处 C 修复已正确应用；TestSurface 选中 make@.，但 plan-touched `.c` 的 cmake 偏好又合成未配置 runner，并按同目录去重挤掉 make，验证在零测试前失败。后续无参 planner 检查虽通过 make，未形成原 applied plan 的 typed passing probe，no-change 恢复按设计 fail-closed。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
