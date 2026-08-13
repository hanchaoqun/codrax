# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T01:50:21Z
- sweep_start_ts: 20260812-185020
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260812-185021 | dimension_substring,answer_contains | none | 66s | 23 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Typed roster was exact (extend=1, foreign-func=1, public-class=3), but the finalizer duplicated the exact public-class Cart row into the extend bucket, omitted every requested visible file path, and overclaimed `extend` as inheritance. Zero pre-emit rejects exposed B691 exact-row multiplicity, B692 typed requested-location enforcement, and B693 declaration-kind semantic-boundary guidance gaps. |
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260812-185021 | typed_inventory_rowset,answer_contains | none | 107s | 24 | read=5,repo_map=1,list=1,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact four @Entry and two @Builder rows, paths, citations, corpus scope, and counts were all preserved. The analyzer's early broad repository guess was corrected by the typed source-inventory lens; final answer remained concise and authoritative. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B691-SOURCEINVENTORYROWMULTIPLICITY1` (P1, confirmed): complete-row coverage checked only membership-at-least-once. One exact `source_inventory_row_id` could be emitted twice across principal buckets. Root fix: count exact typed row IDs across structured principal enumeration carriers and hard-reject only the second occurrence. No title/prose/language-keyword inference participates.
- `B692-SOURCEINVENTORYREQUESTEDLOCATION1` (P1, confirmed): `requested_fields=[location]` was prompt guidance only. A citation proved a row but silently replaced the user-requested visible path. Root fix: for a valid exact row ID and typed location request, require that exact typed source path in the same item's visible text/cells; citation remains proof metadata.
- `B693-DECLARATIONKINDSEMANTICBOUNDARY1` (P1 soft guidance, confirmed): a typed declaration/construct row proves exact kind, location, and attributes, not inheritance/implementation/execution/ownership. Publish this authority boundary as soft typed guidance; do not scan or rewrite model prose.
- ArkTS supplies the positive cross-language control: the same generic source-inventory pipeline can preserve exact inventories without any language-specific answer patch.

Read/Trace/Write paths were untouched. Explicit-window Trace causal projection, deterministic auto-supplement, typed on-chain-only primary roots, actual-occupancy/business and rule-eliminability axes remain unchanged. Active byte-producing streams have no 4ms age-based degradation path; only caller cancellation/deadline, no-first-byte, true byte stall, or transport/decode failure may terminate or recover a stream.
