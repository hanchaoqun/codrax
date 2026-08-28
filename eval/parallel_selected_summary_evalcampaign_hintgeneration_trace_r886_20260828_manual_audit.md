# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T13:11:26Z
- sweep_start_ts: 20260828-061124
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-061126 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 260s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=3,inv=3/1,fin_reject=3,unavail=0,prune=0 | partial | 显式主窗、四跳链、链上 11.000ms IO 第一席、三个独立 1.000ms 优先级候选、实际占时/规则可消双账户、业务下钻、背景隔离、自动补采与完整 Trace 因果投影均保留。模型把 14ms 睡眠跨度误写成 threadpool 自身等待，又把 17−11=6ms 推测为处理/跨核传递开销，typed 证据未授权，按模型波动留观。另确认结构 GAP：只补 summary 的 trace_causal_claim_caliber 时只能整块替换，先丢正文/主席属性，再经历新增第二 summary 与基数修补，共 3 次无新增事实拒绝。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-061126 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 374s | 38 | read=20,repo_map=4,list=0,trace=0,source_lens=0 | midloop=10,inv=9/0,fin_reject=1,unavail=0,prune=0 | pass | 四阶段职责、analyze→explore→extract→finalize 主链和合法 Mermaid 均完整；唯一关系拒绝后，模型一次用 live refs 删除 4 条无证 BusContext→stage 边，并添加 BusContext→BuildAgentContext、Extractor→BuildAgentContext 两条 typed argument-flow。最终还保留 BusContext→Mutable，未再出现 r885 的大小写重复 Mutable 或孤立 hasReuse。B1378 的 cap-v1 键在 Trace patch 车道有生产冒烟，但 read 本轮没有同车道换代，故核心生产正证仍待自然触发。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
