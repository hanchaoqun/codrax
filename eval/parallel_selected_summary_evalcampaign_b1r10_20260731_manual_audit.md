# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T08:11:49Z
- sweep_start_ts: 20260731-011149
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260731-011149 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 155s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 主答案精确列出 typed authority 的三段区间 `0.138/0.147/0.350ms`、总量 `0.635ms`、次数 3 与统一 caller；R19 无 raw observation dump，R20 无跨主体算术假警报，R21 没有误拒绝正确 roster。runner 唯一失败是 E2 oracle 强制要求 `时长（ms）` 表头，而本轮等价地在每个列表项重复 `ms`，属于评测呈现假阴性，不是产品事实失败。窗口约数与额外 wakeup/优先级叙述不改变所问事实，记为模型附加措辞波动，不新增硬门。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260731-011149 | write_apply,answer_regex | none | 218s | 18 | read=8,repo_map=5,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单一 ChangePlan、一次 apply、一次 verify；实现以属性存在性判断覆盖 false/0/空串，同时保留 `default ??=`，已有 default 不被覆盖。verification probe、npm unavailable 后 `make check` 通过，无修改型 cumulative-review/replan。两轮 write-analysis 与 repo-map/read 探索成本继续归 W5 P2，不阻塞 B1 correctness 收口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
