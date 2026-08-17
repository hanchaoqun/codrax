# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T19:48:57Z
- sweep_start_ts: 20260817-124855
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | eval/results/github_issue_libgit2_foreach_worktree-20260817-124857 | write_apply,write_patch_oracle | none | 111s | 25 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (runner false negative) | 只改 repository.c；先保存 cb_result/lookup_result 再比较，make check 通过且 workflow verified。FAIL 仅因 line-oriented oracle 不接受跨行的等价安全实现；已改为 folded-text 语义等价判据。 |
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-124857 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗口、typed 链、#1=8.300ms、两轴、因果投影与自动补齐均保留，0 final reject；B1003 新图例已出现。但模型仍把 dependency runnable delay 写成“低优先级任务占据调度资源/目标必须等它完成调度”，与 final typed mechanism ceiling 冲突。进一步确认 reader causal scope 把互斥 allowed calibers 合并成一句，形成系统合同矛盾；已改为 exactly-one 选择。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
