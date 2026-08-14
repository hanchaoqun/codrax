# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T14:01:03Z
- sweep_start_ts: 20260814-070102
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-070104 | log_attachment,answer_contains | log_triage | 165s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | typed log-triage correctly published two peer errors and `cross_error_relation=unproven`, but an unsupported `behavior_outcome` aggregate with no support refs asserted a Cangjie→ArkTS propagation path; both contradictory carriers reached finalizer and the answer chose the invented relation. B802-LOGPEERCTX1. |
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-070103 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 209s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | requested 3.000..3.010 exceeded physical artifact tail 3.008; independent scheduler state sweeps extrapolated the uncovered 2ms as runnable, minted scheduler-latency/root seats, and the deterministic projection crowned it despite a flat/untraceable chain. Final prose also confused ftrace TGID `(20)` with CPU `[005]` and converted uncalibrated cycles to time. B801/B803/B804. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and disposition

1. `B801-TRACETAIL1` (P0, implemented): when a complete monotonic artifact ends before an explicit query endpoint, every scheduler-duration state machine must stop at `Index.LastTs`. The requested window remains unchanged for coverage accounting; the uncovered suffix is published as typed unavailable and cannot mint CPU busy/idle, runnable latency, or root-rank values. Ordinary selected windows whose artifact continues beyond the endpoint retain their established tail-open accounting.
2. `B802-LOGPEERCTX1` (P0, next batch): a multi-error `LogBundle` proves only peer occurrences unless an error carries its validated recursive `CauseRelation`. Model-authored runtime relationship synthesis facts with no typed support refs must remain in raw mutable audit history but be absent from answer authority and the observation ledger. This is a typed carrier projection, not label/value/prose scanning and not answer rewriting.
3. `B803-TRACECPUROLE1` (P1): finalizer context needs a compact typed ftrace header-role statement and/or explicit target CPU roster. Parenthesized TGID/process identity must never be interpreted as a CPU or migration; bracketed CPU and typed perf CPU are the lane authority. Use soft teaching/typed facts, not final-answer rejection.
4. `B804-RUNTIMEBREADTH1` (P1): the bounded hotspot/evidence-granularity question was classified as full causal diagnosis. Existing cross-field validator correctly rejected one contradictory object, but analyzer teaching still made the full-diagnosis lane the easier retry. Improve typed breadth teaching after P0 closure; preserve genuine root-cause/time-window causal projection and automatic supplement.
5. The model's `9000 cycles ≈ 3–9µs` conversion had no frequency calibration. Existing typed perf caveat already says sample weights are not elapsed duration; retain as a soft-context/final reasoning quality witness rather than introducing an output-prose hard gate.
6. Neither run experienced malformed-JSON recovery, missing-answer fallback, fixed 4ms/total-age degradation, or active-stream loss. Both streams completed normally.
