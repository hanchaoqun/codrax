# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T22:51:31Z
- sweep_start_ts: 20260811-155129
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-155131 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 138s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B578 正证：树中同一 app sleep 物理段只剩一条 `自身·sleep 5.000ms [E1+E2]`，没有重复计量。模型也诚实披露无 frame evidence、无直接阻塞权限、无经典优先级反转，并保留链上 VerifyClass 4.600ms 与目标 runnable 0.800ms；但连续第二轮仍把 5.000ms 唤醒前 sleep 称为“唤醒延迟”，属于确定性区间语义错误。日志显示上游 perf_triage 先把 switch-in 当成实际唤醒并给出 5.8ms“阻塞”等污染性 advisory，后续 typed finalizer 虽准确仍未完全纠偏，故不是单次模型波动。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-155131 | answer_regex,answer_contains | none | 360s | 40 | read=20,repo_map=3,list=0,trace=0,source_lens=1 | midloop=7,inv=2/0,fin_reject=3,unavail=0,prune=7 | pass | 正文、阶段表与最终四阶段 precedence 图准确且有引用；360s 活跃任务由原模型正常完成，没有累计四分钟降级。三次成文拒绝中一次是模型 patch 缺顶层 `kind`，另外两次是系统把同一 `orch` actor 上两条不同 typed operation self-message 聚成冲突 pair，迫使模型删掉 `runAnalyzePhase→dispatchStage` 与 `runTaskGraph→runReadSchedulerLoop`。最终答案仍满足题面，但图层丢失有用且已证的内部关系，立 B579 泛化修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
