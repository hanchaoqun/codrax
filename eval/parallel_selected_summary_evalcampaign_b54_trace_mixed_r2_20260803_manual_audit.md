# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T14:38:08Z
- sweep_start_ts: 20260803-073806
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-073808 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 195s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | `RELSTALE1` 生产复验通过：final handoff 只携带当前 10ms target-state authority，旧 15ms claim 不再形成互斥合同；唯一成文拒绝是首稿漏带当前 block-level relation_claims，精确补齐后即过。显式窗、3 次 trace_query、唤醒链、根因榜、8.300ms 可消除候选和 Trace 因果投影均保留。正文仍把 typed wakeup 时序过度解释为“等待 I/O 完成/直接根因”，但 handoff 已明确禁止该升级，暂记模型波动观察项，不加题面/答案关键词硬门。确认系统 GAP：尾部算术附注把 worker-200 的 runnable 8.300ms 和上下文 10.000ms 错绑给 app-100，发布了 28.300ms 假矛盾；正确 typed 状态行紧邻其后又显示 app-100 running/runnable=0。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260803-073808 | trace_attachment,answer_regex | perf_triage+trace_query | 246s | 38 | read=4,repo_map=3,list=0,trace=2,source_lens=3 | midloop=4,inv=2/1,fin_reject=0,unavail=1,prune=0 | fail-mechanism | `DIAGCALL1` 生产表现符合预期：final 明示 grounded_callsite=0/ordered_path_authority=unproven，模型没有再提交图或伪 call anchor，零成文拒绝；跨 family hard 拒绝路径另由 wiring/pin 覆盖。新 GAP：Explorer 把只说明“MarkerPID 是 marker payload 内 PID”的字段声明摘要成“B/E 配对键”，closure/handoff/final 又将该模型摘要当源码机制权威。实际同步栈键是 artifact source + ftrace header TID；MarkerPID 是 payload owner/namespace process identity。说明当前 EvidenceItem 的位置真实并不保证 Summary 的运行时角色语义真实。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
