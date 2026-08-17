# r598 Cangjie / write production replay manual audit

- date: 2026-08-17T00:32:05Z
- sweep_start_ts: 20260816-173203
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | runner | human | result_dir | sec | ctx% | tools/churn | audit conclusion |
|--:|------|--------|-------|------------|----:|-----:|-------------|------------------|
| 1 | `cangjie_repomap` | PASS | pass | `eval/results/cangjie_repomap-20260816-173205` | 309s | 27 | read=8, repo_map=1, source_lens=1; investigation rejects=1; finalizer rejects=2 | Analyzer emitted `has_per_member_table=true`, so this run exercises B947. The final answer has exactly one principal member carrier: a 12-row table with two `extend`, two `foreign func`, and eight public-class-family rows, each with category, package, path, line and description. Optional sections did not duplicate the roster. The two finalizer rejects were precise row-identity enforcement: the model initially omitted member labels and then omitted two ambiguous `source_inventory_row_id` values. Existing teaching already requested both; the accepted answer preserved all facts without system-authored members or prose rewriting. |
| 2 | `github_issue_zod_prefault` | FAIL (`unverified`) | partial / correct patch, honest proof boundary | `eval/results/github_issue_zod_prefault-20260816-173205` | 178s | 25 | read=5, repo_map=2; no replan/retry loop | The applied commit changes the production predicate to `"_prefault" in result.schema`, preserves `default ??=`, and adds false/0/empty-string regression tests while retaining existing truthy/output/default-retention coverage. `make check` passed, but the fixture implements it with `tests/check_prefault_schema.py`, so both changed TypeScript paths are typed `capability=source_static`. The controller model chose `all_verified` despite the exact boundary in its context; deterministic completion correctly normalized this to `production_verification_source_static_only`. This is not a code failure and must not be made green by lowering the behavior-proof bar. |

## Decisions

- B947 is production-closed: the Cangjie run activated `HasPerMemberTable=true` and emitted one exact table with no section/list duplicate. The implementation is language-neutral typed guidance and therefore covers ArkTS/Cangjie/Go/Java/C/C++/Rust/Python without language-name gates.
- The Cangjie row-label/row-ID retries are model adherence cost, not a contradictory contract: the prompt and typed source inventory consistently require visible identity and exact IDs when candidates are ambiguous. Do not add a case-name, member-name, or answer-text hard fit.
- The write patch is semantically correct by source review, but only statically verified in this minimal fixture. The visible `unverified` result is the intended fail-closed contract. The model's `all_verified` choice is bounded by the deterministic normalizer and did not cause repeated verification or unsafe delivery, so no new code change is justified from this single witness.
- Both calls continued while bytes were active and completed normally. No 4ms/4s/fixed-total-age degradation, empty answer, or system-authored replacement occurred. Read/Trace code paths were unchanged.
