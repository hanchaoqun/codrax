# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T02:48:49Z
- sweep_start_ts: 20260805-194848
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-194850 | answer_regex,answer_contains | none | 108s | 22 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | JSON was well formed and the final stage order/diagram was correct. The first emit mislabeled three stage-order arrows as `relation_kind=call`; the precise call-evidence gate rejected them and the patch changed them to `precedence`. More importantly, the final prose claimed responsibility details were unavailable and gave generic descriptions, although Explorer emitted all four exact StageBinding responsibilities and aligned aggregate `member_notes`. Finalizer projection dropped those notes: support-lane dedup retained only source-line surfaces and the supporting aggregate renderer omitted `member_notes`. This is a generic soft-context projection gap, not missing exploration evidence. |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260805-194850 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 120s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Production witness for the model-owned causal-caliber carrier: the final schema allowed only `no_causal_conclusion|bounded_window_candidate`; the model emitted `trace_causal_claim_caliber=bounded_window_candidate` on the principal summary in the first attempt and explicitly concluded that the selected-window chain was not a proven dropped-frame cause. Explicit `5.000..5.007`, pid/thread scope, three trace queries, auto-supplement, ranked causal projection, wakeup chain, actual-occupancy axis, and existing-rule-eliminable axis all remained present. The system did not rewrite the model lead. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- JSON/schema: neither case produced malformed JSON or lost its answer. Trace completed with one full emit and no reject. Diagram used one lossless string-array compatibility repair, then one semantic patch after the correct typed call-edge rejection.
- Trace red lines: causal strength remained model-authored and bounded by the final elected projection seat, not by a session-wide sticky probe. No prose scanner or deterministic conclusion rewrite fired; explicit-window projection and automatic completion were preserved.
- Confirmed generic gap: explanatory `member_notes` can disappear when their EvidenceItems are already represented in a principal support lane, because the lane keeps the source surface but not the model-authored semantic summary and the supporting aggregate prompt does not render notes. Fix the soft prompt projection, not the evidence hard gate or the final answer text.
- Retry simplification: surface the edge-kind decision before long Mermaid syntax guidance—workflow/stage order uses `precedence`; only a same-direction grounded invocation uses `call`. The existing hard validator remains unchanged.
