# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T07:31:38Z
- sweep_start_ts: 20260804-003137
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | FAIL | eval/results/read_combo_command_current_source_explanation-20260804-003138 | answer_regex | none | 29s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | classifier 这次发出 raw_route=operation、operation=computer_operation、target_surface=desktop，同时 needs_repo=true、current_source=required、source=mixed。旧 guard 按 concrete operation 保留权限，planner 反而要求用户补“命令或目标”，pipeline 仍为 0。证明 required current-source obligation 必须在 route 主轴也漂移时 fail-safe 到只读 pipeline；已由 B67 route-axis 小修覆盖。 |
| 1 | trace_query_wakeup_causal_io_chain | ABORTED | eval/results/trace_query_wakeup_causal_io_chain-20260804-003138 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 572s | 51 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=20/20,fin_reject=0,unavail=0,prune=0 | fail | 人工在第 21 轮后 fail-fast 中止。模型已得到 exact 2.000..2.020 / 20.000ms、完整唤醒链与 11ms IO 主因，但 20 次 completion 都被同一 missing relation_claim 拒绝。模型实际把正确 claim 放进 string-encoded aggregate_facts 的 misplaced sibling tail；兼容解码器恢复 reason/confidence/result_kind 却忽略 relation_claims。与此同时 Explorer 把 format-only carrier 设为必抄，造成 103k context 重试风暴；没有进入 finalizer，不能审计 B67 TRACEVALUE1 的最终摘要效果。 |
