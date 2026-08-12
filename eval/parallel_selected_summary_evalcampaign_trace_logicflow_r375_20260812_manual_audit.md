# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T08:39:26Z
- sweep_start_ts: 20260812-013924
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-013926 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 200s | 45 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | B627 生效：49.623ms 等 adjacent 席只作为背景，未再进入有效归因；显式窗、链上排名、双轴、投影与自动补齐均保留。但模型仍把 typed `tail_open=8.793ms, already_included=true` 写成“已排除/部分尾部未计入”。Explorer 无 support_refs 的模型 scalar 被请求形状误授运行时主值权限，和最终 principal_state 账本冲突；B628 软教学不足，立案 B629。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-013926 | answer_regex,answer_contains,mermaid_edge_count | none | 353s | 36 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=5,unavail=0,prune=0 | fail | 图最终保留四阶段 precedence 与 analyzer/explorer→Mutable accessor 的已证调用，但 BusContext 被迫断开；正文却又称它与 MutableState 保持连接，内部矛盾。系统已经有 `state_carrier(owner=BusContext, field=Mutable, type=*MutableState)` 精确事实，却把它降成 context-only 且 participant gate 不把 containment/grouping 当关系表达，造成 5 次成文拒绝。立案 B630；不应让系统生成边，只应把 typed carrier 作为可选分组/包含关系权限交给模型。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `2/2 PASS`; human: `0/2`.
- H7 is a numeric provenance conflict, not a missing trace observation. The exact principal-state ledger was present and correct. A request-level runtime classification must not authorize a model-authored scalar/count without a typed support reference.
- The logic-view failure is a graph expression-layer gap. A field carrier proves containment/ownership but not arbitrary data-flow direction; the correct capability is to expose that typed relation to model-owned grouping, not to synthesize an arrow or replace the answer.
- Both runs remained active beyond ordinary short-response latency and completed normally. No elapsed-age downgrade, old-draft substitution, or empty answer occurred; only precise transport/byte-silence/cancel/safety/decode signals may enter recovery.
