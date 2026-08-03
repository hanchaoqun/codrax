# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T00:08:41Z
- sweep_start_ts: 20260802-170840
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260802-170841 | log_attachment,answer_regex | log_triage | 160s | 32 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Typed `observed_log_line_order_only / cross_event_transition=unproven / typed_transition_witness=absent` reached triage, exploration and final context, and K/N domains were correct. The answer nevertheless promoted line order plus decoder definitions into a proven retry/rewrite causal chain and called it the root cause. No production caller-to-callee edge was read. The hard-required `principal_path_edge` facet conflicts with the typed no-edge authority and nudges an ordered path; register FACETAUTH1, while residual refusal to follow the explicit fence remains model variance. |
| 1 | github_issue_commons_lang_random_ascii_symptom | FAIL | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-170841 | write_apply,answer_regex | none | 313s | 19 | read=13,repo_map=3,list=2,trace=0,source_lens=2 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Workflow blocked before verification: the replan declared `src/main/java/org/apache//commons/lang3/RandomStringUtils.java`, while Git/patch-effect reported the canonical single-slash path. String-identity scope review raised `patch_effect_path_outside_plan_scope` although the file was the same. Immutable prior-plan and cumulative-scope artifacts were present, so REPLANPROOF1a worked; CAPCAL1 was not reached. Register PATHID1. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
