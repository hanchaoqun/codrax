# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T20:57:38Z
- sweep_start_ts: 20260818-135737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260818-135738 | write_apply,answer_regex | none | 149s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Patch and six falsy/default tests are correct; `make check` passes, but changed production TypeScript has typed capability `source_static`. The controller shows that boundary, normalizes the model's `all_verified` to `accept_unverified`, and does not schedule a mandatory JS probe because this host has no Node runtime. This is an honest environmental verification limit, not a new routing gap; do not wrap it in another language or lower the proof bar. |
| 1 | trace_query_wakeup_causal_io_chain | FAIL | eval/results/trace_query_wakeup_causal_io_chain-20260818-135738 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 257s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Substantively correct: explicit 20ms window, on-chain threadpool IO #1=11ms, full threadpool→network→cookie→app path, three disjoint 1ms scheduler-supply rows, raw-occupancy/priced axes, causal projection, and background demotion all survive. Runner fails only because its last regex requires the extra word “中间/传递/path” near network/cookie even though the answer already has a “相关链路” section and exact ordered path; this is an overfit oracle, not an answer gap. Model prose has one timing fusion (`irq` wake at 2.016; typed evidence says irq→threadpool at 2.014 then threadpool→network at 2.016) despite precise context; leave as model fluctuation, do not rewrite prose. Production revealed omitted target_scope vs explicit thread causing all four second-lane views to miss memo; fixed by B1108b typed-default normalization. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- Runner: 0/2 PASS. Human: 2 partial; neither failure justifies weakening a production gate.
- Trace oracle debt: replace the wording-coupled last regex with a structural ordered-path check in a later eval-harness batch; do not require the model to emit one preferred synonym.
- B1108b production witness: second explorer used the same attachment/view/target/window but added only `target_scope=thread`; pre-fix it rebuilt window/wakeup/rank/blocking. Empty scope is the validated thread default, while process remains distinct.
- Active stream had no fixed-4ms degradation. No finalizer rejection, malformed JSON recovery, system-authored conclusion, or missing Trace projection occurred.
