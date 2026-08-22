# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T16:13:06Z
- sweep_start_ts: 20260822-091305
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-091306 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 38 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、app-100 四态闭合、threadpool-400→network-300→cookie-200→app-100 四节点唤醒链、11.000ms 链上 IO 第一席及三个独立 1.000ms 候选完整；正文和确定性投影均分开实际占时与规则可消量，邻近/背景明确不进根因排序，补采 root_cause_rank/critical_blocking_calls 生效，Trace 因果投影未因耗时阈值降级。模型明确限定优先级候选不证明锁持有者/同步阻塞，未把 IO 活动指数当墙钟或主因。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260822-091306 | answer_regex,answer_contains | none | 179s | 28 | read=7,repo_map=1,list=1,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 第一轮原生 blocks 成文即通过，零 reject、零 JSON 字符串化恢复。requested outputs 仅 summary/trace；图为 typed 可选 DiagramPlan 下的模型自主选择，不是系统硬要求。答案准确区分日志写入与 sink 工厂选择两段，终点是 fputs(stderr)，未再虚构 flush；可选图只画显式已证边，Sink::write→ConsoleSink::write 的 C++ 虚分发边未伪造成直接调用，并在 caveat 中说明证据边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
