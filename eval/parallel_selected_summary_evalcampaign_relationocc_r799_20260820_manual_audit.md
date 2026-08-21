# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T05:55:41Z
- sweep_start_ts: 20260820-225539
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-225541 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 241s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 明确使用用户窗 2.000..2.020s；8 次 typed trace_query 覆盖状态、唤醒、阻塞、资源和根因视图；最终保留完整 Trace 因果投影、threadpool-400→network-300→cookie-200→app-100 链、11ms IO 席与三个独立 1ms 调度候选，并把邻近/背景隔离。模型正文却把 typed `pre_wakeup_dependency` 候选写成“阻塞了后续三个线程/最终导致”，超出了同轮 handoff 明示的 `work_completion_dependency_authority=not_provided` 与 `direct_blocking_authority=not_provided`；随后 caveat 又否认该直接传导，形成内部矛盾。系统投影本身保持候选边界，未改写模型结论；记为模型遵循波动观察项，不以原文扫描硬门。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-225541 | answer_regex,answer_contains | none | 387s | 39 | read=13,repo_map=5,list=0,trace=0,source_lens=3 | midloop=9,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | 最终有合法 sequenceDiagram、完整四阶段顺序和逐阶段表，阶段转移使用业务化描述；但初稿多画一条无 typed call authority 的 Orchestrator→StageAnalyze 后被精确删除。第 1 次 patch 同时携带正确 failure_ref 和与其一致的旧式 block/match/body_occurrence，工具因字段互斥再次拒绝，第 2 次才成功；这是可泛化的修补协议心智/容错 gap。最终图中的 Orchestrator 变成孤立参与者，表内状态载体也没有把 Mutable/AnswerDocumentV2 的实际持久化通道讲清，答案可用但不算完全。B1277 的重复同端点 occurrence 修复本轮未被生产形触发，仍只由回归测试证明。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
