# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T13:50:57Z
- sweep_start_ts: 20260828-065055
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-065057 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四跳唤醒链、11.000ms 链上 IO 第一根因、三个独立 1.000ms 优先级候选、真实占时/规则可消双账户、业务下钻、邻近/背景隔离、自动补采与完整 Trace 因果投影均保留。模型把等待调用点进一步断言成缓存未命中/后端 IO 根源，并把重叠的各线程状态跨度描述为可相加且“基本吻合 20ms”，typed 证据不支持，按模型推理波动留观，不新增 prose 硬门。B1379 的生产 schema 与工具调用已生效，但模型同轮既用 block_field_edits_v1 设 surface_role，又在同块完整 replace 中显式携带同值和 facet_ids，被同目标冲突拒绝；accepted draft 仍可渲染。确认 B1380：只吸收 replacement 明确携带的同字段同值冗余，缺省/异值继续冲突。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260828-065057 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 526s | 37 | read=21,repo_map=3,list=0,trace=0,source_lens=0 | midloop=19,inv=8/0,fin_reject=7,unavail=0,prune=0 | partial | 四阶段责任、Analyzer→Explorer→Extractor→Finalizer 主链与可解析 Mermaid 正确；BusContext/Mutable 以无箭头分组和 typed unproven boundary 诚实披露。7 轮修补中先出现同一 failure_ref 的重复 remove/replace、跨代旧 ref 重放及 participant 清理级联；后续 BusContext→Extractor 用 o.busCtx→BuildAgentContext 锚定到错误业务节点，被关系门正确拒绝并删除。runner 唯一失败为 mermaid_incident_nodes=4<6；当前 oracle 无条件要求 6 个关系参与节点，与 typed “请求关系未证、保留无箭头边界”合同冲突，可能反向鼓励虚构边。确认 B1381：应以结构化 requested-participant coverage（已证 incident edge 或 typed unproven no-arrow boundary）验收，而非任意 incident node 总数；本批只立案，不为过 case 放宽关系证据门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
