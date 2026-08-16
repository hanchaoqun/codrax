# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T20:12:16Z
- sweep_start_ts: 20260816-131215
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-131216 | answer_regex | none | 202s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B930 production context positive: authoring capsule contains exact Python→native call, registered-export handoff, wrapper→core and core→helper edges. Final answer is readable and mostly correct, but the principal ordered list declared call/registration claim_uses while emitting zero edge_anchors, so B929 authority did not run and the claimed complete chain remains structurally unverified. B932 filed; no prose scan or system-authored bridge is acceptable. |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-131216 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 318s | 42 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=2,inv=3/2,fin_reject=0,unavail=0,prune=0 | fail | Trace data and seven query calls were available. Analyzer first emitted causal_contributor_set+causal_diagnosis, but after two local tuple rejects demoted the required causal role to stage_or_workflow and scope to bounded_fact_set. That accepted shape explicitly suppressed root-cause ranking/projection, so the answer omitted 65.912ms supply fold, 49.623/0.033ms split, incomplete enumeration and Trace causal projection; it also ranked raw S/running states as causes and expanded blocked_reason caller morphology into a GPU/display mechanism. B931 is a typed retry-contract gap, not data loss or ordinary model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusions

- B931 (P0): runtime causal breadth was redundantly encoded in five fields. A local repair to one field could therefore make the model erase its own already-declared causal dimension. The canonical authority is now the precise conjunction `runtime_question_profile.scope=causal_diagnosis` plus one required `causal_attribution|causal_contributor_set`; legacy intent/scenario/diagnostic fields no longer duplicate breadth authority.
- B932 (P1, open): a principal structured call-chain block can declare relation claim forms yet omit every endpoint anchor. This is a typed-schema omission, so the future hard check must consume family/block/surface_role/claim_uses only; raw request, item prose, final prose, and labels remain forbidden inputs.
- No active-stream age degradation was observed. Both cases completed through normal model streams; no 4ms/fixed-age fallback occurred.
