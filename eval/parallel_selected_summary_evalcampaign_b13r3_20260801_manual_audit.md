# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T00:01:25Z
- sweep_start_ts: 20260731-170123
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-170125 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 119s | 43 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Full exact-window projection returned and the supplement executed `root_cause_rank`; the tree carries the 36.757ms D-state family, wakeup chain, ranked causes and eliminable account. Human principal answer is still wrong/ambiguous: the engine payload has 11 exact D-state occurrences summing to 36.757ms, while 12/39.157 is a separate blocked_reason record/delay census. The complete occurrence authority failed to join because projection label `attached_trace.txt` and occurrence label `attached_trace` differed. Model prose consequently answered with 12 and invented a window-tail explanation. The state account also covers only 231.794ms of the 233.190ms window without disclosing the 1.396ms unaccounted interval. |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-170125 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 134s | 46 | read=2,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Full trace projection and all typed values remain intact. Leading authority still says binder blocking is only a 1.409ms lower bound and blocked_reason records are not a sleep partition; later model prose nevertheless publishes exactly one/only one binder wait and treats the 50-record caller census as a complete attribution. This remains the filed model/system conclusion-conflict gap, not oracle variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case system findings

- `EVAL-B13-AJ1/AJ2`: exact-window causal projection is restored in r3 and no
  `no_typed_target` skip occurred. This replay's analyzer emitted a valid user
  runtime target, so it validates non-regression but does not independently
  exercise the missing-target selector-consensus arm.
- `EVAL-B13-AK1/P0`: occurrence authority and projection used two display
  labels for one typed SourceRef (path basename versus lane ArtifactID), so 11
  complete rows were silently unavailable at the principal card.
- `EVAL-B13-AK2/P1`: target state coverage is partial (231.794/233.190ms) but
  the principal card lacked `unaccounted=1.396ms` and typed coverage status.
- `EVAL-B13-AK3/P2`: the causal tree calls four per-CPU aggregate buckets
  `4次`, while the true occurrence roster has 11 intervals. The old runner
  oracle intentionally pins that legacy word, so runner PASS is not human
  correctness for the user's “发生几次” dimension.
- `EVAL-B13-A3/P1`: H1 still contains model prose that conflicts with leading
  typed authority. Keep open without raw-answer keyword gates.
- Runner verdict 2/2; human correctness 0/2.
