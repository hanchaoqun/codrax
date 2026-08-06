# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T18:12:35Z
- sweep_start_ts: 20260806-111234
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-111235 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 140s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S16 operationally worked: first native final emit, zero reject/recovery; exact selected window was preserved and the out-of-window 34579.595130 preview row did not enter the answer. The answer still selected no_causal_conclusion while ranking bounded candidates, called density 5.26 medium despite aggregate_absolute_level_authority=not_provided, added overlap/absorbed seats (about 17.609ms, 24.5ms and 43ms), called them independent, and overclaimed that all delay passed through the wakeup chain. Treat CALIBER metadata as model variance/observe, PRESSCAL1 open, and RELSYNTH1 repeated P1; do not add prose hard gates or system-authored conclusions. |
| 2 | github_issue_libgit2_foreach_worktree | PASS | eval/results/github_issue_libgit2_foreach_worktree-20260806-111235 | write_apply,write_patch_oracle | none | 356s | 20 | read=8,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Delivered bytes are correct: plan 1 fixes line 12, failed verify exposes line 16, plan 2 fixes line 16, test_repository.c is untouched, and make check passes. Internal proof state is nevertheless contradictory: plan 2 and cumulative scope retain analyzer fallback outcome-2 saying line 16 remains unchanged, while the active patch and acceptance test require changing it; controller then signs all_verified and reasons with the stale statement. Confirmed generalized typed replan-contract generation gap, not a patch-quality failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
