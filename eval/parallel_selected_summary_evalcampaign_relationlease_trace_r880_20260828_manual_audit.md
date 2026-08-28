# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T10:16:22Z
- sweep_start_ts: 20260828-031620
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-031622 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 137s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面通过：显式 2.000000..2.020000s 窗、四跳唤醒链、11.000ms 链上 iowait 第一席、三项独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、背景隔离、自动补采和完整 Trace 因果投影均在；活动流未按固定耗时降级。r879 的重复 IO 指数本轮未复现。模型仍把调用点名称扩写为文件系统缓存页机理，并提出缓存未命中/预取/空间不足等未证分支；正文同时披露对象和后端未知，保持 B1269/B1271 软教学观察，不增加答案原文硬门或系统改写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-031622 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 502s | 46 | read=20,repo_map=4,list=0,trace=0,source_lens=0 | midloop=18,inv=8/0,fin_reject=7,unavail=0,prune=0 | partial | B1370 生产转正：whole-block 误用后专用 `answer_doc.patch_relation_repair_scope` 携当前代 failure refs 出现，后续没有 unknown/stale ref，最终正常成文且 Mermaid 可解析。仍有 7 reject：component-split 的 typed join tuple `bus.Mutable -> AgentContext.Mutable` 已在旧可见边 `BusContext -> AgentContext` 上锚定，participant addition producer 因 canonical tuple 已存在而过滤 allowed addition，却未发布把该既有边映射到请求参与者节点的 replace capability。提示继续要求 join，模型只能先添加重复 tuple、再删除其中一条，额外消耗四轮。最终图与职责说明可用，但含 `codraxNode1["o.busCtx"]` 等内部技术节点，关系表达仍偏实现细节。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
