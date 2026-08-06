# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T17:18:18Z
- sweep_start_ts: 20260806-101817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260806-101818 | typed_inventory_rowset,answer_contains | none | 163s | 21 | read=6,repo_map=2,list=1,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer visibly and correctly lists all 4 @Entry rows and 2 @Builder rows with matching paths/citations, no finalizer reject or malformed JSON. Runner false-red: marker-row fallback sees the literal @Entry only on Index and overrides the case's more precise explicit section label, so it reports got1/want4. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-101818 | typed_inventory_rowset,dimension_substring,answer_contains | none | 228s | 24 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Visible row set and package/path facts are correct 2/2/8, with family headings attached. But generic candidate-role binding rewrote exact extend Cart@30 to public class Cart@14 and native_add@Bridge:6 to Bridge class@15, then detached both item citations; answer emits a misleading two-citation-removal note while the appendix still contains the correct sources. One blocks-string recovery plus one metadata/member-label patch. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
