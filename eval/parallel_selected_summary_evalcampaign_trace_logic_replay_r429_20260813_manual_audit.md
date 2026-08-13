# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T09:38:59Z
- sweep_start_ts: 20260813-023858
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-023859 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 259s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 显式 233.190ms 用户窗、Trace 因果投影、自动补齐、链上根因、邻近/背景分层，以及实际占用/业务线索和现规则可消除量双轴均保留；四次 trace_query 也都沿用同一用户窗，没有复现 r428 的嵌套窗漂移。但模型仍把不同席位的 65.912ms 与 36.757ms 相加成 102.669ms，又把多个优先级候选跨席相加成约 7.67ms，违反同一输入板已经发布的 `cross_seat_aggregation_authority=forbidden`。系统投影随后仍按席正确显示，未改写模型结论。该轮证明 B716 的多窗上下文根修仍需施工，但跨席求和是独立 B717 观察，不按答案关键词加硬门；先审计 typed 决策上下文是否存在相反加总教学。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-023859 | answer_regex,answer_contains,mermaid_edge_count | none | 298s | 37 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | B715 获生产正证：route/Analyzer 均保持 `diagram_required=true`，四轮关系修补后必需 Mermaid 仍在，系统没有再教模型删图。人工答案仍不合格：图只含四阶段 precedence 与 `Orchestrator.Run→BusContext`、`BuildInitialInstruction→Mutable` 两条局部映射，未画出 analyzer/explorer/extractor/finalizer 与 BusContext/Mutable 的请求数据流；正文却宣称完整共享状态传递。现有 per-participant incident 门允许多个互不相连子图共同签绿，确认 B718 需要按 typed flow scope 审计关系拓扑完备性；不能让系统补边或按节点名/答案原文硬化。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
