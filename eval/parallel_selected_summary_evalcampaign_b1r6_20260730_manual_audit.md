# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T06:44:04Z
- sweep_start_ts: 20260730-234404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260730-234404 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 128s | 30 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Engine 的完整目标 occurrence roster 正确给出第 3 段 `34579.471372..34579.471722 / 0.350ms`，但 explorer 把 `sched_blocked_reason@34579.471723` 当起点并自造终点 `34579.471876`；错误进入 aggregate fact 后被 finalizer 原样发布。普通 Observation Ledger 只露出 roster 第 1 段，R14 的值级投影又因该 carrier 未被 repair ref 精确引用而没有生效。首个无界 `thread_timeline` 实际覆盖完整 artifact，但 finalizer 仍误报“无 typed whole-artifact supplement”。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-234404 | write_apply,answer_regex | none | 231s | 18 | read=6,repo_map=6,list=1,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass | 只有一个 ChangePlan、一次 apply、一次 verify；实现使用 `_prefault !== undefined` 且保留 `default ??=`，新增 false/0/空串回归，已有 default 保留负例通过。未生成 cumulative-review 修改批，W2 真实回放覆盖。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
