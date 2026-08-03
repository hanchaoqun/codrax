# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T12:26:14Z
- sweep_start_ts: 20260803-052613
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-052614 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail-system (core answer restored) | RELWIN1 已覆盖：模型第一次漏 final supplement 新增的 closure claim，收到精确 ID 后第二次成文成功；完整模型答案和系统 Trace 因果投影均在，未发生系统替答。主结论正确区分 10.000ms sleep、8.300ms pre-wakeup 可消除候选并披露无锁 holder/waiter 证明。但摘要声称“无 CPU 压力证据”，随后 typed 系统投影发布 3.500ms 跨线程调度压力；该 background row 未进入模型前 Trace Decision Inputs，属于上下文缺口，不以 PASS 收账。 |
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260803-052614 | write_apply,answer_regex | none | 203s | 20 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单批修改 `include/tmfmt.hpp`，把 year_offset、render 参数与中间值统一提升为 `long long`；比只扩表达式的最小修略宽，但仍为同一内部值通道且无测试改动。真实 `make check` 编译执行并覆盖目标路径，final verified 与 typed proof 一致；记模型实现选择波动，不新增 C++ 特判。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
