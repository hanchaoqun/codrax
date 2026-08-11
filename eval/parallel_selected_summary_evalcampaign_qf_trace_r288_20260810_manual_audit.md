# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T02:44:10Z
- sweep_start_ts: 20260810-194409
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260810-194410 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 固定 114.940ms 窗、自动补采、唤醒链、链上反转/调度/D-IO/算力/类校验与业务 span 均在；actual/effective、因果边界及背景隔离正确。r287 的泛化“结合源码”附注消失，B499 获生产正证。系统只并置 typed 投影与原始观察，未替换模型结论。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-194410 | answer_regex,answer_contains,mermaid_edge_count | none | 295s | 36 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=5,unavail=0,prune=0 | fail | 图仅保留三条 stage precedence，BusContext/MutableState 仍断开，正文却继续宣称共享数据流和无证写入职责。Principal Support Path 已收窄，但旧 Prior Stage Findings 仍把 128 条全局 FlowFinding/无关 Primary Evidence 旁路带入 finalizer；确认 B500。MutableState 边界身份因 `MS`/次级括注标签连续误解五次，给 B491 增加生产 witness。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- runner: 2/2; human: 1/2.
- `B499-RUNTIMEGENCAVEAT1`: production-closed.
- `B498-SUPPORTSCOPECTX1`: v2 only partially closes the finalizer context. The bounded Principal Support Path and typed enrichment are clean, but the legacy explorer StageReport is a separate, unscoped carrier.
- `B500-STAGEREPORTSCOPE1/P1-high`: render-only explorer StageReport inputs must reuse the same typed support scope as the required diagram. Preserve complete `StageOutput`/TurnA evidence and findings for audit and later consumers; only the finalizer-facing digest is scoped.
- Required-diagram flow support must require an exact evidence id or both ordered endpoints. Matching only one principal endpoint admits unrelated sibling flows and defeats the bounded scope.
- `B491-PARTALIAS1`: keep the exact participant identity contract fail-closed, but reduce repair cognition by explicitly asking for the exact node id or first visible label; do not accept secondary parenthetical aliases as identity authority.
- No raw request/model/final-answer prose gate, no system-authored relation/diagram/conclusion, and no Trace window/projection/root-cause/value-path change is authorized by this audit.
