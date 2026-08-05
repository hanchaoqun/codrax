# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T23:10:14Z
- sweep_start_ts: 20260805-161013
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260805-161014 | typed_inventory_rowset,dimension_substring,answer_contains | none | 141s | 20 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Typed inventory and Explorer both found `extend Cart` at `Cart.cj:30`, but the finalizer handoff flattened all 12 rows into one generic set without `surface_family`. A title-derived hard scope then rejected Cart:30 from the public-class block with an ambiguous remove/move hint; the model removed it instead of moving it to extend. This is a generic typed-partition-loss plus model-title hard-gate gap, not a Cangjie parser miss. |
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260805-161014 | typed_inventory_rowset,answer_contains | none | 168s | 21 | read=5,repo_map=2,list=1,trace=0,source_lens=2 | midloop=3,inv=2/1,fin_reject=1,unavail=0,prune=0 | fail | Core roster was correct: four `@Entry` and two `@Builder` symbols with citations. The model declared two columns (file path/function name) but emitted only `label`, so rendering silently collapsed the missing file dimension. Production now rejects this carrier shape and teaches the two canonical row conventions. Runner also had a false `@Component` substring oracle although the question requests `@Entry` + `@Builder`; the oracle is corrected independently. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- `EVAL-B142-TABLEROWSHAPE1` (P1, implemented): the JSON/schema/checklist wording described `columns[]`, `label`, `text`, and `cells[]` differently enough that a syntactically valid but visually incomplete row could pass. Full emit and patch now share a pure structural row-width check. It reads no user/model prose and fills no missing answer value.
- `EVAL-B142-CJFAMILYCARRY1` (P1, implemented after this frozen replay): source inventory already owned an exact row-local family, but aggregate projection/finalizer handoff lost it and later reconstructed scope from a model-authored block title. Exact typed partitions and row family are now preserved; shipping hard validation uses only the optional typed family carrier or the global roster, never title prose.
- `EVAL-B142-ARKTSORACLE1` (eval-only, fixed): `EXPECT_CONTAINS` asked for `@Component` even though the user contract asks for `@Builder`.
- JSON carriers in both finalizer runs were parseable. The defects were carrier semantics and typed-context loss, so malformed JSON salvage is intentionally not used as a substitute.
