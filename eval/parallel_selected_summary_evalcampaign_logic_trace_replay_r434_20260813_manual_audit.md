# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T12:10:28Z
- sweep_start_ts: 20260813-051025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-051028 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 335s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | 显式窗与五次 typed 查询一致；链上-only 主因、running 65.912ms、非 IO D-state 36.757ms、反转/调度/算力/IO、业务线索及实际占用/规则可消双轴保留。两次拒绝均为模型遗漏必需块/范围披露修形，最终无降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-051028 | answer_regex,answer_contains,mermaid_edge_count | none | 437s | 42 | read=8,repo_map=4,list=0,trace=0,source_lens=0 | midloop=13,inv=4/0,fin_reject=7,unavail=0,prune=0 | partial | B722 生产闭环：共享 normalize 触发一次，最终 Mermaid 无 `-..-`、无 synthetic operator node，合法 fence 和多条关系边保留。新 B723：首稿把 text+diagram 融在 summary；现有 split 只认 rows/columns，故 discriminator 把 summary 改为 diagram 并触发长修补链。后续另有模型 JSON/string/kind 波动和真实关系门拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- B722 已获生产正证并关闭。最终 `20260813-051742.857-38787.md` 为合法 `flowchart TD`；没有 `-..-` 或 `codraxNode["-..-"]`，也没有进入 text fallback。生产指标记录一次 source repair，说明不是模型碰巧绕开修点。
- B723（P1）是通用结构/JSON 心智负担 GAP。模型首次提交的 summary 同时携带完整 `text` 与 `diagram`。现有 fused-block splitter 只在 `items/columns` 可见时拆成“原 block + diagram block”，对 prose 可见载体不拆；后续 normalize 因 typed diagram sibling 把 discriminator 改成 diagram，summary 文本和 diagram 的所有权混在一起。模型随后花三轮才完成 carrier split，其中包括缺 `kind`、把 `participant_boundaries` 放在 summary、以及把 `add_blocks` 写成畸形 JSON 字符串。
- 最优方案是扩展已有 typed fused split seam：任何有效非 diagram block 同时有非空 normalized diagram 与其 kind 对应的可见 payload（text 或 rows）时都做同一无语义选择的 carrier split。保留原 id/kind/text/rows/claim/facet/surface；派生 diagram half 只带 diagram/edge anchors/boundaries。它不读题面/答案关键词、不画边、不改模型结论，也不降低后续关系证据门。
- 其余关系拒绝并非 Mermaid 语法问题：模型首稿画了大量无 anchor 的容器内部边，validator 正确拒绝；终稿只保留已证 local call/precedence，`BusContext/Mutable` 诚实列为 requested-flow unproven。该结果关系表达仍为 partial，但不能通过系统代画或放宽证据杆硬闭环。
- 两个 335s/437s 活跃流都没有固定总年龄降级。4ms 仍只属于无 streaming liveness owner 的 terminal evaluator 微等待；存在活跃字节流时，不会因 4ms 未形成完整答案而降级。
