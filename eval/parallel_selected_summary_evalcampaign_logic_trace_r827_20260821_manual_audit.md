# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T21:24:35Z
- sweep_start_ts: 20260821-142435
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-142435 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 213s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s；3 次目标过滤查询覆盖四个维度族；四节点唤醒链及逐跳 CPU 完整；11.000ms 链上 IO 排第一，三个 1.000ms 调度/优先级候选保持独立不可加；实际占时与现规则可消量分账；邻近/背景未进入主因；Trace 因果投影和自动补齐在，finalizer 零拒绝，无固定 4ms/4m 降级。模型对具体缓存对象和同步阻塞保持未证边界。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-142435 | answer_regex,answer_contains,mermaid_edge_count | none | 922s | 52 | read=34,repo_map=3,list=0,trace=0,source_lens=0 | midloop=21,inv=9/0,fin_reject=11,unavail=1,prune=1 | fail | B1311 生产正证：未再出现 participant carrier 与 node identity 互斥，最终文档可出厂且未降级。但显式要求的 BusContext/Mutable 数据流没有闭合，二者成为与四阶段图断开的孤岛；三条阶段边各重复两次。过程耗费 52 次探索迭代、34 次 read、11 次 finalizer reject，说明 participant 完成导航仍偏向局部操作/错误坐标，关系锚修补还会把已有可见边再次追加。散文责任说明可用，图层没有满足主要关系要求，不能按 runner 字面命中判通过。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
