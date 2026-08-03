# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T12:05:42Z
- sweep_start_ts: 20260803-050540
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260803-050542 | write_apply,answer_regex | none | 310s | 19 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 首次补丁只把加法提升为 `int64_t`、随后又窄化回 `int`，真实 `make check` 将其打红；replan 保留宽整数并直接序列化，最终 applied tree 只改 `include/tmfmt.hpp`，`make check` 实际编译运行且覆盖目标路径，普通年份和 `INT_MAX→2147485547` 均通过。verified 与 typed project-runner proof 一致。 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | eval/results/trace_query_wakeup_causal_runnable-20260803-050542 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 539s | 42 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=16,inv=1/0,fin_reject=40,unavail=0,prune=0 | fail-system | Trace 查询和系统补齐已给出 `worker-200→app-100`、8.300ms pre-wakeup dependency、10.000ms sleep 与 0.020ms post-wakeup runnable；模型前上下文也正确区分两类根因并禁止把候选升级为锁持有/唤醒后抢占。失败来自 typed authority 自相矛盾：账户窗 `1.000000..1.010020` 以 F-2 ±1ms 规则接入 `1.000000..1.010000` 投影，但关系编译又用 anchor 的 10.000ms 拒绝账户自身闭合的 10.020ms；探索已接受的 authority 在成文期消失，handoff 仍要求复制，validator 必拒，20 轮/40 次 reject 后降级为空答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
