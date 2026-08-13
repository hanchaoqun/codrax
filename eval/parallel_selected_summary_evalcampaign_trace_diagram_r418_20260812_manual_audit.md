# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T03:32:42Z
- sweep_start_ts: 20260812-203240
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-203242 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、五态守恒、NetworkService→CookieMonsterCl→目标链和 T7→目标链完整。链上 priority inversion 5.951ms、VerifyClass .285ms、调度供给与算力供给席位均保留；业务 span、实际占用/新修向、规则计价可消除量两轴齐全。邻近/背景只支撑不晋升，frame absent/missing wakeup 边界诚实。无 4ms 活跃流降级。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-203242 | answer_regex,answer_contains | none | 211s | 26 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 最终答案正确保留 12 个 production implementer、文件表和 12 条 implementer→LoopController 关系，没有错误系统 caveat。但首稿是 `flowchart TD` 中混入 class-only `<\|--`；既有自愈把它转成反方向，触发 24 项关系 mismatch 和一次 patch。另 Analyzer 因 13 个 participant 超过固定 12 cap 连续返工，虽这些 inferred participant 最终会按 provenance 丢弃。两者均是确定性合同 GAP，不是最终知识错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `B701-MIXEDCLASSFLOWDIRECTION1`：flowchart 里出现 `<|--/--|>` 是语法载体混用，但 UML 语义仍精确。syntax-only normalizer 必须按与 classDiagram ParseEdges 相同的 subtype→supertype 方向转成标准 flowchart edge；不得按字面左右顺序反向。引号内 token、classDiagram 本体及不唯一的复合行仍不动。
- `B702-DIAGRAMPARTICIPANTCAP1`：固定 `maxItems=12` 与来源权限无关，会在宽关系图上制造 Analyzer 重试，并诱导模型删掉明确参与者。participant 已有 exact current-request provenance、去重、长度、role 和 relation-scope 校验；移除固定数量 cap，保留所有语义门。它们仍只是 planning guidance，不能铸边。
- r418 最终图通过说明 B698/B699/B700 的必要性；本轮残余是首稿自愈方向与任意数量上限，不回退“必须有图”或 typed relation evidence 门。
