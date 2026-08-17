# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T11:17:37Z
- sweep_start_ts: 20260817-041736
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-041738 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗口内完成 3 次 typed 查询，因果投影、自动补齐、链上/背景边界均保留，零成文拒绝；`bounded_window_candidate` 只留在 JSON 控制字段，用户可见正文已改为自然中文，B967 获生产正证。正文仍把无 holder/waiter 证明的依赖供给候选称作“主要阻塞原因/典型优先级反转候选”，虽同时披露“仅作为窗口内验证方向”，故 B965 继续观察，禁止答案关键词硬门或系统代写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-041738 | answer_regex,answer_contains,mermaid_edge_count | none | 319s | 42 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=1,unavail=0,prune=2 | partial | operation-level participant touch 排序把运行时从 r610 的 590s/24 reads/6 repo_map/2 rejects 降到 319s/13/3/1，模型最终找到 `SetTurnAArtifacts` 与 `TurnAArtifacts` 真交接点。但第一次 exact repair 仍选到无关 `forcedReadCancelled(busCtx)`：真实 extractor 交接点此前已读却未 emit，被“只选未读位置”过滤。终图诚实保留 Mutable/BusContext 断开边界，正文仍泛化宣称完整共享数据流；B966 为明显改善但未闭环，新立 B968 修复 read-closure 与 evidence-emission 状态混淆。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
