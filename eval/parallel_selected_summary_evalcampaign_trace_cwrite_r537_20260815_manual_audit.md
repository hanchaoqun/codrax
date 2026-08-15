# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T23:43:45Z
- sweep_start_ts: 20260815-164343
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree_symptom | PASS | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260815-164345 | write_apply,write_patch_oracle | none | 141s | 24 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 实际补丁把两处赋值整体括起，保留 -42/-7，且 `make check` 三臂通过、测试文件未改；但用户可见 ChangePlan 摘要仍把修复写成 `error = (call(...)) != 0` / `error = (call(...)) < 0`，这仍会先比较再赋值，和实际正确 patch 不一致。记 B864/P1-observe；不扫描或改写模型 prose，先用异构写案例确认是否为泛化上下文/展示问题。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260815-164345 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 215s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、Trace 因果投影、系统补齐、链上-only 排名、实际占时/业务 span 与规则计价双轴均完整；活跃 finalizer 流持续约 1 分钟仍正常收敛。模型把 caller `dma_fence_default_w` 过推为等待对象/所有者，属既有 B788 soft-teaching 观察。确定的系统 gap 是 EVAL-B13-AK3：typed 目标账为 11 个 D-state 区间/36.757ms，但镜像行把 4 个按 CPU 汇总桶渲染为 `4次(3.774~16.064ms)`；这不是发生次数，且与同页 11 次冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
