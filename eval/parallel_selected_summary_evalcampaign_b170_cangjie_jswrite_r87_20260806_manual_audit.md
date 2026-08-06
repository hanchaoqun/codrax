# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T08:07:03Z
- sweep_start_ts: 20260806-010702
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dayjs_duration_nan | FAIL | eval/results/github_issue_dayjs_duration_nan-20260806-010704 | write_apply,answer_regex | none | 174s | 20 | read=4,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | The patch is the intended generic fix, `Number(value ?? 0)`, and the static Make checker passed. B170-S2 also correctly queued the independent Node behavior surface and stopped claiming verification when `npm` was absent on this host. Code review supports the patch, but no JavaScript runtime executed, so the run remains honestly unverified rather than a human PASS. |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-010704 | typed_inventory_rowset,dimension_substring,answer_contains | none | 450s | 26 | read=8,repo_map=2,list=1,trace=0,source_lens=2 | midloop=5,inv=2/0,fin_reject=4,unavail=0,prune=0 | pass | The dedicated source-inventory lens produced the exact 2 extend / 2 foreign-func / 8 public-class roster with correct package declarations, and the final answer preserved all rows. Runner failure was false: its list cardinality counter counted the trailing explanatory note under the public-class section as a ninth member. Production still had a severe compound-carrier contradiction: the strict typed type roster rejected two separately accepted grounded foreign-function principal rows, causing four patch attempts before acceptance. Whole-blocks string recovery occurred once but was lossless; there was no missing answer or degraded salvage. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
