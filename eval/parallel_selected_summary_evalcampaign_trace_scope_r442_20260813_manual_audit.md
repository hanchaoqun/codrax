# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T16:42:48Z
- sweep_start_ts: 20260813-094247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-094248 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 134s | 33 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B729/B730 production-positive：完整 11 段/36.757ms 与独立 blocked_reason 12 条/Σ39.157ms 均发布，结构化 projection count=0；没有假跨工件关系块。模型仍把 kernel callsite 扩写为“等待 DMA fence 信号”，但系统 typed 边界明确资源对象/holder 未证；作为连续软教学服从性观察项，不以答案关键词门或系统改写处理。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-094248 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 完整因果投影仍在：目标 running 原始74.915ms/供给折算可消65.912ms 双轴、D-state 36.757ms、链上调度/优先级候选、业务 span 与邻近 support-only 分层均保留；无假跨工件块、无成文重试、无4ms年龄降级。模型部分将方向枚举写成内部英文 token，属既有低优先展示债，不影响事实与结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
