# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T10:46:25Z
- sweep_start_ts: 20260820-034624
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-034625 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 225s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、threadpool→network→cookie→app 已证链、11.000ms iowait 主因、三项 1.000ms 调度供给、实际占时/现规则可消双轴、Trace 因果投影与系统补齐均完整；邻近与 IO 综合评分只作支撑/背景。活跃流未因 4ms、4m、首字节、stall 或累计年龄降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-034625 | answer_regex,answer_contains,mermaid_edge_count | none | 306s | 41 | read=6,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=6/0,fin_reject=4,unavail=0,prune=0 | fail | B1230 生产正向：最终 argument-flow 已把 o.busCtx 映射到 BusContext，不再错绑 Mutable；Mutable 保留可见未证边界。整体仍不合格：图只剩阶段 precedence 与一个局部参数流，未回答请求的完整数据流；正文把少数 Mutable getter/复用判断泛化为各阶段共享读写，并混淆证据已证关系与概括性职责。四次成文拒绝还说明端点、partial scope、boundary、可见 participant 的 repair delta 未一次闭合。系统没有代写边、节点、标签或结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
