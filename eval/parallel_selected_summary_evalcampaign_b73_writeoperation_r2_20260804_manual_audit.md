# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T09:07:21Z
- sweep_start_ts: 20260804-020720
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-020722 | log_regex,answer_regex | none | 128s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Runner false positive. The operation fetched only the landing-page artifact (14606 bytes, ending in CSS), guessed `/docs` and `/doc`, then stopped after five rounds without fetching the typed `user_guide.html` target or obtaining a complete manual coverage receipt. The final answer honestly says partial completion, yet the lexical oracle passes. B72 completed the same task on the same product contract, so the retrieval miss is retained as model variance; the missing typed terminal/coverage oracle is an eval-quality gap. |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260804-020722 | write_apply,answer_regex | none | 328s | 20 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The analyzer again emitted only flat behavior contracts; no `transition.steps[]` reached the planner. Tests cover fresh iterators only, and the final `nth_back` implementation still ignores the already-advanced front cursor (`next(); nth_back(2)` can return an already-consumed element). Verification was also preempted by a Go inline probe explicitly bound to `path:src/types/list.rs`; its Go compile error was mislabeled as the changed Rust build failing while typed `make@.` remained untried. Final unverified status was conservative, but its stated reason was imprecise. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
