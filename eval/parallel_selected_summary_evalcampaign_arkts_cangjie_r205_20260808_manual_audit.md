# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T09:41:15Z
- sweep_start_ts: 20260808-024114
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260808-024116 | typed_inventory_rowset,answer_contains | none | 81s | 21 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | All four Entry types and both Builder functions are present in structured tables with exact file/line citations. The model JSON-encoded blocks[] as one string; the existing bounded flat-mode repair recovered it losslessly with no retry or row mutation. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260808-024116 | typed_inventory_rowset,dimension_substring,answer_contains | none | 119s | 23 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The 2 extend, 2 foreign-func, and 8 public-class rows are complete. Same-name native_add rows bind independently to Bridge.cj:6/demo.bridge and 07_foreign_ffi.cj:6/demo.ffi. Finalizer accepted the first draft with zero rejects. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

1. S37bq is production-positive for both structured carrier shapes. ArkTS's first answer used table label/cells and exact family partitions; it passed without the former delete-table/add-table contradiction. Cangjie used section item labels and exact citations; it passed without the former row-id/family oscillation.
2. Cangjie same-name identity is correct end to end: the two `native_add` rows have distinct file/package prose and distinct matching inline citations. No system supplement, row deletion, or citation rebinding changed the model's selected facts.
3. ArkTS exercised malformed-but-recoverable JSON handling: `blocks[]` arrived as a JSON-encoded string. The pre-existing flat-mode tolerance path re-parsed the full array, retained all five blocks and six citations, and required no retry. Because recovery was lossless, no degraded user disclosure was necessary; the diagnostic warning and metric remain auditable.
4. ArkTS analyzer prose initially inferred an empty repository result from a narrow pre-scan, but that provisional narrative did not become typed answer authority. The subsequent source-inventory lens found the complete third-party/corpus rows and finalization used those rows. This remains context-quality observation, not a hard-gate or answer-correctness gap in this replay.
5. Cangjie analyzer had one justified retry after inventing exclusion-policy source quotes that were not verbatim from the request. The correction removed the invalid optional policy; no contradictory contract or finalizer churn followed.
6. No Trace input was used. Explicit-window causal projection, automatic supplementation, wakeup-chain construction, on-chain root eligibility, off-chain background demotion, dual root-cause dimensions, and eliminable-amount calculations were not exercised or changed.
