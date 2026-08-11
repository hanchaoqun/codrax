# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T06:21:32Z
- sweep_start_ts: 20260810-232130
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-232132 | primary_answer | none | 112s | 22 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B508 v2 正确禁止了无选择时的 sibling-leaf 回退，模型也精确读到 `AuditLog.java:5-7`；但首批 `emit_evidence` 漏发已读 `audit -> AuditLog` initializer。active-loop evaluator 仍看 dispatch-start 旧 evidence snapshot，未在该轮发 selection hint；completion downgrade 的 `RepairEmitEvidence` 又只渲染文件表，丢失 producer-owned rationale，模型误以为 aggregate 格式错并直接重试 completion，低增量车道随后强制收口。`AuditLog.record -> System.out.println` 因无 selection 仍未成为 body-call fact，最终继续声称“审计写库/持久化到数据库”。确定为 live typed-state 与 repair-context 两个通用接线 gap，不是模型波动。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260810-232132 | answer_regex,answer_contains | none | 634s | 46 | read=16,repo_map=5,list=0,trace=0,source_lens=0 | midloop=14,inv=3/0,fin_reject=7,unavail=2,prune=3 | fail | 表格/正文大体可用，但经历 7 次成文拒绝、3 次探索/提炼分派和 634s。最终 sequence 仅保留 `Run -> runAnalyzePhase/emitAnalysisReady` 与 `applyStageOutput -> appendStageOutputEvidenceToMutable` 三条实现 call；`analyze/finalizer/AnalysisIR/EvidenceItems/AnswerDocument` 被保留为无关系 boundary，StageExplore/StageExtract 的时序没有可见关系。多轮中模型持续把 stage dispatch、state transfer 与 call/precedence/data_flow 错配，证明 B510 是跨 relation kind/diagram kind 的合同与上下文组织问题，不能按单箭头修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
