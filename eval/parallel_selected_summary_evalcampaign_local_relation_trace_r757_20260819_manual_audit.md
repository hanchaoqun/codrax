# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T05:02:23Z
- sweep_start_ts: 20260819-220220
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260819-220223 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 210s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗 2.000000..2.020000、threadpool-400 → network-300 → cookie-200 → app-100 已证唤醒链、11.000ms 链上 IO 首席、三个 1.000ms runnable 调度席、目标 20.000ms S 态症状及实际占用/规则计价双轴均保留，Trace 因果投影和自动补齐完整。模型仍把四个节点误称“四跳”，并把“高层同步阻塞机理未证”写成“直接唤醒者缺结构化依赖”；上下文已逐字说明 wakeup edge 已证而同步阻塞/持锁机理未证，故按模型波动留档，不加 prose 硬门。系统投影图例仍过长且含 typed/口径/席位等内部词，B1217 保持 P2。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-220223 | answer_regex,answer_contains,mermaid_edge_count | none | 387s | 40 | read=7,repo_map=4,list=0,trace=0,source_lens=1 | midloop=7,inv=6/0,fin_reject=2,unavail=0,prune=1 | partial | 最终图保留三条已证阶段顺序和 BusContext → BuildAgentContext 参数流，Mutable 只作 BusContext 内无箭头包含并保留未证关系边界；无伪桥、业务标签合法。该轮 Explorer 没有发出可引用的 Mutable 局部调用/读写操作，故 B1208 的 local-only 候选生产臂未触发，不能把“未触发”虚报成失败或正证；单元/全内部 pin 已覆盖。正文仍泛称 Explorer 写 Mutable、Extractor/Finalizer 读写 Mutable，超出本轮关系证据，按模型服从残余记 partial，系统未代写或修正文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
