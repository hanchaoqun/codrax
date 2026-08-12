# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T02:22:53Z
- sweep_start_ts: 20260811-192251
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-192253 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 30 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | r351 的时间算术/先后矛盾已消失：终稿正确保留 5ms span、wake 早于 span end、frame_causality=unproven；显式窗、typed 链、实际占用/规则可消双轴、确定性语义优化、调度供给、因果投影、自动补采及非链背景边界均完整。但同一终稿明细写 worker-200 在 CPU2、app-100 在 CPU1，摘要却说 worker 占 CPU1 且二者“同一 CPU 核心重叠”。payload 已有 waker_cpu=2/wakee_target_cpu=1/cpu_relation=cross_cpu；这是模型消费自冲突，runner 未覆盖。确认 B594：把精确跨核拓扑发布到中性 typed 对照面，不扫描/改写模型正文。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-192253 | answer_regex,answer_contains | none | 229s | 32 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | B593 生产正臂成立：首个有效 analysis 显式发出 stage/input/output/state-carrier 四维，Current Run Stage-Lane Authority 发布 3 条 checkout-verified precedence；最终 Mermaid 连续保留 analyze→explore→extract→finalize，表格包含四阶段输入/输出/载体，未再靠删关系过关。两次 reject 分别清理首稿未证 call/data-flow 与已连参与者的 stale boundary，最终结构合法。软观察：箭头消息仍用内部“dispatch StageX”而不是业务流转词，且保留一条“部分边类型支持不完整”的过时谨慎 caveat；核心关系/答案正确，暂不因模型措辞波动增设终稿关键词硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
