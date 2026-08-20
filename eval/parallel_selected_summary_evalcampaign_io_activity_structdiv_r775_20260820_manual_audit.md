# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T14:22:45Z
- sweep_start_ts: 20260820-072243
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-072245 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-wording-caveat | B1241 production positive: explicit 2.000–2.020s window, causal projection, auto supplement, 11.000ms on-chain IO root and three 1.000ms scheduler seats all preserved. Background io_pressure appears once as typed non-wall-clock activity index and has no window/chain/effective value. Residual user wording still says IO pressure/综合评分 instead of activity index; queued as B1243. |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-072245 | answer_regex,answer_contains | none | 278s | 38 | read=13,repo_map=3,list=0,trace=0,source_lens=1 | midloop=10,inv=3/1,fin_reject=5,unavail=0,prune=0 | partial | B1240 production positive: all 12 production implementers and exactly 12 same-direction visible implements edges are present, without the old weak-evidence caveat. Analyzer incorrectly made the unresolved collection label 实现类型 an incident participant, so the system required an impossible boundary and caused five finalizer rejects/six patches; final answer therefore contradicts its complete graph. B1242 fixes the typed planning source. The system source-anchor supplement also identifies LoopObservation.ToolAvailable rather than LoopController and is queued separately. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. Trace answer correctness is materially good. The chain-only root-cause rule is obeyed; app-100/cookie/network sleep remains symptom or context, while the chain IO wait and runnable supply delays carry the conclusion. The typed IO activity index no longer impersonates elapsed milliseconds.
2. Type answer content and graph are materially complete, but the system-appended `未证关系边界` is false. This is not model variance: the analyzer emitted a generic collection role as a participant, and the finalizer faithfully followed the contradictory typed contract.
3. Both streams remained active and returned current model answers. No 4ms/4m/first-byte/stall/total-age degradation path fired.
