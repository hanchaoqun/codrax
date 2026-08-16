# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T07:18:02Z
- sweep_start_ts: 20260816-001801
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-001802 | answer_regex,answer_contains | none | 199s | 34 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Analyzer emitted predicate_axis=define and is_relational_lookup=false despite the explicit finalizer relationship surface. The first diagram's unsupported typed edges were correctly rejected; the patch then deleted all edge_anchors but retained/re-authored five visible arrows, which were accepted because the typed relation axis was absent. The final answer asserts the shared evaluator/tool path while its system supplement simultaneously says the principal relation remains missing. Runner PASS is a shape/keyword false positive. |
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260816-001802 | answer_regex,answer_contains | none | 232s | 34 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | The model voluntarily read mermaid_render.go:101..300 and 901..1050 before completion, so the final answer now distinguishes replaceMermaidFence/renderMermaidFenceBody/mermaidFallbackFence and preserves the text-fence fallback. B879 did not queue a forced read in this stochastic run, so this is a no-regression/answer improvement witness rather than direct production activation. The answer still incorrectly says handleMermaidCmd calls TryRenderMermaidBlocks/RenderMermaidBlocks; that command only renders stats/help, so the REPL entry relation remains ungrounded. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
