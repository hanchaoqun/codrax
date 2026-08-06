# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T15:24:03Z
- sweep_start_ts: 20260806-082401
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260806-082403 | typed_inventory_rowset,answer_contains | none | 79s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correct complete inventory: four @Entry page types and two @Builder functions, each with the matching ArkTS source citation. No completion retry, finalizer reject, malformed JSON, or JSON recovery. Six exact source quote backfills were disclosed and did not change row identity. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-082403 | typed_inventory_rowset,dimension_substring,answer_contains | none | 141s | 21 | read=4,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=4,prune=0 | pass | S6 replay passed: all 2/2/8 rows and citations are correct, including `Cart.cj:30` for extend Cart; no wrong cross-row citation survived. One finalizer reject remains: model aggregate members embedded path/package detail, the system treated each full verbose string as byte-exact identity, and the otherwise correct short labels were all reported missing. Patch retry copied the verbose labels, making the answer correct but noisier. This is an identity-vs-presentation contract gap, not malformed JSON. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
