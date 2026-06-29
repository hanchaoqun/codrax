# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T07:58:55Z
- sweep_start_ts: 20260629-155855
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-155855 | typed_inventory_rowset,dimension_substring,answer_contains | none | 160s | 21 | read=8,repo_map=3,list=0,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | Human audit: final answer correctly lists 2 `extend` rows, 2 `foreign func` rows, and 8 `public class` rows with package/path citations; no `ohSum` / `runOnMainThread` wrong-category leakage and no duplicate source-inventory supplement. Remaining `finalizer_rejects=2` are local answer-form churn, tracked separately as D1-G162. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
