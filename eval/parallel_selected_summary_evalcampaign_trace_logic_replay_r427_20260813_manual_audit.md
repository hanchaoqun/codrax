# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T07:23:22Z
- sweep_start_ts: 20260813-002321
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-002323 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 259s | 46 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 显式 233.190ms 窗、目标五态、11 段 D-state、链上-only 排名、优先级/调度/供给/D/IO/确定性 JIT/业务线索、实际占用与规则计价可消除双轴均保留，邻近调度压力明确为非主因。模型仍越过 typed 上下文：把 `sched_blocked_reason.caller=dma_fence_default_w[devhost.elf]` 扩写成“DMA fence 由 devhost.elf 持有”，并把 12 条 caller 记录与 11 段状态账的不同口径解释成“截断溢出”；系统已明确 caller 不是 holder、记录账不能替代状态分区。这是模型未遵守精确边界，本批不按答案原文增加硬门。259s 活跃流未因 4ms 降级。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-002323 | answer_regex,answer_contains,mermaid_edge_count | none | 474s | 39 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=4/0,fin_reject=5,unavail=0,prune=0 | fail | B712 获生产正证：Analyzer 最终将 analyzer/explorer/extractor/finalizer/Mutable/BusContext 六席全部标为 incident_required；BusContext 获得 `out.AnalysisIR -> o.busCtx.AnalysisIR` typed data-flow。Mutable 精确补采两轮仍失败：repair 已携 callee `appendStageOutputEvidenceToMutable`，但下一窗固定提示“Run grep”，模型触发 894 行宽搜并被压缩，遗漏真正的 `stage_output_evidence_ingest.go` 函数 body；最终 Mutable 只能断开标未证，正文却概括为完整共享流。5 次成文拒绝均是未证边/参与者边界修补，没有发现 JSON schema 自相矛盾；B713 stale facet-softened footer 仍在。新确认 B714 typed repair 导航降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
