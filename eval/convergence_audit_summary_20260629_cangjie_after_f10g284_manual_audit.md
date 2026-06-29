# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T08:44:56Z
- sweep_start_ts: 20260629-164456
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-164457 | typed_inventory_rowset,dimension_substring,answer_contains | none | 143s | 21 | read=3,repo_map=4,list=0,trace=0,source_lens=4 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | First draft had the right high-level row set, but deterministic presentation/normalization damaged the final answer: `extend Cart` was no longer visible in the extend section and only a system supplement for `extend String` remained. Root cause was row identity/presentation authority split across source-inventory observation, enumeration display rows, and principal enum pruning. Tracked/fixed as D1-G165. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
