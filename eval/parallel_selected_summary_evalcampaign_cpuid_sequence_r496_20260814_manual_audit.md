# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T15:52:41Z
- sweep_start_ts: 20260814-085239
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-085241 | answer_regex,answer_contains | none | 190s | 29 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | The answer correctly rejects a nonexistent directed `buildAnalysisIR -> gate.Run` path and renders the two proven arrows into the shared callee `gate.RunWith`. Mermaid is valid and survived the first final emit. Exploration was long because it initially tried to force the requested direction, and the 19-item intermediate-function list is verbose, but both are grounded rather than fabricated. |
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-085241 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 211s | 33 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer correctly reports CPU 5 / 8ms, one unsymbolized `libraw.so:0x1234` IP-only sample, and refuses function/line/call-path/hotspot-share claims. B810 is therefore final-layer production-positive. Earlier analyzer/explorer thoughts still repeatedly misread task identity `(20)` as CPU 20, and the first completion was wrongly rejected for source operation-flow proof despite typed `bounded_fact_set`; B811/B812 address these two generalized context/contract gaps. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- B810's deterministic final target-CPU roster prevented the earlier false CPU20→CPU5 migration, but this is only a final-layer fix. Perf triage already read CPU 5 correctly while analyzer and explorer independently hallucinated CPU 20 from `raw-21 (20) [005]`; the shared syntax boundary therefore belongs at all trace-reading LLM stages, as soft typed-context teaching rather than a prose gate.
- The Trace analyzer explicitly emitted `runtime_question_profile.scope=bounded_fact_set` with finite count/duration and frequency-residency families. Nevertheless, `PredicateAxis=flow` plus generic current-source allowance caused `flow_operation_carrier_evidence` to reject the first completion and demand source producer/transfer/consumer sites. This is a contract-routing bug: a finite external runtime fact does not become a current-source call-flow request. The second attempt only succeeded after an external-only waiver and retained an irrelevant “operation-level flow remains unproven” caveat.
- The sequence case eventually established the correct shared-callee topology: `buildAnalysisIR -> gate.RunWith <- gate.Run`. Its Mermaid compatibility shim repaired source quoting once; there was no finalizer rejection and no lost relation in the emitted diagram.
- Neither case used malformed-answer JSON salvage, prior-draft recovery, empty-answer fallback, or system-authored conclusion replacement. No fixed short-age/4ms active-stream degradation was observed.
