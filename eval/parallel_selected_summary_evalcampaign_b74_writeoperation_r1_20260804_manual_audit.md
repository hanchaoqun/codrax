# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T12:02:07Z
- sweep_start_ts: 20260804-050206
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-050207 | log_regex,answer_regex | none | 163s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Round 2 actually had a complete 20-page material ledger (`visible_runes=118802`, source/pages untruncated, coverage receipt present). The first evaluator selected the source payload path instead of the receipt, so validation correctly requested repair. Compact repair omitted the ledger/receipt and exposed only `fully_visible=false`; the model therefore downgraded the terminal to `partial_answer_possible`. The answer then says both “partial” and “full manual extracted”. Lexical answer/log regex still signed PASS: confirmed EVAL-B74-OPREPAUTH1 plus EVAL-B73-OPEVAL1. |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | TIMEOUT | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260804-050207 | write_apply,answer_regex | none | 900s | 19 | read=28,repo_map=3,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer again emitted only flat contracts (`phases=single/0`) despite the available transition schema. The first patch added a contradictory test that expected forward elements after a past-end `nth_back(usize::MAX)`. Verification did not sign it, but the fixture's own DOTALL regex also falsely rejected the valid `nth_back(0)` test because it matched the eventual `next()==None` after all remaining elements were consumed. Replan then spent 3.5 minutes waiting for the model and hit the 900s sweep ceiling. This is an over-hard eval oracle plus missing typed state-sequence adoption, not a reason to relax product verification. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
