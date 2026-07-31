# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T05:45:39Z
- sweep_start_ts: 20260730-224539
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260730-224539 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 137s | 37 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 新 principal oracle 正确拒绝假 PASS。模型把未给时间边界的全 trace 请求自行收窄到 `34579.450627..34579.470000`，第三段从 `34579.471372` 开始，被范围权威错误排除；窄窗内引擎诚实返回 2 段/0.285ms。R12 set/leaf 只以 observation ref 进入 Typed Repair Handoff，成员值和 notes 未进入 top-10 ledger，最终正文继续自行算成 2 段/0.351ms。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-224539 | write_apply,answer_regex | none | 379s | 17 | read=10,repo_map=3,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | 首个 plan 已正确用属性存在性检查并补 false/0/空串测试；系统仍追加 `batch-1-cumulative-review` 第二修改 plan，把 `??=` 改成无条件赋值、重复追加 4 个测试，并要求依次 cherry-pick 两个 plan。测试绿掩盖了“已有 default 被 prefault 覆盖”的语义回归；W2 已由控制面冗余升级成用户补丁错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
