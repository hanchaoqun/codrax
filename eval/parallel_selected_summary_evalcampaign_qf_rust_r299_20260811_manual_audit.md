# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T07:09:43Z
- sweep_start_ts: 20260811-000941
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260811-000943 | answer_regex | none | 110s | 22 | read=3,repo_map=1,list=1,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 五条跨模块调用边、方向、调用点与 walker 的文件遍历角色均正确；首次图因缺 edge_anchors 被拒后，模型只替换图块并精确复制 typed capsule，正文关系未丢。`collect_files` 被称为返回“绝对路径”略有过度，源码只保证 `PathBuf` 转字符串，记为措辞校准观察项，不影响主结论。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-000943 | answer_regex,answer_contains | none | 466s | 42 | read=22,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=3,unavail=0,prune=7 | fail | 表格与正文覆盖四阶段输入/输出/载体，但最终 sequence 只剩三条 stage precedence；首稿已有 AnalysisIR/EvidenceItems/AnswerDocument 的读写交互，系统却只发布 precedence recipe。Explorer 两条 assignment 因 subject/object 不是源码 exact LHS/RHS 被降为 text_reference，tool summary 给出精确端点但声明 `Current actionable repair targets: none`，模型未重发，故值流关系确定性丢失。B510-A1 的 mixed repair 正向生效（重试重新携带三条 recipe），但 producer→repair 闭包仍缺。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner：2/2；人工：1/2。
- B510-A1 获得异构正证：Rust 的 typed call capsule 在 patch 修复中完整保留；pipeline 的 mixed relation + participant reject 也收到同一正向 capsule，不再退化为纯负面 mismatch。
- pipeline 失败不是模型随机遗漏。`emit_evidence` 已从精确源码行解析出 assignment 的真实 receiver/value，却在撤销错误 relation authority 后没有发布 action-required typed repair；completion 继续推进，Finalizer 因而只能消费 stage precedence。
- r299 二进制在 B510-A2 提交前构建，不能验收 actor/presentation 分层；下一批必须重新构建后 A/B 回放。
