# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T16:53:08Z
- sweep_start_ts: 20260814-095307
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260814-095309 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B815 生产正证：Analyzer 首稿的 typed breadth 冲突被 fail-loud，第二稿改为 coherent `causal_diagnosis` 并保留 4 个维度；最终恢复完整 Trace 因果投影、worker-200 链上优先级反转候选、8.300ms 有效归因/9.000ms 累计以及实际占时与规则可消双轴。人工不能签 pass：模型把 8.300ms runnable 错写成“持续占用 CPU”，又把 app 的 10ms sleep 过度归成完全由 worker runnable 贡献；Finalizer 上下文已准确说明 runnable 是 off-CPU 排队等待、跨 CPU 不证明竞争，因此属于模型忽略准确上下文，不加答案原文硬门或系统代写。一次 reject 仅为 citation_ref 修正；无空答案、JSON 恢复或 4ms 降级。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-095309 | answer_regex,answer_contains | none | 210s | 31 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=1,unavail=0,prune=0 | partial | 代码事实和文字边界正确：不存在 `buildAnalysisIR -> gate.Run` 有向路径；二者分别调用 `gate.RunWith`。Explorer 已发出 9 条 typed 直接调用证据，第一版图也画出全部 9 条真边，但只给两条端点边写了 `edge_anchors`。Validator 正确拒绝 7 条缺锚边；错误发生在系统修补提示，它用仅含两条端点边的 copy-ready 骨架替换原图，诱导模型删掉 7 条已有证据的中间关系。终稿列表仍保留中间函数，但图关系缩水。登记 B816：已证可见边仅缺元数据时必须保持模型 Mermaid body/措辞/顺序不动，只补完整 anchors；无证或混合失败继续 fail-closed，系统不代画关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
