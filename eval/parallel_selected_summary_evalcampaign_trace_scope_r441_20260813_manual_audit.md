# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T16:12:47Z
- sweep_start_ts: 20260813-091246
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-091247 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 231s | 39 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B729 production-positive: uncapped typed block完整列出11段/36.757ms，独立 census 为12条/Σ39.157ms，且没有因果投影。Runner FAIL 是旧 `EXPECT_NOT_CONTAINS=Trace` 扫整段终端输出，误咬正常“trace 附件”字样。新确认 B730：模型对首个 query 的 payload JSON 再调用 trace_query 后，系统把 producer-owned payload 误识别为第二物理工件，注入无意义跨工件关系表。模型正文仍把 kernel callsite 扩写为 DMA fence/GPU 对象，虽随后承认 holder 未知，记软教学服从性，不以答案关键词硬改。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-091247 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 281s | 48 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B728/B729 均未破坏 causal 主车道：完整 Trace 因果投影、目标自身 running 74.915ms/规则折算缺口65.912ms、D-state 36.757ms、链上调度/优先级候选、业务 JIT span 与邻近 support-only 均保留。一次 finalizer reject 是模型把 add_blocks 发成畸形 JSON 字符串；下一轮成功，无空答案或4ms年龄降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
