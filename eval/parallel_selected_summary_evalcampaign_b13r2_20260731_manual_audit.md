# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T23:43:36Z
- sweep_start_ts: 20260731-164334
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-164336 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 131s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer emitted the exact time window and entity `CompThread_0-2955` but omitted `runtime_targets`. Model calls used two engine-equivalent selectors, `CompThread_0-2955` and `pid=2955`; the cursor registry retained their raw spellings, so the supplement skipped `no_typed_target`. The model did distinguish 11 D-state segments/36.757ms from 12 blocked_reason records/39.157ms, but invented window-boundary overlap as the explanation. The rendered projection was consequently degraded and lacked the anchored root/wakeup/eliminable-account chain required by the exact-window case. |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-164336 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 47 | read=3,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | AI typed authority now leads: target blocking is explicitly a 1.409ms lower bound, the target state partition is exact, and 50 blocked_reason records are not a sleep partition. The later model narrative nevertheless says there is exactly one binder wait, all other requests did not block, and all 49 sleep segments are fscache-backed while also citing a 39+11=50 record roster. Full causal projection, user window, rank, wakeup chain and eliminable amount are intact, but the visible answer remains internally contradictory. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case system findings

- `EVAL-B13-AJ1/P0`: legal selector spellings that resolve to the same TID
  must canonicalize through the trace engine's one parser before cursor
  registration. Distinct positive PIDs must remain ambiguous/fail-closed.
- `EVAL-B13-AJ2/P1`: when analyzer `runtime_targets` is absent, a model cursor
  alone cannot become answer authority. A private answer-time target may be
  recovered only when a strict typed `name-pid` entity and an actually executed
  system supplement agree on the same PID.
- `EVAL-B13-A3/P1`: leading typed authority materially improves safety but
  does not remove contradictory model prose. Keep this open; do not scan raw
  answer keywords or rewrite prose as a one-case fix.
- Runner verdict 1/2; human correctness 0/2. Neither failure is accepted as
  oracle variance.
