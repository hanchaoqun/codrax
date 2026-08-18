# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T21:08:03Z
- sweep_start_ts: 20260818-140802
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_sink_impls | PASS | eval/results/sr_cpp_sink_impls-20260818-140803 | typed_inventory_rowset,answer_regex,answer_contains | none | 188s | 30 | read=8,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=4/0,fin_reject=1,unavail=0,prune=0 | pass-with-caveat | Parser/typed inventory correctly supplied the three concrete implementations and the two exact inheritance edges. The final answer preserved `ConsoleSink -> Sink`, `FileSink -> Sink`, and `RotatingSink -> FileSink`, gave all definition files, and clearly explained the indirect two-hop relation without inventing a diagram. The first emit duplicated table row values through `cells[]` plus `label/text`; the exact shape validator correctly rejected it. A second accepted document still missed hidden `member_set`, so a third patch added it. These were precise schema repairs, not contradictory contracts or evidence loss, but the two avoidable authoring repairs remain model/schema-load churn to observe. No internal relation enum reached the final answer. |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260818-140803 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 313s | 39 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | The final answer preserved the explicit 2.000..2.020s window, 20ms target sleep, `threadpool -> network -> cookie -> app`, on-chain IO #1=11ms, three disjoint 1ms scheduling-supply seats, measured-occupancy vs rule-eliminable axes, `Trace 因果投影`, and background demotion; no finalizer reject or active-stream/fixed-4ms degradation occurred. The model again fused/overstated semantics around the typed path: it described downstream application sync mechanisms as possible but did qualify them as follow-up. More importantly, visible prose copied internal vocabulary (`typed reason`, `window_total`, `pre_wakeup_exit_split`, raw state tokens). This is reader-language/context pressure, not authority to scan or rewrite the answer. The run did not exercise B1108b memo equivalence: all later requests differed materially by PID selector, explicit platform provenance, view, or wakeup depth, so zero memo hit and repeated indexes were correct. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `2/2 PASS`; human: C++ `pass-with-caveat`, Trace `partial`.
- B1108/B1108b did not receive a production positive or negative in this run because no two calls had the same effective selector/provenance/capacity identity. Unit pins remain the authority; this run must not be misreported as a cache failure.
- The C++ relationship surface is complete without a diagram: a table plus an explicit two-hop chain is sufficient for this question. Requiring Mermaid would be a format overfit and would let the system choose presentation for the model.
- The Trace appendix and model lead remain semantically useful, but customer-visible internal field vocabulary is still intermittent. Existing guidance correctly forbids copying it; because the user has prohibited hard gates over model/final prose and system answer rewriting, follow-up must reduce raw control-token exposure or strengthen typed-to-reader handoff, never add a substring reject/normalizer.
- Explicit-window Trace query/projection/automatic supplement, on-chain-only primary causes, off-chain support-only context, both time-concentration and rule-eliminable axes, and active-stream liveness were preserved.
