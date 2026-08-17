# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T19:57:54Z
- sweep_start_ts: 20260817-125753
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree | PASS | eval/results/github_issue_libgit2_foreach_worktree-20260817-125754 | write_apply,write_patch_oracle | none | 197s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只修改 `repository.c`；两处均改为先赋值、后比较，负错误码不再压成布尔值；`make check` 与改后树 oracle 均通过。B1004 的等价实现 oracle 修复获得生产正证。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-125754 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 237s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 `2.000..2.020s` 窗、链上-only 排序、`#1=11.000ms` IO、三席各 1.000ms runnable、实际占时/可消除双轴、因果投影和自动补齐均在，B1005 的 exact-one scope 也到达 finalizer。但正文把附件窗长 `20.020ms` 误写成所选窗/目标非运行时长，与 typed `sleep=20.000ms` 自相矛盾；把 4 节点链称“四跳”；又把 kernel wait caller 与窗口内 on-chain IO 候选升级为“直接拉长整个链路”，虽 caveat 同页承认 completion/sync relation 未证。最终阶段仍同时收到完整 raw trace 和已完成 typed 查询，形成弱源重算强源的上下文权威冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
