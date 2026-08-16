# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T05:20:35Z
- sweep_start_ts: 20260815-222033
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-222035 | answer_regex,answer_contains | none | 152s | 31 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | B880 以更短路径稳定复现。首稿试图画 finalizer/evaluator/dispatch/full/patch/output 的主关系，relation gate 因缺 typed ownership/dispatch carrier 正确拒绝；模型随后删除所有工具关系，只留下 `NewFinalizerAgent -> NewBaseAgent`，两个工具成为断开的孤点。Name literal、首次 full 与 retry patch 的表格说明正确，但用户明确要求的“它们在 finalizer 里的关系”仍未由图表达。1 reject/1 patch 比 r551 收敛，但不是内容闭环；source ownership carrier 仍是下一高 ROI 根修。 |
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-222035 | answer_regex,answer_contains | none | 226s | 30 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 本轮 Analyzer 把双机制题的 principal axis 归为 configure，因此没有 flow-operation repair，B879b 的 operation-leaf seam 未触发；这不是 B879b 代码回归。更强的新 witness 是 Explorer 已显式发出 `evidence_kind=mechanism + definition(RenderMermaidBlocks/SanitizeDegradedMermaidBlocks/BuildDegradationLedgerView)`，completion 仍只按无 member_notes 的 roster 放行，没有要求读 callable body。终稿因此把普通 `RenderMermaidBlocks` 说成返回 Outcome，把仅用于 rejected/text-recovered draft 的 `SanitizeDegradedMermaidBlocks` 接到普通 REPL 失败链，又把通用 DegradationLedger 当作 Mermaid 主链组成；实际 explicit Mermaid 失败由 `maybeReplaceMermaidFence/replaceMermaidFence -> mermaidFallbackFence` 改写 text fence、显示原因并保留源码。记 B879c：以模型已选、fact support 精确绑定的 typed mechanism definition 作为跨 predicate-axis 的下钻 seed，不能依赖 flow gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
