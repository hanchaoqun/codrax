# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T08:15:22Z
- sweep_start_ts: 20260812-011521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-011523 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 119s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 233.190ms 窗、目标四态、typed 唤醒链、链上根因、算力/调度/D/IO/PI/确定性语义、业务线索、占用与现规则可消双轴、邻近降级、Trace 因果投影与自动补齐均保留。模型却把 `tail_open=8.793ms already_included=true` 说成窗外并与 1.396ms 未归账并列；又把 logd.writer/JankManager 等 adjacent 席称为“有效归因”。后者来自 Finalizer 自冲突：Axis B 从全 rank roster 取值，而 contextual lane 同时声明其 target causal authority 不存在。确认 B627/B628；只修 typed lane/账本优先级，不扫描或改写答案。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-011523 | answer_regex,answer_contains | none | 160s | 27 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最终表格和图完整列出 12 个 production `LoopController` 实现、文件位置与逐一实现关系，测试实现明确排除；图中没有虚构边，B626 的显式 required 图车道保持。Analyzer 先因 13 participants 超帽、后因自造 `Evaluator 实现类型` 与来源不符而拒绝两次，第三次以集合角色过关；这是软教学/模型服从效率观察项，未造成可见答案或关系丢失，不新增终稿硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: 2/2; human: 1/2.
- B626 production positive: explicit typed diagram intent still yields a complete relation diagram, while absent diagram intent no longer creates presentation debt.
- B627 is a generalized typed-contract conflict, not model variance: a positive rank does not grant target-causal authority to an `adjacent` or `background` seat.
- B628 is a final-accounting precedence gap: `tail_open/head_carry already_included=true` belongs to the selected-window state partition; only `unaccounted` is a separate unknown remainder.
- No final-answer prose scanner, system-authored diagnosis, relation synthesis, fixed-age stream fallback, or change to explicit-window Trace causal projection/automatic supplement is authorized by either fix.
