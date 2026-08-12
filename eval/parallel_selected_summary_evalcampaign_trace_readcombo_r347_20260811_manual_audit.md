# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T00:20:49Z
- sweep_start_ts: 20260811-172048
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-172050 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Deterministic context preserved the explicit 5.000..5.007s window, four-state accounting, on-chain VerifyClass 4.600ms plus runnable 0.800ms ranking, actual-vs-eliminable axes, absent frame evidence, causal projection, and the B581 pre-wakeup caliber. The model nevertheless reversed the typed temporal boundary by saying VerifyClass completed before waking app-100 although the span ends 0.400ms after sched_wakeup, and narrated idle/1 as CPU competition. This is model-authored semantic drift, not a missing typed trace row; do not repair it by scanning or replacing final prose. Analyzer also attempted the wrong JSON carrier: it set answer_exclusion_policy=true with an empty role set for the request's source-analysis exclusion even though external_observation_policy=exclude is the dedicated carrier (B585). |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-172049 | answer_regex,answer_contains | none | 323s | 42 | read=25,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=4,unavail=0,prune=2 | fail | B583 production positive: the final five-column stage table uses one row convention and no cell is shifted. B584 production positive: entity provenance reports three ambiguous symbols, source_operation_required is empty, request_visible_boundary_only contains only analyze/finalizer, and the impossible codrax/stage source-operation loop is gone; exploration fell from 47 iterations/499s to 25/323s. Four finalizer rejects remain. The accepted sequence diagram expresses two forward calls with standalone dashed -->> response syntax, so the visual relation semantics are misleading even though exact call anchors exist (B586). The prose/table also overstates several StageOutput members without a direct struct-field citation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
