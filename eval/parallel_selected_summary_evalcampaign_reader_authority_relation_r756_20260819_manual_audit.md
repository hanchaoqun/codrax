# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T04:29:56Z
- sweep_start_ts: 20260819-212954
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260819-212956 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 209s | 40 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 完整保留 threadpool-400 → network-300 → cookie-200 → app-100 已证链；链上 D/IO 11.000ms 为首席，三个 runnable 各 1.000ms 另车道分账，app-100 自身 20.000ms S 态仅作症状。B1215 生效：模型正文不再复制 status=complete/state_partition_coverage/io_wait_caliber，专用目标状态清单只发布一次且使用读者语言；显式窗因果投影和自动补齐均在。系统附加 Trace 图例仍有 typed/口径等偏内部词，记 P2 展示残余，不影响本轮模型结论。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-212956 | answer_regex,answer_contains,mermaid_edge_count | none | 345s | 39 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | 四阶段顺序与职责正确，但 Mutable 最终仅作为 BusContext 内的孤立分组成员；图只保留三条 stage precedence 和 BusContext → BuildAgentContext。第一稿曾画 Orchestrator → Mutable/各阶段，关系 gate 正确拒绝无 anchor 边；修补提示只给 BusContext request-scoped candidate，未给已经存在的 Mutable local-operation candidate，模型遂删除真实 SetResult/EmittedAnswerSymbols 局部边。B1208 获生产复现：局部关系可见性与未证请求关系被错误二选一，需发布“local candidate + retain boundary”，不能放松 gate 或由系统代画。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
