# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T10:46:52Z
- sweep_start_ts: 20260828-034650
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-034652 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 191s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 显式 2.000000..2.020000s 窗、5 次 typed trace_query、threadpool-400→network-300→cookie-200→app-100 四跳链、11.000ms 链上 IO 第一席、三个 1.000ms runnable/优先级候选、实际占时/规则可消双账户、背景隔离和完整「Trace 因果投影」均在；无固定 4ms/4m 降级或系统代写。模型仍把 fscache_page_wait_on_page_bit 扩写为具体缓存页/文件系统 IO 同步等待，虽保留 owner/backend 未知限定，继续归入 B1269/B1271 软教学观察，不据答案文本加硬门。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-034652 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 245s | 32 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=5/0,fin_reject=1,unavail=2,prune=0 | partial | B1373 未自然触发；finalizer 仅 1 次关系拒绝，模型正确删除两条无锚直连，Mermaid 语法合法。但最终四阶段链与 BusContext→Mutable 形成两个可见孤岛，缺少用户要求的阶段/载体数据流。typed evidence 已证明可经 types.AgentExtractor/o.busCtx→BuildAgentContext 两条边连接，旧 component join 只识别单条跨现有组件边，因共享技术节点尚未可见而漏报；runner 的边数/incident-node 下界也误签 PASS。确认 B1374 多边 typed join frontier + 证据断连范围披露一致性 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
