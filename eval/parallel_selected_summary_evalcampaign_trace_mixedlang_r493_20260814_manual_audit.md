# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T15:28:53Z
- sweep_start_ts: 20260814-082852
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-082853 | log_attachment,answer_contains | log_triage | 81s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | First finalizer emit, zero validation retry. The typed peer boundary reached the finalizer, but the structured Log Triage section also replayed an unbound triager interpretation saying ArkTS captured and propagated the Cangjie error. The answer therefore first says the peer relation is unproven and then asserts root-cause/propagation. This is a system context-authority conflict, not a clean-context model fluctuation (B808). |
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-082853 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 125s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | First finalizer emit, zero validation retry. The typed perf statistical boundary reached the finalizer. The answer correctly treats 9000 as a cpu-cycles event count, not elapsed time/utilization, rejects workload-hotspot certainty from one sample, and leaves temporal coverage unavailable. No Trace causal projection was required for this bounded sampling-caliber question. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- Both cases completed with one investigation closure and one finalizer emit. There was no malformed JSON recovery, missing answer, Mermaid repair, or answer-document validation retry.
- No fixed short-age or 4ms downgrade path was observed. Runtime completion followed typed pipeline state.
- The Trace case preserves explicit-window analysis and deterministic supplementation without inventing a causal projection for a question that asks only sampling-evidence caliber.
- The mixed-log case proves that downstream typed guidance cannot compensate for an earlier contradictory model-authored summary. B808 removes that competing summary only for the typed multi-top-level-error shape while preserving literal observations and the model's authorship.
