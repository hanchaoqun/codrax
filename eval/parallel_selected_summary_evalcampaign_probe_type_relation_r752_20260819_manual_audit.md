# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T01:54:34Z
- sweep_start_ts: 20260819-185433
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260819-185434 | answer_regex,answer_contains | none | 316s | 38 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | Runner false positive. Analyzer/repo-map and finalizer context carried 12 exact production `member -> LoopController` implements recipes. The model's first draft rendered all 12 correctly, but participant coverage ignored exact repo-map type relations, required an unproven boundary, then rejected that boundary as connected. Four deterministic finalizer rejects drove the model to delete every relation edge; the final diagram is only a node roster. This is a same-evidence-pool contract conflict, not model variance. The summary also says implementations provide `ShouldContinue`; current interface evidence says `Observe`, so factual synthesis remains a lower-priority model/context precision issue after relation preservation. |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-185434 | log_regex,write_apply,answer_regex,answer_contains | none | 377s | 26 | read=8,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | partial | Patch is correct and both named project assertions pass. The proof-only follow-up probe also passes and the final cumulative proof has `status=strong`, `impact_target_count=5`, `impact_verified_count=5`. The active single-report profile nevertheless keeps `verification_probe_missing_soft_contract_ref` for outcome-1..4 even though exact project-test receipts cover those refs, so both proof batches are marked unverified. This is report-local typed proof reconciliation drift, not a code or test failure. B1206 malformed-probe production arm was not exercised because this probe was syntactically valid. No fixed-age active-stream cutoff occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized findings

- `B1209-TYPERELATIONPARTICIPANTPARITY1` (P0/P1): the ordinary diagram relation gate re-queries exact request-scoped repo-map type relations, while participant coverage consumed only Explorer-authored flow operations. The same model-authored exact edge therefore passed one hard gate and failed another. Root fix: one typed requested-relation evidence lane, with exact kind/target/direction matching; no model prose or diagram-label scan, and no system-authored edge.
- `B1210-REPORTLOCALPROOFRECONCILE1` (P1): report-local proof strength retained a missing-probe-ref warning after an independent exact project-test receipt covered the same contract ref. Cross-report cumulative code already resolves the identical typed shape. Root fix: apply the same exact-ref reconciliation before assigning single-report proof strength; unrelated refs remain unable to close the warning.
- `B1206-INLINEPROBESYNTAXPREFLIGHT1`: no production exercise in r752. The new plan contained valid Python and therefore passed admission; retain unit-covered/await-production-replay.
- `B1208-DIAGRAMRELATIONCONTEXTPRECISION1`: still open at lower priority. The interface method-name error (`ShouldContinue` vs `Observe`) is visible evidence/synthesis drift, but it did not cause the systematic relation deletion and should not be hard-fitted into the two validator fixes above.
