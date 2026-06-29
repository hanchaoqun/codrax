# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T08:12:27Z
- sweep_start_ts: 20260629-161227
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-161227 | typed_inventory_rowset,dimension_substring,answer_contains | none | 183s | 22 | read=10,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Human audit: final answer still lists 2 `extend`, 2 `foreign func`, and 8 `public class` rows with package/path citations; no `ohSum` / `runOnMainThread` wrong-category leakage and no duplicate source-inventory supplement. Compared with v2, finalizer retry noise is removed (`fin_reject=0`, `patch=0`) because row-local source-inventory surface aliases now satisfy the precise member coverage gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
