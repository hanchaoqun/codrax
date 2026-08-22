# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T23:43:04Z
- sweep_start_ts: 20260821-164302
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-164304 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 277s | 40 | read=0,repo_map=0,list=0,trace=14,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、4 线程/3 边唤醒链、threadpool-400 的 11.000ms 链上 IO 第一席、3×1.000ms runnable/优先级候选、实际占时/规则可消双轴和完整 Trace 因果投影均在；邻近/背景未升主因，277s 活跃流未按固定阈值降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-164304 | answer_regex,answer_contains,mermaid_edge_count | none | 626s | 53 | read=21,repo_map=4,list=0,trace=0,source_lens=0 | midloop=22,inv=8/0,fin_reject=13,unavail=1,prune=0 | partial | 最终正文和 Mermaid 可读，Analyzer→Explorer→Extractor→Finalizer 主链以及 BusContext/Mutable 局部数据关系可见，Mutable 的完整请求关系仍诚实标未证明；但经历 13 次拒绝。B1314 的 label-pair ref 与 B1313 的 stale-boundary ref 均获得生产正证。拒绝链暴露 B1316：显式 diagram payload 缺外层 kind 仍耗两轮、evidence-negative failure 暴露不可执行 replace、已存在阶段关系因身份大小写差异又作为 allowed additions 发布造成重复边、retain_as_context 的模型多行标签被 schema 接受却被执行器拒绝。最终没有恢复旧稿或系统代写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
