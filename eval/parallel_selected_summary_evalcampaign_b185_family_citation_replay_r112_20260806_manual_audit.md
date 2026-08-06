# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T17:34:41Z
- sweep_start_ts: 20260806-103439
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260806-103441 | typed_inventory_rowset,answer_contains | none | 125s | 22 | read=5,repo_map=3,list=0,trace=0,source_lens=3 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact 4 @Entry + 2 @Builder rows, no extra member, all six citations aligned. Explicit-section-first oracle now counts the whole section rather than the one row repeating @Entry. One fully lossless blocks-string recovery, no finalizer reject or visible degradation. Explorer needed 8 iterations to re-anchor struct/surface terms; efficiency observation only. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-103441 | typed_inventory_rowset,dimension_substring,answer_contains | none | 145s | 20 | read=8,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact 2 extend + 2 foreign func + 8 public class rows and packages. One native emit, no patch/reject/recovery. extend Cart remains at Cart.cj:30, public class Cart at :14, and both native_add rows cite their own declaration; no cross-family rebind, detach, or false degradation note. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
