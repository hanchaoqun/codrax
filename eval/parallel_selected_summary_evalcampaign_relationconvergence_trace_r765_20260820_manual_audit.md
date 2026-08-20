# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T10:32:56Z
- sweep_start_ts: 20260820-033255
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-033256 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、threadpool-400→network-300→cookie-200→app-100 已证链、11.000ms iowait 主因、三项 1.000ms 调度项、实际占时/现规则可消双轴、Trace 因果投影和系统补齐均保留；背景综合评分未参与时长排序，活跃任务未因 4ms/时长降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-033256 | answer_regex,answer_contains,mermaid_edge_count | none | 307s | 38 | read=10,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B1229 生产生效：只运行一次 Explorer，第二个稳定 Mutable blocker 在第 20 轮携带未证 caveat 闭环，未再重启探索；较 r764 少 142s、read 12→10、repo_map 5→4、midloop 12→8。最终图仍有精确语义错误：edge anchor 的 from_identity=o.busCtx 被画在 Mutable 节点上，读者看到“Mutable 作为参数传递”，实际传入 BuildAgentContext 的是 BusContext；正文还把 Emitted* getter 说成 extractor 写入，并把 finalizePreviewHook 的 SummaryExtractor 字段误当成 pipeline extractor 输入。局部覆盖披露诚实，但答案关系与职责仍不准确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
