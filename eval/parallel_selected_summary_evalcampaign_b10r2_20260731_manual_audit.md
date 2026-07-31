# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T19:32:01Z
- sweep_start_ts: 20260731-123200
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260731-123201 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 124s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Z2/Z4 生效：principal window 仅为 114.940ms；36 次 target wakeup 及 34+1+1 roster 正确，且未再把 sleep_exit 反写成“唤醒后立即睡眠”。Z3 仅部分生效：#4 供给席被保留为次级候选，但正文仍把 cross_row_additivity=forbidden 的 23.994ms 与 19.041ms 相加为 43.035ms；又把目标线程 running=23.4% 错写为 CPU 利用率不饱和。主方向正确，但存在实质语义越权。 |
| 2 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-123201 | trace_attachment,answer_regex | perf_triage+trace_query | 193s | 41 | read=3,repo_map=2,list=0,trace=4,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Y1/Z1 生效：heavy-compute 未升格根因；缺 sched 数据被诚实保留为 data gap；86.111ms > 用户给定 50ms 的标量关系正确，trace 行与源码行也分席。但 current-source 机制不可信：把 EventTraceMark 枚举常量写成 B/E 解析器，虚构 classifyFrameCategory，并把用户比较值 50ms 写成系统 jank 阈值。emit_evidence 还把 strings.Contains 调用行错误 grounded 为不存在 callee 的 call anchor，暴露通用结构化证据完整性 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
