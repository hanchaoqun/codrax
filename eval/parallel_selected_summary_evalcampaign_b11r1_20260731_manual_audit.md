# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T19:55:56Z
- sweep_start_ts: 20260731-125555
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_libgit2_foreach_worktree_symptom | FAIL | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-125556 | write_apply,write_patch_oracle | none | 217s | 18 | read=7,repo_map=5,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Product patch and the C regression binary are correct: both precedence defects are parenthesized and the test fixes `-42`, `-7`, and `0`. The repaired Python probe invokes gcc and the resulting binary successfully, but `run_tests` skips the still-available `make check` suite before changed-path coverage is evaluated. The passed wrapper probe is then downgraded to `changed_path_verification_uncovered` for `repository.c`, so the write workflow terminates unavailable despite a matching project suite. General gap: probe-pass continuation is ordered before changed-source coverage, not a libgit2/C-case defect. |
| 2 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-125556 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 242s | 43 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Correctly identifies the surfaced synchronous binder wait as 1.409ms at 13762.835861–13762.837270, transaction 12145859, peer binder:496_9-10961; it also keeps 15.758ms pacing idle out of causal ranking and emits the full explicit-window Trace causal projection. However the model-visible typed top observation itself pairs cumulative 15.758ms with 13762.984951–13762.985960 (about 1.009ms), so the answer publishes a physically impossible duration/window pair instead of the projection member window around 13762.992415–13763.008173. Its hand-written rank table is also internally unsorted (#6 1.409ms before #7 3.429ms/#8 3.309ms) and invents a binder rank absent from the typed ranked seats; the deterministic system note catches both. Runner oracles are therefore too shallow. AA1 scope authority is present in the writer prompt and the answer does not infer CPU-wide utilization from target-thread occupancy. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
