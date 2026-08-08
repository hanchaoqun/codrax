# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T15:55:40Z
- sweep_start_ts: 20260808-085539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-085540 | answer_regex,answer_contains | none | 149s | 26 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | R217 production proof is positive for the new carrier: `StageAnalyze -> StageExplore -> StageExtract -> StageFinalize` precedence anchors passed once the returned ordered slice was reached. Only the unsupported boundary edges `Request -> StageAnalyze` and `StageFinalize -> AnswerDocumentV2` remained. The repair loop nevertheless deleted all already accepted anchors and then all visible arrows, leaving six disconnected nodes. Runner PASS only proves names/fence, not the requested pipeline flow. No malformed JSON, raw-prose hard gate, system conclusion rewrite, or Trace lane was involved. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-085540 | answer_regex,answer_contains | none | 305s | 34 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=5,unavail=0,prune=0 | fail | Analyzer emitted typed `predicate_axis=flow`, so the strict owner lane correctly rejected invented `runReadPipeline()` calls and invented Agent-to-Mutable assignments. The model then removed all arrows but retained the nonexistent `runReadPipeline()` node and unsupported prose claims: it attributes analyzer/explorer/extractor/finalizer writes and final output storage to concrete MutableState fields without matching call/assignment evidence. The 24,689 -> 128 flow handoff was bounded yet dominated by irrelevant helpers/tests and exposed no verified component-level transfer path. The system stage-binding supplement was explicitly separate and did not replace the model answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
