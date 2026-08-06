# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T18:42:14Z
- sweep_start_ts: 20260806-114213
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-114214 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 181s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 最终答案保留显式窗、双轴根因、可消除量与 frame 因果未证限定，也不再硬分压力等级或跨关系加和；但把 typed 排名/唤醒邻接行串成 ThreadPool→Network→Cookie→目标的单一核心因果链，并以“持续控制调度节奏”等词越过现有边凭证。过程另有结构性 JSON 修复 gap：首稿没有 kind=summary，校验只要求补 caliber 字段，模型把该字段加到 section 后又被 schema 拒绝，第二次才新增 summary，造成 2 次可避免 reject/patch。 |
| 2 | github_issue_libgit2_foreach_worktree | PASS | eval/results/github_issue_libgit2_foreach_worktree-20260806-114214 | write_apply,write_patch_oracle | none | 253s | 20 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 两处赋值优先级错误分两代 plan 修复，最终 worktree 与测试正确；S17 已令第二代 plan 的同 ID fallback 由当前 acceptance 重建，finish 理由不再声称第16行不变。残余是 plan1 的 unique-ID fallback 仍进入累计域，且 Mutable context/header 与 controller typed-task 仍并置初代目标，属于同一代际单源 gap，纳入 S17b。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
