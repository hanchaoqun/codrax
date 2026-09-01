# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T13:20:13Z
- sweep_start_ts: 20260901-062012
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-062013 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 精确 10ms 窗、唤醒链、链上排序、实际占时/规则可消双账户、邻近背景隔离、业务线索、帧因果边界、Trace 因果投影与确定性补采均在；默认 v2 `.root-causes.json` 也由模型本轮提交的 5 个有效 candidate selection 正常生成，证明 optional selection 成立时默认侧车可达。但模型 lead 写成“确定性优化工作与目标存在链上因果关系”，后文 typed runtime_work_relation 又明确该语义工作到目标的因果未证，形成同页因果口径冲突；按红线不扫描/改写正文，记模型服从性残差。B1544/B1545 的异常 carrier 恢复本轮未命中，B1543 显式 flag 仍由结构测试闭环。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-062013 | answer_regex,answer_contains | none | 703s | 50 | read=42,repo_map=3,list=0,trace=0,source_lens=0 | midloop=20,inv=7/1,fin_reject=0,unavail=1,prune=6 | pass | B1547 生产闭环：首稿 sequence 只含 Analyze→Explore→Extract→Finalize 三条相邻 precedence，零成文拒绝、零 relation patch、零 Analyzer→Finalizer 首尾直连，也没有 Orchestrator/Dispatcher/BusContext/MutableState 孤立节点。四阶段输入/输出/状态载体表可读，阶段定义和调度锚比 r1011 精确。末尾自动清单额外披露 3 个条件前置阶段及“证据稍弱”提醒略显冗余，但没有改坏用户要求的主四阶段图或表。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
