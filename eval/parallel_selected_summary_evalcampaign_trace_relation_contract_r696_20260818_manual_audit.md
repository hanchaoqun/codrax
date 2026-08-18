# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T17:32:07Z
- sweep_start_ts: 20260818-103205
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-103207 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1093/B1096 的精确窗事故已闭：Runtime Root-Cause Board 与终稿都不再出现窗外 0.020ms/#2，只保留请求窗内 worker-200 链上 #1；Trace 因果投影、8.300ms 可消量、10ms 目标状态账、邻近/背景 support-only、实际占用/现规则可消双轴均保留，首稿直接成文且无 4ms 降级。人工仍不能签正确：模型三处把 sleep/唤醒先后扩写成“协作式等待上游工作完成/等待归因于 worker 工作完成”，而同一 Finalizer typed boundary 已明确 `target_wait_for_work_authority=not_provided`、`work_completion_dependency_authority=not_provided`、sleep 不证明等待谁/工作完成；这是模型越过已提供的精确边界。本轮未发现系统反向教学，先记模型波动观察，不用终稿关键词硬门或系统改写。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-103207 | answer_regex,answer_contains | none | 361s | 35 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=5/1,fin_reject=1,unavail=0,prune=0 | pass | 终稿事实与图均正确：明确无 `buildAnalysisIR -> gate.Run` 有向路径，分别画出 `buildAnalysisIR -> RunWith` 与真实反向 wrapper `gate.Run -> RunWith`，未造 `RunWith -> gate.Run`；Mermaid 合法，列表为 caller 内部函数而非伪 callee 链。B1095 教学冲突已消失，首稿 diagram anchors 已携 exact identities，重试从 4 降为 1。过程仍揭示 B1097：模型把既有 `ordered-list-2` 误放 add_blocks，保留 call_edge 但漏 anchors/surface_role；add→replace 归一化晚于 carrier inheritance，导致 principal 身份静默丢失、同一 empty-anchor 硬门未重现，终验只留 1 个 soft violation 即发射。可见答案未造边，但 patch “完整重校验”不成立，需按 schema 元数据根修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B1093-SELECTEDWINDOWPOSTBOUNDARY1`: production closed in r696; the 0.020ms post-boundary row is absent from principal prompt/board/final answer while the full ledger remains auditable.
- `B1096-EXPLICITWINDOWTOLERANCESEMANTICS1`: production closed in r696; exact requested-window authority is distinct from ±1ms disclosure grouping.
- `B1095-RELATIONANCHORSCHEMADRIFT1`: production closed for the contradictory teaching witness; relation direction and final answer are correct, with one remaining non-schema first-draft repair.
- `B1097-PATCHNORMALIZEDCARRIERESCAPE1`: new P0/P1. Existing-id add_blocks recovery occurs after omitted principal/facet carrier inheritance, so the recovered replacement can silently demote and evade the same merged-document relation hard gate. Reapply the narrow omission rule after operation normalization; preserve explicit clears and never inherit relation content.
- `trace-mechanism-boundary-model-variance`: observed. Typed guidance correctly forbade work-completion/synchronous-blocking claims, but the model overclaimed them in prose. Do not add a raw-prose hard gate or system rewrite; replay heterogeneously after higher-confidence contract work.
- `active-stream-4ms-degrade`: forbidden and not observed.
