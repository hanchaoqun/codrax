# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T09:13:56Z
- sweep_start_ts: 20260629-171356
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-171356 | typed_inventory_rowset,dimension_substring,answer_contains | none | 257s | 34 | read=17,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=0,unavail=0,prune=1 | fail | Final answer over-included Java-style public classes and missed Cangjie rows such as `demo.greeter`; logs showed analyzer emitted a precise source-inventory profile but pre-explore auto observation persisted only lens execution marker, not durable candidate row sets. Root gap tracked as D1-G166: executable lens state and candidate row-set authority were split. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
