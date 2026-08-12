# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T05:23:15Z
- sweep_start_ts: 20260811-222313
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_trace_current_source_explanation | FAIL | eval/results/read_combo_trace_current_source_explanation-20260811-222315 | trace_attachment,answer_regex | perf_triage+trace_query | 144s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B608 aggregate-value omission alone was insufficient: Analyzer explicitly set current_source_explanation_profile=false despite the current request saying “结合当前源码”, so B609 never ran and no source evidence was collected. Final answer repeats unsupported 60/90fps budgets and 5.2x/7.8x ratios although typed refresh/deadline authority is absent. It also repeats the wrong payload-PID/adjacent-E sync-pairing account. Confirm B608 finalizer authority wiring gap for janky=false/omitted and B610 analyzer soft-carrier omission. |
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-222315 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 174s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Structural oracle passed and all trace authority surfaces remained present, but model regressed relative to r363: it summed CookieMonsterCl 23.994ms + NetworkService 19.041ms into 43.035ms despite typed non-additivity, and upgraded priority-inversion candidates into “真实瓶颈/调度阻塞”. Treat as model variance because the precise context already carries the contrary contract; do not add answer-prose scans, hard rejection, or system-written conclusion. Explicit window, wakeup chain, on-chain/background split, 10.331ms positive compute-supply secondary, dual occupancy/eliminable axes, VerifyClass clue, causal projection and supplement remain intact. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
