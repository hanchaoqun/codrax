# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T12:51:47Z
- sweep_start_ts: 20260815-055146
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-055147 | write_apply,write_patch_oracle | none | 144s | 25 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct one-file normalization; all 8 tests pass. No replan occurred, so B828 had no production positive trigger and remains replay-required. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-055147 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail-system | Explicit window/projection survived, but non-target VerifyClass chain-interval overlap still minted effective=4.600ms and crown despite typed direct wait/completion proof being absent. B829 fixed only the host-edge lane; B830 must close the exact-intersection bypass. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. The write case is behaviorally correct: the applied tree contains one normalization path, integer-valued floats work, non-integral floats remain rejected, and the complete test suite reports 8 passes. Because the model finished in one apply/verify path, this run cannot validate B828's replan-current-worktree receipt in production.
2. The trace case is a deterministic system failure, not model fluctuation. The final answer says `VerifyClass` is the direct blocker and crowns 4.600ms, while its own caveat says no typed holder/waiter or direct blocking relation was established. The wakeup occurs at 5.005000s and the span ends at 5.005400s, so completion cannot have triggered that wakeup.
3. B829's production bypass is the earlier exact same-thread chain-window intersection lane. It treated a non-target semantic interval overlap as positive causal attribution even though the target's typed runnable delay (0.800ms) was the only positive scheduler seat in this fixture.
4. The generalized B830 correction is typed and two-dimensional: target-thread deterministic semantic work may price its self interval; non-target `semantic_chain_interval_relation` or `host_wakeup_edge_pre_span` retains exact raw occupancy, semantic/business identity, roster, and optimization guidance but has zero effective/eliminable impact until an exact typed target-wait or semantic-completion binding exists. Scheduler, priority, compute-supply, D-state, and IO seats are unchanged.
5. Both streams delivered complete answers. No 4ms/4s/4m fixed-age fallback, stale-draft recovery, empty answer, or system-authored conclusion was observed.
