# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T23:19:45Z
- sweep_start_ts: 20260821-161944
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-161945 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 236s | 40 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程三跳唤醒链、threadpool-400 的 11.000ms 链上 iowait 第一席、三个各 1.000ms runnable/优先级反转候选、实际占时/规则可消双账户、业务下钻方向和完整 Trace 因果投影均保留；邻近/背景未升为根因，0 次成文拒绝。模型明确等待点不证明具体文件/资源/持有者，未把时序邻近扩写成直接阻塞；活动流 236s 正常完成，无固定 4ms/4m 降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-161945 | answer_regex,answer_contains,mermaid_edge_count | none | 447s | 50 | read=29,repo_map=5,list=0,trace=0,source_lens=0 | midloop=23,inv=10/0,fin_reject=4,unavail=0,prune=1 | partial | 最终没有回退旧草稿，A→E→X→F 与 BusContext→BuildAgentContext 四条 typed 关系可见，Mutable 诚实保留 unproven 边界；但该轮首个 participant defect 是“有 typed incident edge 未画出”，按权限必须由模型先自写可见关系，因此没有形成 B1313 的纯边界 ref 生产 witness。4 次拒绝中前两次是未修 sibling 引用错误，后两次是 typed candidate 的 visible_label 缺失后，schema 未发布 label-pair failure_ref，模型使用 identity 不完整的 legacy relabel 必然找不到 exact anchor，只能再次整块重写。最终 `SetResult` 项仍显示 `orchestrator.go:4602`，而同轮精确 advisory 已给 candidate `:4532`，说明引用同主体一致性也未闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
