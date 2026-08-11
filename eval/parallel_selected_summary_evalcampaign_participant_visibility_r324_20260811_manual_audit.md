# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T17:27:23Z
- sweep_start_ts: 20260811-102722
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-102723 | answer_regex,answer_contains | none | 112s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 正文正确说明 `Logger.log -> Sink.write -> ConsoleSink.write -> fputs` 与 `make_sink -> SinkRegistry.create -> ConsoleSink` 两段，但首稿混写未证 call 边后，系统 copy-ready skeleton 只保留 3 条直接 call。最终图成为 `Logger.log -> Sink.write/flush` 与 `make_sink -> SinkRegistry.create` 两个断片，虚分发、guard、factory return/construction、concrete type binding 全部从图中消失。日志的 typed return/type/guard facts仍在，说明是 mixed-relation diagram projection 只选 call family 的系统 GAP，不是模型波动；归并 B538，并立 B551。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-102723 | answer_regex,answer_contains | none | 160s | 25 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass-with-caveat | 模型一次将 implements 方向画反，精确 type_relation gate 给出逐边方向修补后，最终 12 个生产实现均以 `implementer -> LoopController` 保留，文件和逐项 citation 完整；B548 未误伤合法业务 participant/type graph。系统末尾却追加“枚举类条目中部分项证据支持稍弱”，与 12 条 typed relation+citation 相矛盾，确认 B550：通用 weak-support supplement 仍可跨题族误发。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
