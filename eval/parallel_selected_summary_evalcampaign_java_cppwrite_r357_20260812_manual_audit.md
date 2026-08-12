# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T03:38:36Z
- sweep_start_ts: 20260811-203834
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_annotation_route | PASS | eval/results/sr_java_annotation_route-20260811-203836 | answer_regex,answer_contains | none | 113s | 23 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Source truth and the answer's own step 2 say `StatsHandler` carries `@Route(path="/stats")`, but the lead says it has no direct `@Route`. Five model-submitted call rows were normalized/demoted to text references with exact parser repair notes; because this was a non-diagram `predicate_axis=register` request, the old repair contract did not make those exact rows durable or completion-blocking. Final context reported `grounded_callsite_facts=0`, `explicit_typed_directed_relations=0`, and `ordered_path_authority=unproven`; a soft missing-relation caveat was appended after the answer instead of reopening evidence. One ordered-list citation was also bound to the unrelated `Router.java:45` guard. This is B599, not a Java-only/model-only failure. |
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260811-203836 | write_apply,answer_regex | none | 121s | 22 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exactly two intended source surfaces changed `%.*lg` to `%.*Lf`; the project-declared `make check` compiled with `-Wall -Wextra -Wformat -Werror`, executed the test binary, and exited 0. Cumulative write verification remained populated and final status was passed. The analysis sentence that associated `%lg` with `double*` is imprecise wording, but it did not alter the correct patch or verification and is not promoted into a new system gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch disposition

- `B599-NONDIAGRAMCALLRELATIONREPAIR1/P1-high`: confirmed. Exact call endpoint repair was incorrectly owned by required flow/call diagrams, even though typed registration/configuration/call/flow mechanism answers consume the same directed source relations without asking for a diagram.
- The generalized repair selector now consumes only schema-validated `PredicateAxis` / `RequirementKind`, plus a model-submitted line-scoped `AnchorCall` and one unique parser-owned caller→callee tuple. It does not inspect request prose, evidence summaries, or final answer text and never creates an edge. Wrong rows remain citable text until the model re-emits the exact relationship; durable repair debt survives unrelated successful evidence batches.
- `condition/implement/define/return` axes remain non-blocking for incidental calls. Runtime Trace and `QFRootCauseTrace` remain isolated, so explicit windows, causal projection, auto-supplement, on-chain root ranking, and off-chain background authority are unchanged.
- `B600-CROSSLANGUAGEDECORATORAPPLICATION1/P1-follow-up`: the same replay shows that an adjacent declaration annotation can remain only a soft `surface_terms` suggestion. Python has a typed decorator-application carrier, while Java custom annotations and the other supported language annotation/decorator forms do not yet share one closed application relation. Audit and implement this as a cross-language provider batch; do not add a `@Route`-specific final-answer rule.
- No malformed-JSON recovery, finalizer retry, elapsed-time recovery, or system-authored answer occurred in either case. Both cases were below four minutes; B560's 351s/328s production witnesses and typed progress tests continue to prohibit fixed-age degradation once real model progress exists.
