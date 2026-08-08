# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T09:25:35Z
- sweep_start_ts: 20260808-022534
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260808-022535 | typed_inventory_rowset,answer_contains | none | 120s | 21 | read=0,repo_map=2,list=1,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Exact @Entry rows were present in the first table, but the strict validator rejected table-cell identities; a conflicting table-required contract then forced duplicate list+table output and the typed oracle still missed four Entry rows. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260808-022535 | typed_inventory_rowset,dimension_substring,answer_contains | none | 202s | 22 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=7,unavail=0,prune=0 | partial | The visible 2+2+8 roster is complete, but seven deterministic rejects alternated between refusing prompt-issued foreign-func row IDs and refusing that exact family. The model finally removed row IDs; the second same-name native_add row then displayed the first file's inline citation, contradicting its own location text. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

1. This replay did not close the ArkTS gap. S37bp repaired role normalization for label carriers, but the production first draft used the schema-supported table-cell carrier. The prompt required the four `@Entry` rows, while the hard source-inventory registry did not admit those exact sibling-role rows, so the system first ordered deletion and then separately required a table.
2. The Cangjie runner PASS hides a serious process and citation defect. `Principal Enumeration Rows` printed two distinct `foreign func` row IDs, yet row-id validation rejected both because its admitted family registry contained only type-derived families. Adding `source_inventory_family=foreign func` was then rejected because that same family was absent from the validator registry. This loop consumed seven finalizer attempts.
3. After the model removed both exact row IDs, the final answer retained both textual locations but bound the second `native_add` display row to the first `Bridge.cj:6` citation. The runner's membership oracle did not catch that cross-file citation contradiction; human correctness is therefore partial, not a clean PASS.
4. The generalized root cause is one authority split, not ArkTS/Cangjie parser quality: the finalizer-visible principal display sets and the pre-emit source-inventory row registry used different admission domains for exact repo-lens rows in sibling coarse roles.
5. No Trace input was used. Trace explicit-window projection, automatic supplementation, wakeup chain, on-chain root eligibility, off-chain background demotion, and eliminable-amount computation were not exercised or changed.
