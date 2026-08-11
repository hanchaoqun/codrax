# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T19:29:13Z
- sweep_start_ts: 20260811-122911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-122913 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 5.000000..5.007000 窗、自动补采、Trace 因果投影、链上 class-verification/runnable 双席、实际占用/规则可消双轴及背景隔离均保留；但 typed context 明示 direct blocking 未建立、span 在 wake 后仍延续 0.400ms，模型仍称 app-100“等待 worker-200 完成 VerifyClass”并称其“阻塞唤醒”，再次复现 B544 的机理越权。未用答案原文硬门修补。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-122913 | answer_regex,answer_contains,mermaid_edge_count | none | 1132s | 61 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=5/1,fin_reject=14,unavail=0,prune=1 | fail | B562 的逐席 repair_action 与 canonical anchor fields 已进入生产反馈，但合法的 member-qualified endpoint 被 Mermaid 显示换行截成 owner，造成同一 typed call edge 反复误拒；14 次拒绝后虽发出有效文档，图仍是 stage 主链、两段 Mutable 方法调用和 BusContext stage 写入的多个断片，未形成用户要求的共享状态数据流。1132s 活跃流未因四分钟累计时长降级，B560 正证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
