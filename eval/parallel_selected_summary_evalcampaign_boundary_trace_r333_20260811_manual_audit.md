# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T20:07:03Z
- sweep_start_ts: 20260811-130701
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-130703 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 30 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、自动补采、因果投影、链上/背景隔离及实际占用/规则可消双轴均保留；但模型在已收到 `pre_wakeup_dependency` 的 typed 机理上限后，仍把唤醒前重叠候选写成目标“被迫等待 VerifyClass 完成/直接阻塞”的确定机制。该 span 在 wake 后仍继续 0.400ms，席位也没有 typed target-blocking authority，因此 B544 仍是可重复的上下文显著性 gap。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-130703 | answer_regex,answer_contains,mermaid_edge_count | none | 233s | 30 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=1,unavail=1,prune=0 | fail | 最终图只保留 Analyzer→Explorer→Extractor→Finalizer；Orchestrator、BusContext、MutableState、TaskState 均为断开节点，请求要求的共享状态/数据流关系缺失。本轮 analyzer 的 typed `diagram_hint.participants=[]`，与其自身 relation scope/targets 不一致，故 B564 没有 participant obligation 可执行；首次草稿中的无证边被正确拒绝，修补后删边交付。归 B566/P2-watch：先观察 typed analyzer 清单波动，不以 relation quote 或答案原文扫描造硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
