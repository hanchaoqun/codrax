# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T16:34:41Z
- sweep_start_ts: 20260804-093440
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260804-093441 | typed_inventory_rowset,answer_contains | none | 88s | 20 | read=0,repo_map=2,list=1,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Product result is exact: 4 `@Entry` rows and 2 `@Builder` rows, each with the correct path:line; `EntryAbility` and the former 20-row coarse-role expansion are absent. Finalizer accepted first pass. The recorded FAIL is an eval false negative: the correct ordered-list answer used no category heading. The follow-up typed row-marker oracle replays this artifact as PASS while still rejecting the B86 five-row artifact. |
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260804-093441 | dimension_substring,answer_contains | none | 120s | 19 | read=3,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact five-row inventory remains intact: one extend block, one foreign func and three public classes, with package and file:line; the two `Cart` identities remain distinct. Five citations are present, with zero finalizer rejects/advisories and no false follow-up caveat. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
