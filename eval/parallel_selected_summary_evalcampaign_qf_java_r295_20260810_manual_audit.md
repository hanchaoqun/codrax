# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T05:30:05Z
- sweep_start_ts: 20260810-223004
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260810-223005 | answer_regex,answer_contains | none | 417s | 40 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=2,unavail=0,prune=5 | pass | B509 生产正证：Analyzer 首次把请求未点名的 Orchestrator/Explorer Agent 等作为 participant 时，逐席 current-request source_quote 校验精确拒绝；修正后不再产生这些硬义务。最终答案完整保留 analyze→explore→extract→finalize、各 stage 输入/输出/状态载体及一条独立已证 runAnalyzePhase→dispatchStage 调用。仍有 2 次 finalizer 拒绝：模型曾把独立实现调用与 stage skeleton 跨层连接，修复后才拆成两个 typed component；记 B510 soft-context salience，不放宽关系门。 |
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-223005 | primary_answer | none | 561s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 五条 call edge、容量 guard 与精确 citation 均在；摘要也正确写 `AuditLog.record` 输出到标准输出。但第 5 跳同一句又称“实现审计落库”，与自身及实际 `System.out.println` 实现矛盾。Explorer 已读 AuditLog.java:6，却没有发出该函数体调用的 typed EvidenceItem；Finalizer 只收到 incoming callsite，`definition_status=unproven`，故 B508 为确定性 terminal-body evidence gap。Analyzer 第一轮超长无工具输出被正常重试，单独按 provider/model 波动记录。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- `B509-PARTPROV1`：生产关闭。精确 current-request provenance 阻止了 Analyzer 猜测 participant 升级成最终硬义务；未发现 request 关键词分类、模型/答案原文扫描或系统补边。
- `B508-TERMINALBEHAVIOR1`：确认且进入下一开发批。问题不是模型单次措辞波动，而是已读终点函数体没有精确 typed 事实载体，Finalizer 无法区分“调用名看似落库”与“实现仅 stdout”。
- `B510-STAGESKELETON-SALIENCE1/P1-watch`：stage skeleton 与独立 implementation relation 均有证据，但上下文没有足够醒目地声明二者属于不同图组件，模型先跨层连接再修复。关系 validator 行为正确；先观察复放，不以单案新增硬门。
- 本批未触及 Trace 显式时间窗、自动补齐、因果投影、链上根因及业务线索通道。
