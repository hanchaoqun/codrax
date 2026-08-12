# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T02:02:06Z
- sweep_start_ts: 20260811-190204
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-190206 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 129s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 7ms 窗、3 次 trace_query、typed 唤醒链、实际占用/规则可消双轴、因果投影、自动补采、frame_causality=unproven 与非链背景边界均保留；但终稿把 5.000400..5.005400 的 5ms span 写成 4ms，又把 wake=5.005000 叙述成 span_end=5.005400 后才唤醒，并进一步断言 app 等待 worker 工作完成。属于 B544 的 typed 时间/机理消费复发，不是 runner PASS 能覆盖的正确性。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-190206 | answer_regex,answer_contains | none | 351s | 42 | read=19,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=3,unavail=0,prune=7 | fail | B592 生产正臂生效：context.go 被调查，BusContext/MutableState/AnalysisIR/EvidenceItems/AnswerDocument 均进入答案，同名 dataflow 子系统未再污染终稿。B560 生产正臂也成立：模型持续活跃 351s，未因超过 4 分钟降级或被系统答复替代。但 analyzer 漏发 requested_answer_dimensions 且参与者清空，导致 canonical stage precedence authority 未激活；三次关系校验后模型删除全部阶段关系，只留下两条彼此孤立的调用边。正文另有 emit_analysis 归属 Explorer、Finalizer 唯一写 MutableState 等无充分证据断言。runner 只验 token/图存在，误签 PASS。确认 B593。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
