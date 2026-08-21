# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T21:52:59Z
- sweep_start_ts: 20260821-145257
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-145259 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 284s | 41 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s；10 次 pid/thread/window 过滤查询覆盖 5 个维度族；四节点唤醒链、逐跳 CPU、11.000ms 链上 IO 第一席与三个独立 1.000ms 调度/优先级候选完整；实际占时/现规则可消量双账户、邻近/背景隔离、自动补采与 Trace 因果投影均在；finalizer 零拒绝，无固定 4ms/4m 降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-145259 | answer_regex,answer_contains,mermaid_edge_count | none | 677s | 55 | read=20,repo_map=3,list=0,trace=0,source_lens=1 | midloop=20,inv=8/0,fin_reject=13,unavail=1,prune=0 | fail | B1309 生产正证：completion 明确导航并读取 internal/context/builder.go:47-71，最终引用 `Mutable: bus.Mutable`；read 从 r827 的 34 次降到 20 次，52 explorer iteration 降到约 32，未再遗漏真实值绑定。但第一稿把一条 initializer 证据扩成 Mutable→多个阶段，正确被拒；随后 live lease/整块替换/旧 ref/visible label/participant boundary 等合同造成 13 次 finalizer reject。最终图无重复边，阶段链与 BusContext→BuildAgentContext 成立，但 Mutable 只有无箭头包含和 unproven 边界，没有把已证局部 `bus.Mutable -> AgentContext.Mutable` 数据流表达出来，仍未完整满足显式图层关系要求。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
