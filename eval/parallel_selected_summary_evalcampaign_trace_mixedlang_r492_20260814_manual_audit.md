# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T15:09:32Z
- sweep_start_ts: 20260814-080931
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-080933 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 128s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The old `1 sample / 8ms = 12.5%` error disappeared, but the final still called one observation a hotspot and said 9000 cpu-cycles provided a rough CPU-occupancy ratio. Event weight, observed-cohort share, elapsed time, temporal coverage, and workload-hotspot confidence remain distinct. |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-080933 | log_attachment,answer_contains | log_triage | 133s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B802 context cleanup worked: no investigation narrative, no peer member-note relation, and the final tail explicitly said cross_error_relation=unproven. The model nevertheless called Cangjie the true trigger and ArkTS the capture/propagation side, then contradicted itself with the unproven paragraph. Analyzer also spent three iterations because scope teaching falsely forbids diagnostic bounded_fact_set although the validator accepts that exact tuple. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Evidence-backed conclusions

1. `B802-LOGPEERCTX1` is production-positive: the finalizer prompt has two principal peer occurrences, the explicit tail boundary, and no `Investigation Narrative Handoff`; raw audit history remains intact.
2. `B806-RUNTIMERELATIONASSERT1` should not immediately become a prose-scanning hard gate. The clean-context model still made an internally contradictory cross-peer claim, so the next safe step is stronger typed peer-only soft guidance and further heterogeneous replay. A model-filled enum alone would not prevent contradictory free text.
3. `B807-RUNTIMESCOPECONTRACT1` is a confirmed contract-teaching drift. `AnalysisRuntimeScopeFromDimensionTeaching` says bounded_fact_set requires non-root-cause intent and all diagnostic flags false, while emit_analysis accepts root_cause/diagnostic plus bounded_fact_set for finite crash-frame observations. This caused two avoidable analyzer rejects. Align teaching with validator; do not broaden causal projection.
4. `B805-PERFSAMPLECALIBER1` remains confirmed. Provide a typed statistical-caliber observation and a finalizer-tail explanation: sample count is not elapsed coverage; sample weight share is within an observed same-event cohort, not CPU utilization; workload-hotspot confidence and temporal coverage are unavailable without sampling-design/representativeness receipts. This remains model guidance, not system-authored answer text.
5. Both answers were complete on the first finalizer attempt. No malformed JSON, draft recovery, Mermaid failure, empty answer, fixed-age active-stream downgrade, or four-millisecond answer cutoff appeared. Trace explicit-window facts stayed bounded and no full causal projection was forced for this finite perf-fact request.
