# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T19:57:07Z
- sweep_start_ts: 20260815-125705
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-125707 | write_plan,write_patch_oracle | none | 55s | 24 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | The one-line `retrun -> return` plan is correct and no plan reject occurred. The planner still emitted one valid Python import probe, so the user-visible retry is closed while optional-probe omission guidance is only partially adopted; the system must not delete a valid model-selected probe. |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-125707 | answer_regex,answer_contains | none | 123s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | The facts remain correct (`1`, `explorer`), but the table row again used only one citation and the registration citation was pruned. The count patch then quarantined its stale `citation_ref=1`; pre-emit treated derived `1` as a source literal and attached unrelated `internal/agent/explorer.go:19917` (`len(p)-1`). This is a system citation-authority defect, not a factual/model variance. Analyze also attempted unavailable `grep` after a completed source-inventory pre-scan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B850-PROBEOMISSIONGUIDANCECHURN1`: production-positive for the important outcome. The write plan was accepted on its first emission, so the prior plan-reject/retry loop is gone. The remaining probe is executable and language-compatible; omission is soft guidance, not a reason for deterministic rewriting or rejection.
2. `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1`: still partial. The finalizer had both registration and `Name()` anchors but again submitted only scalar `citation_ref` on the row. The unbound registration pool entry was correctly pruned. Do not turn this into a per-row multi-citation hard quota or auto-select a second anchor.
3. `B846-PATCHCITATIONIDENTITYREMAP1`: reopened by a new production shape. The investigator emitted a `member_set` aggregate without an explicit role. Request-aware compilation correctly presented it downstream as `role=principal_answer`, but scalar citation normalization checked only the stored raw role. It therefore failed to recognize the derived count and globally searched source literals, finding unrelated `len(p)-1`.
4. The generalized root fix is to compute the aggregate's effective role through `AnswerAggregateFactRoleForRequest` after typed demotions, and use that same effective role for visible-value authority. This consumes only the structured request model and aggregate fact; it does not inspect request, reasoning, block, or final prose for meaning.
5. No malformed JSON, prior-draft fallback, empty answer, stream-age fallback, Trace mutation, or system-authored conclusion occurred in this replay.
