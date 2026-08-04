# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T15:20:11Z
- sweep_start_ts: 20260804-082009
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | FAIL | eval/results/operation_web_manual_summary-20260804-082011 | log_regex,typed_operation_terminal,answer_regex | none | 38s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Initial `steps` was a serialized command-object array containing an invalid nested JSON escape (`doc\|manual`). Flexible decoding fell back to one shell command; both lint and executor only recognized `json.Valid` containers, so the array reached `sh` and failed as command-not-found. Replan correctly refused auto-running a new local-file write and paused for approval, hence no terminal event. This is deterministic `EVAL-B84-OPSTRUCT1`, not missing manual-content coverage or model-answer variance. |
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260804-082011 | write_apply,write_patch_oracle,answer_contains | none | 124s | 19 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-line `retrun -> return` patch only; apply report, isolated worktree diff and Go verification agree. Workflow reached typed `verified`, with no answer-contract/finalizer rejection. Analyzer corrected one invalid predicate combination in its own next turn; this was not a contradictory composition contract or repeated “成文校验未通过”. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner/human: `patch_go_typo` PASS/PASS; `operation_web_manual_summary` FAIL/FAIL.
- Operation root cause is a typed-plan boundary failure: malformed serialized command-step data was executed as shell after two permissive layers both missed it. The fix must happen before execution and remain backed by an executor fail-closed guard.
- This batch observed zero finalizer/composition rejects and zero answer rewrites. It does not reproduce the earlier contradictory “成文校验” contract class.
