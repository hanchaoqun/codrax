# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T22:10:26Z
- sweep_start_ts: 20260807-151024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-151026 | log_regex,answer_regex | none | 45s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact JSON-only answer `{"ids":["u1","u3"]}` remains correct. The planner consumed the new `candidate_sample_complete` fact but reasoned at task scope: because the overall task was computational it chose `instructions.md=script_consumed`, then emitted a script that read only `users.json`. The unchanged material guard correctly forced one repair. S37ay is therefore context-wired but not process-closed; B298 remains open with a per-material-vs-per-task teaching ambiguity. |
| 1 | github_issue_nlohmann_long_double | PASS | eval/results/github_issue_nlohmann_long_double-20260807-151026 | write_apply,answer_regex | none | 142s | 21 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | S37ax production-positive: ModePlan's visible action enum omitted apply/verify and the controller selected explore then plan; apply/verify appeared only in ModeApply. `workflow_action_not_in_mode` and unavailable tool attempts both fell to zero. Applied bytes still change only the two `%.*lg` sites, the test file is unchanged, and `make check` covers both production paths. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- Runner/human verdict: 2/2 PASS. No finalizer/answer-contract retry, malformed JSON recovery, or system answer replacement.
- `EVAL-B317-WRITECONTROLLERMODEAUTH1` is production-closed: dynamic enum, schema and runtime all agree across ModePlan → ModeApply transitions.
- `EVAL-B298-DATATEXTUSAGEMODE1` remains open: typed completeness reached the model, but “the task is computational” was incorrectly used as the material-mode discriminator. The next fix must clarify that usage mode is chosen independently for each material; no output or filename hard gate.
- Trace was not entered and remains unchanged.
