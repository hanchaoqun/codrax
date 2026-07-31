# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T21:30:25Z
- sweep_start_ts: 20260731-143024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-143025 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 228s | 42 | read=1,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | AC1/AC2 replay verified: independent on_chain/adjacent complete rosters, correct on_chain #1..#4, and binder 1.409ms owns 13762.835861..13762.837270. New generalized caliber failure: prose sums four synchronous IPC send/reply latencies as 1.558ms of target blocking even though typed critical_blocking proves one 1.409ms target-owned wait; it also uses zero D-state to deny binder-caused sleep although the proven binder wait is an interruptible S-state interval. Enumeration remains incomplete while prose says all/only. |
| 1 | github_issue_libgit2_foreach_worktree_symptom | PASS | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-143025 | write_apply,write_patch_oracle | none | 327s | 18 | read=12,repo_map=4,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | AC3 replay verified: selected real `make@.` / `make check`. First plan fixed only the visit branch and incorrectly called the isomorphic lookup branch correct; project verification exposed `got 1, want -7`, durable replan fixed the second branch, and the final applied tree plus project suite are correct. Treat the initial inconsistency as model reasoning variance contained by verification, not a new hard-gate target. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner: 2/2; human: 1/2.
- AC1, AC2, and AC3 are verified on their original witnesses. Explicit-window causal projection, root ranking, wakeup chain, target four-state accounting, eliminable amounts, and deterministic supplementation remained present.
- Highest-ROI next batch is a typed target blocking-wall-clock authority: union only target-owned critical-blocking occurrence intervals, keep IPC transport latency separate, and publish lower-bound status when the source rowset is truncated. This is generic across Binder, futex, locks, D-state, and other blocking types; it does not inspect request or answer prose.
