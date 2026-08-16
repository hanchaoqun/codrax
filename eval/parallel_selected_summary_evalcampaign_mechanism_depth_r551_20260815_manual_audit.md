# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T04:59:29Z
- sweep_start_ts: 20260815-215928
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-215929 | answer_regex,answer_contains | none | 236s | 36 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=0,unavail=2,prune=0 | partial | B879 的 completion seam 在生产中生效：第一次完成被 `flow_operation_carrier_evidence` 驳回，模型补了 `TryRenderMermaidBlocks -> maybeReplaceMermaidFence` 的 parser-owned call edge，且没有成文 reject/patch。可是最终 principal member_set 选择的是 `OutcomeRendered/OutcomeUnsupportedKind/OutcomeLibraryRejected` 三个 enum，而不是 callable；新 semantic-descent 门因此没有排出 callee body read。Explorer 只用 repo_map 调用点完成，没有读取 `maybeReplaceMermaidFence` 784-867 或 `mermaidFallbackFence` 881-891。终稿仍只说明 library rejection 会触发重提示，遗漏 explicit-mermaid 最终失败/unsupported 均改写为 text fence、显示原因并原样保留源码，也漏列 OutcomeFallbackRune；runner PASS 仍是假绿。记为 B879b：从最终 accepted operation carrier/typed call edge 的 leaf 继续有界读取，不依赖 aggregate member 恰为 callable。 |
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-215929 | answer_regex,answer_contains | none | 395s | 33 | read=22,repo_map=2,list=0,trace=0,source_lens=0 | midloop=15,inv=6/0,fin_reject=5,unavail=0,prune=0 | partial | B880 确定性复现且更重：3 次 Explorer dispatch、22 次 read、5 次 finalizer reject、6 次 patch、395s。模型正文和表格正确说明两个 Name literal 与 full/patch 选择，但关系图的 guard/precedence/dispatch 边因没有匹配 typed carrier 被硬拒；修补后图只保留 `NewFinalizerAgent -> NewBaseAgent -> answerDocumentEvaluator`，`EmitAnswerDocument` 与 `EmitAnswerDocumentPatch` 节点存在但完全断开，随后又发布内部味很重的“未证关系边界”及整份系统保留第一稿。源码已有 evaluator 对两个工具的 struct/field/registration ownership，系统却不能表达，导致用户要求的主关系被删。runner 只检查工具名/表格/Mermaid 存在，无法识别关系缩水。B880 仍为下一高 ROI typed-carrier 批；禁止放松所有边或由系统代画。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
