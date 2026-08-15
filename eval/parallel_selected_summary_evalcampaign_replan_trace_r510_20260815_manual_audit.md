# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T14:10:59Z
- sweep_start_ts: 20260815-071057
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-071059 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail-system | B833 is production-positive for its exact purpose: the concrete `worker-200 CPU2 -> app-100 target CPU1` tuple reached the pre-generation final boundary, and the answer stopped claiming worker occupied CPU1 or caused same-CPU contention. The answer still called VerifyClass a synchronous RPC block, said the target waited for its completion, and said it had just completed at the 5.005000 wakeup even though the span ended at 5.005400. Earlier perf/analyzer/explorer model summaries carried these unsupported mechanisms into final context despite the later typed boundaries. B835 must separate model-authored investigation hypotheses from typed evidence authority; no prose hard gate or system answer rewrite. |
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-071059 | write_apply,write_patch_oracle | none | 948s | 25 | read=25,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-system | The initial patch was correct. TestSurface recorded the canonical selected command `python3 -m unittest discover -v`, but the root-level impacted test path became suite=`.` and the executor rebuilt it as `python3 -m unittest "." -v`, which fails with `ValueError: Empty module name`. Planner probes then ran all five tests successfully, but the ordinary replan was not authorized for `changes: []`; the model spent 25 reads and multiple generations inventing no-op/duplicate/doc-only edits before terminal block. B834 fixes the deterministic root unittest selector so correct code reaches the real suite without entering this impossible replan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B833 exact topology handoff works in production. The final prompt contained the exact cross-CPU tuple and explicit authority limit, the main answer no longer invented same-CPU occupancy, and explicit-window Trace causal projection plus automatic supplementation survived unchanged.
2. Trace remains system-fail because upstream model-authored summaries are still presented as if they can explain mechanism. Perf triage called the semantic span a synchronous RPC stall; analyzer and explorer repeatedly said it blocked the target or completed before wakeup. The final typed boundary contradicted those summaries, but forcing the final model to arbitrate contradictory context is avoidable system load. B835 will make typed rows authoritative and upstream summaries explicitly hypothesis-only/replaceable; it will not scan or rewrite final prose.
3. The write failure is a deterministic TestSurface command-construction bug, not a code/test failure. For a root-level unittest file, impact scoping emits suite=`.`; Python unittest interprets `.` as an empty module name. The surface itself already held the correct root command (`discover -v`), proving selection was correct and execution rendering drifted.
4. B834 maps the typed root selectors empty/`.`/`./`/`.\\` to canonical root discovery while retaining filesystem-verified `discover -s <subdir>` for subdirectories and ordinary module selectors elsewhere. A production-shape test uses an applied ChangePlan touching root source+test files, runs the auto-selected impact lane and pins the executed command plus passing report.
5. Fixing the first deterministic verifier prevents the subsequent impossible replan; it does not accept no-op patches, relax tests, or let a planner observation impersonate project-suite authority. The empty-change sentinel remains fail-closed unless its typed authorization is genuinely present.
6. Both streams remained active well beyond 4ms/4s/4m without fixed-age degradation. Trace completed normally at 187s; write terminated from workflow exhaustion at 948s, not from a short answer-age fallback.
