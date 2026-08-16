# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T22:34:14Z
- sweep_start_ts: 20260816-153412
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-153414 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 185s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | Explicit 13762.791708..13763.024898 window and Trace causal projection survived. The answer keeps typed on-chain ranking separate from adjacent/background pressure, preserves running 74.915ms versus effective 65.912ms, D-state 36.757ms/11 waits versus blocked_reason 12 rows and sum 39.157ms, runnable 1.536ms, exact io_wait zero, inversion candidates, business-span clues, and the occupancy-versus-eliminable split. One first-attempt answer_document was accepted; no prose replacement, malformed JSON, retry fallback, or fixed-age stream degradation. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260816-153414 | typed_inventory_rowset,dimension_substring,answer_contains | none | 361s | 33 | read=8,repo_map=2,list=1,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | All 12 typed rows are visible exactly once in three member-preserving groups: extend=2, foreign func=2, public class=8. Every row retains its member, category/group, exact file:line, package, and row-local citation, including duplicate native_add identities and the two Cart rows. The first answer_document was accepted with no patch or degraded fallback. The 361s elapsed time is exploration work (8 reads, 2 repo_map, 2 source_lens), not final-contract retry churn. B942 is production-positive. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
