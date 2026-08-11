# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T16:22:57Z
- sweep_start_ts: 20260811-092255
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260811-092257 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 43 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass-with-caveat | 显式 114.940ms 窗、5 次 typed trace_query、自动补齐与 Trace 因果投影均在；CookieMonsterCl 23.994ms、NetworkService 19.041ms、ThreadPoolForeg D/IO、目标 running 算力缺口和链上 VerifyClass 均保留，邻近/背景未进入链上榜。模型最后把 `lower_priority_dependency_only` 扩写为“被低优先级线程阻塞”，超过 `measured_dependency_scheduler_supply_before_downstream_wakeup` 的授权；确定性语义业务线索主要由系统事实投影保留，模型摘要未主动消费。记 B544 软口径/上下文精度债，不扫描终稿硬改。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-092257 | answer_regex,answer_contains,mermaid_edge_count | none | 464s | 39 | read=18,repo_map=4,list=0,trace=0,source_lens=1 | midloop=13,inv=6/0,fin_reject=4,unavail=0,prune=0 | fail | B541b operation metadata 已让 Current-Source Authority 报六席全部 incident，但同一 Finalizer prompt 的 Diagram Contract 仍发布 BusContext/Mutable 两条 uncovered boundary recipe，合同自冲突；最终图丢失两载体的数据流，只保留 BusContext 孤点。Explorer 还因 `firstFinalizeDraft = strings.TrimSpace(out.FinalAnswer)` 被要求把完整 RHS object 改为 parser 截短的 `strings.TrimSpace`，暴露采集/完成端点口径不同源。464s 活跃链路始终等待模型并最终交付，未发生四分钟降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
