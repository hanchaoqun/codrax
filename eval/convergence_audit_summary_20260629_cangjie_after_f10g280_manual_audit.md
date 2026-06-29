# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T08:21:35Z
- sweep_start_ts: 20260629-162135
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-162135 | typed_inventory_rowset,dimension_substring,answer_contains | none | 266s | 27 | read=17,repo_map=0,list=3,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Final answer was partially correct but missed `demo.greeter` and several typed public-class rows. Root cause is not finalizer citation repair: analyzer emitted a source-inventory profile, but support-only list/read observations suppressed the executable repo_map source_inventory lens, so no typed lens authority ran. Tracked/fixed as D1-G163. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
