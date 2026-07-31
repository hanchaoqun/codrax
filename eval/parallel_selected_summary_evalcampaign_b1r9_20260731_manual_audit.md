# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T08:01:49Z
- sweep_start_ts: 20260731-010149
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260731-010149 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 173s | 41 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | R19/R20/E2 均生效：无 raw observation dump、无跨主体算术假警报、runner 正确拒绝事实错误。正文第三段写成 34579.471372..34579.471743/0.371ms 且称无 blocked_reason，冲突于 typed authority 的 34579.471372..34579.471722/0.350ms/caller。R8 未拒绝，因为三段位于未标 surface_role 的 section，checker 只扫描 summary/显式 principal，形成 R21 通用结构接线 gap。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260731-010149 | write_apply,answer_regex | none | 203s | 18 | read=6,repo_map=4,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 单一 ChangePlan、一次 apply、一次 verify；`!== undefined` + 保留 `??=`，false/0/空串测试与 existing-default 负例由 make check 通过。Node probe/npm unavailable 正确降级，未 replan。两轮 write-analysis 与一次不可用 repo-map/read 仍属 W5 P2 效率债。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
