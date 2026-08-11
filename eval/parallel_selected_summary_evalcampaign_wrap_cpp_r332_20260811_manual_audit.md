# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T19:50:09Z
- sweep_start_ts: 20260811-125008
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-125010 | answer_regex,answer_contains | none | 196s | 24 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Logger→Sink→ConsoleSink→fputs/fputc 的调用与虚分发完整，SinkRegistry::create 的 console/file/rotating 三个选择分支由 typed unary Note 保留；一轮修补后 call/guard/return 图层没有越权，`::` 身份无回归。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-125010 | answer_regex,answer_contains,mermaid_edge_count | none | 499s | 32 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=13,inv=5/0,fin_reject=3,unavail=0,prune=0 | fail | B563 正证：显示换行后的 exact endpoint 未再被误判，拒绝从 14 降至 3。最终图却让 `n13[Mutable]` 承载 `MutableState.AppendEvidence` 的已证调用边，同时保留 `participant_boundaries[{Mutable,unproven}]` 并在正文称其关系未证；“unproven 必须为断开节点”只有教学无结构校验。另有采集残余：已读到 `Mutable *MutableState` 字段声明却未发 typed declared-binding，现有 binding resolver 无证据可消费。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
