# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T15:34:00Z
- sweep_start_ts: 20260814-083358
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-083400 | log_attachment,answer_contains | log_triage | 109s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B808 worked: the finalizer structured-log section retained literal facts and no longer contained the triager's unbound propagation summary. The explorer nevertheless widened bounded_fact_set into a root-cause/propagation closure; the final answer repeated that unsupported edge. B809 must carry the typed finite scope into the explorer handoff as well as finalizer guidance. |
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-083400 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 124s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The answer again kept sample_weight as cpu-cycles rather than elapsed time/utilization, stated that one sample cannot establish hotspot rank/stability, and limited symbol resolution to module/IP granularity. No causal projection was required for this finite evidence-caliber request. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- Both cases emitted a valid answer on the first finalizer attempt; no malformed JSON recovery, final validation rejection, missing answer, or Mermaid repair occurred.
- B808 closed the context double-authority defect but exposed a separate scope-handoff defect: the log explorer did not consume the analyzer's typed bounded runtime breadth, unlike the trace-specific explorer path.
- B809 generalizes finite peer-error handling across observation-only log exploration and final composition. It uses only RuntimeQuestionProfile plus the validated peer-error shape, does not scan prose, and does not author or replace the answer.
- No fixed 4ms/short-age active-stream degradation was observed.
