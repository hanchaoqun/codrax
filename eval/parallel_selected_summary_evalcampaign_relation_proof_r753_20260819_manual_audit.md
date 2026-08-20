# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T02:21:48Z
- sweep_start_ts: 20260819-192147
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260819-192149 | answer_regex,answer_contains | none | 190s | 35 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=4/3,fin_reject=0,unavail=0,prune=0 | pass | The model-authored final Mermaid retained all 12 exact production `implements` edges and the matching file roster. The prior participant-boundary contradiction did not recur and finalization had zero rejects. Secondary B1208 remains: several evaluator duty descriptions cite only struct definition lines, so evidence precision is weaker than the requested relation/file result, but it does not invalidate that result. |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-192149 | log_regex,write_apply,answer_regex,answer_contains | none | 2195s | 36 | read=11,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Raw request and the preserved five-newline test both require one rank token, but write-analyzer `expected_outcomes` invented “5 -> 2” and the system minted it as required fallback/Workflow SuccessCriteria. The first wrong patch was correctly rejected by project tests; replan then spent multiple long active-output rounds reconciling mutually exclusive authorities and ended context-canceled with the bad patch only in a recovery ref. This is B1211 model-prose authority escalation, not a fixed-age stream cutoff or merely model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B1209-TYPERELATIONPARTICIPANTPARITY1`: production-positive in r753. Exact model-authored type edges survived both relation and participant gates; no system-authored edge was introduced.
- `B1210-REPORTLOCALPROOFRECONCILE1`: the replay did not reach the intended closed mixed-proof generation because an earlier authority error drove a wrong implementation. Unit/package pins remain green; production closure stays pending rather than being falsely inferred from this run.
- `B1211-MODELPROSEWORKFLOWAUTHORITY1` (P0/P1): analyzer `expected_outcomes[]` and plan acceptance strings are model-authored summaries, yet were automatically promoted to required verifier contracts and workflow SuccessCriteria. This allowed an invented result to compete with the system-bound request and an exact preserved project test.
- `B1212-ACTIVEPLANNERCONTRADICTIONCHURN1` (P1 consequence): an active planner stream remained correctly alive for more than eight minutes in individual rounds, but the contradictory authority pack consumed 2195 seconds and never rematerialized a plan. Root fix B1211 should remove the contradiction; do not add a fixed active-stream age cutoff.
