# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T14:51:12Z
- sweep_start_ts: 20260820-075110
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260820-075112 | write_apply,write_patch_oracle,answer_contains | none | 92s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Write mode completed analyze→plan→apply→verify→finish. The applied commit changes only main.c retrun→return; make test passed 1/1. No plan/verification contract conflict and no time-based degradation. |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-075112 | answer_regex,answer_contains | none | 206s | 33 | read=16,repo_map=4,list=0,trace=0,source_lens=1 | midloop=5,inv=2/1,fin_reject=1,unavail=0,prune=0 | pass-with-source-supplement-caveat | B1242 production positive: analyzer accepted only concrete LoopController as the request participant; all 12 production implementers and exactly 12 same-direction implements edges are present; no false unproven boundary remains. The one finalizer rejection was missing item evidence_ids, not a contradictory contract. Independent B1244 persists: system source-anchor supplement mislabels agent.go:516 as LoopObservation.ToolAvailable instead of LoopController at line 519. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B1242 is production-closed. The generic implementation-member collection is no longer an impossible diagram actor, while discovered concrete implementers remain model-authored from typed evidence.
2. The analyzer's first rejected attempt tried to promote discovered implementers into request participants. Existing provenance validation correctly rejected that shape; the accepted retry kept only LoopController. This is a healthy precise gate, not a new GAP.
3. The source-location supplement is independently inaccurate and remains visible even though the model answer is correct. It is recorded as B1244 and must be fixed at the typed anchor-owner selector, not by rewriting model prose.
4. Both streams remained active and delivered current outcomes. No 4ms/4m/first-byte/stall/total-age degradation path fired.
