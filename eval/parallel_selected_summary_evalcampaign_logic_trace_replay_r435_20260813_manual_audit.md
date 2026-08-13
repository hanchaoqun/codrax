# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T12:36:45Z
- sweep_start_ts: 20260813-053644
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-053645 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗、typed 链、模型根因结论、系统因果投影与自动补采完整；自身 running/算力供给 65.912ms、D-state 36.757ms、链上反转/调度/IO、业务占时和未计价新方向均保留。零成文拒绝、零降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-053645 | answer_regex,answer_contains,mermaid_edge_count | none | 210s | 37 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | B723 未触发：解码后的 prose 与 diagram 本来已分块。新 P0 B724：Analyzer 实际漏发 question_kind/predicate_axis，执行器以 Go 零值静默接受，关系图掉入 presentation-only 车道；模型删掉 anchors 后可保留未证箭头。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- Trace 为强正证。最终 `20260813-053927.094-53612.md` 保持用户给定的
  `13762.791708..13763.024898` 窗；模型正文先给出链上根因排序与修向，系统投影随后提供精确
  账本和自动补采，没有替换模型结论。自身 running 真实占时 74.915ms、规则折算可消
  65.912ms，非 IO D-state 36.757ms、内核调用点 `dma_fence_default_w`，以及链上优先级反转、
  调度/算力/IO 和业务线索均未丢。`未计价占用` 另列 19 行、最大 15.960ms，明确作为真实占时/
  新修向而非冒充规则可消量或背景主因。
- 逻辑案例最终正文质量尚可，但关系证据合同被绕空。Analyzer 第一次 `emit_analysis` 的 JSON
  完全遗漏 schema-required 的 `question_kind` 和 `predicate_axis`；运行时 strict decoder 只拒未知
  字段和错误类型，不验证 JSON Schema 的 required presence，因此 RequestModel 落成
  `kind=unknown + axis=empty`。Finalizer 前两稿带 relation metadata 时被正确拒绝，最后删除所有
  anchors，却仍保留 Analyzer→Explorer→Extractor→Finalizer 与 containment 箭头；空 relation axis
  使该图被当作 presentation-only 而通过。它不是模型波动，而是 typed 路由权威静默丢失。
- B724 最优根修不是从题面、思考或答案文字推断关系，而是在 `emit_analysis` 唯一执行入口检查
  决定证据域的字段是否显式出现：`question_kind` 始终 presence-required；经当前请求 typed 展示
  授权后仍为硬要求的 diagram 还要求显式 `predicate_axis`，允许 `""` 表示确实无明确动作方向；
  未获授权的模型 `required=true` 先软化而不触发该门。其他已有安全默认/兼容语义的 profile
  不新增硬拒，避免制造普遍重试。Schema required 清单仍维持单源，执行器 presence 子集另有明确
  边界与正负 pin。
- B723 的生产正形本轮没有出现：模型把 `blocks[]` 外层误编码成 JSON string，但已有兼容解析将其
  还原后，summary 与 diagram 已是两个独立 block；因此 B723 的 full/patch 入口测试已闭环，生产
  fused 正证继续等待异构自然样本，不把本轮记成命中。
- 164s/210s 两条活跃流均没有因 4ms 未形成完整 answer 而降级。4ms 不拥有 active byte stream 的
  终止权；合法恢复入口仍仅是 caller cancel/deadline、无首字节、真实 byte silence、transport/
  decode failure 或重试耗尽，并须披露模型失败，不能由系统代写结论。
