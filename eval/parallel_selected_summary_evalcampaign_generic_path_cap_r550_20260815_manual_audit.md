# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T04:23:59Z
- sweep_start_ts: 20260815-212357
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-212359 | answer_regex,answer_contains | none | 164s | 32 | read=6,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B878 production witness closed: two independent mechanism-topic ordered lists no longer hit a false global MaxCount and finalizer completed with 0 reject/0 patch. Config half is grounded. Mermaid half remains materially incomplete/contradictory: explorer read the file header and `RenderMermaidBlocks` wrapper but not the actual `maybeReplaceMermaidFence` failure branches; the final answer says source is merely kept/quietly degraded and even names library rejection both as the only retry case and as an example of a quiet failure. It omits the actual L7-visible behavior: failure/unsupported paths rewrite to a `text` fence and prepend a warning while preserving source. Runner PASS is therefore a false positive for mechanism correctness (B879 remains open). |
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-212359 | answer_regex,answer_contains | none | 185s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | partial | Exact Name() literals and first-emit versus retry-patch timing are correct. The requested relationship diagram is not: the first evidence-poor graph was rejected, the repair relabelled unsupported edges, the second reject then caused the model to delete both tool nodes and finish with only `NewFinalizerAgent --> answerDocumentEvaluator`. This is technically renderable but no longer shows the two tools' relationship requested by the user. The system had struct-literal/field ownership evidence in source but no typed carrier for it, so a hard relation gate converted missing evidence expressivity into answer shrinkage (B880). |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- B878 is closed by production replay: the generic mechanism-path contract now requires at least one path without asserting an unproved global maximum, and the former normalizer/reject loop is absent.
- B879 is a producer/callee-depth evidence gap, not a wording gap: mechanism answers can complete after reading an entry wrapper or header comment while omitting the implementation branch that determines visible behavior.
- B880 is a typed relationship-carrier gap: constructor/aggregate ownership and tool registration are real source relationships, but the current diagram contract cannot express them. Hard rejection therefore encourages deletion of useful relationships. The repair must add source-derived typed relationship support or soften only the unsupported presentation edge; it must not synthesize a replacement graph or infer relations from answer prose.
