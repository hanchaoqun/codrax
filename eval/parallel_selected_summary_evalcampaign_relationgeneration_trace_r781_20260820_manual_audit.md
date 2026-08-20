# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T17:20:10Z
- sweep_start_ts: 20260820-102008
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-102011 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 250s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 显式 2.000–2.020s 窗、Trace 因果投影与自动补采完整；链上首因仍为 threadpool-400 的 11ms IO 等待，三个 runnable 调度供给席各 1ms；实际占时/规则可消双轴、sleep 症状和背景 IO 隔离均在。250s 活跃流使用当前模型答案完成，无时间型降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-102011 | answer_regex,answer_contains | none | 945s | 60 | read=51,repo_map=7,list=0,trace=0,source_lens=0 | midloop=36,inv=15/0,fin_reject=12,unavail=0,prune=3 | partial | B1248 生产命中：第 9 次 patch 清空旧失败边后，租约被消费，下一代 precedence spine 最终通过，不再跨代互锁；最终只有一张合法图和一张表，无重复载体。仍有 B1249：两阶段“先删旧边、再加 typed candidate”让模型反复在同一轮加入系统已知的精确 precedence 候选，造成 12 次拒绝和 60% 上下文；应把 producer-owned candidate tuple 结构化进 lease，只允许列出的候选同轮加入，不解析 candidate 文本、不由系统选边。另有 P2 展示债：表头退化成“项目/列2…列6”，且 Explorer 行误述为调用 emit_answer_document；先记为展示/模型事实校准，不用正文关键词硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
