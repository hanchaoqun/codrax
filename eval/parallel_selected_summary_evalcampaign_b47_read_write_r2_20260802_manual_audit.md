# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T22:52:13Z
- sweep_start_ts: 20260802-155212
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_dimensions | PASS | eval/results/read_combo_log_current_code_dimensions-20260802-155213 | log_attachment,answer_regex | log_triage | 159s | 33 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | typed protocol rows correctly identify `4/4` as `pipeline_stage_progress` and `1/3` as `agent_dispatch_attempt`, but Log Triager still says “4/4 models failed / 4 complete retries”. Explorer and Finalizer preserve the namespace split yet upgrade stage position into retry count, exhaustion and causal transition. The compact final ledger prioritizes the model aggregate while dropping the system protocol rows from the visible top 10. Runner regex is therefore a false green. |
| 2 | github_issue_commons_lang_random_ascii_symptom | PASS | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-155213 | write_apply,answer_regex | none | 312s | 19 | read=11,repo_map=4,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Patch is correct and final `make check` succeeds. `declared_project_check` now covers the final plan's exact Java source path, so XCOV1 works. However the previous plan's still-applied Java test-file edit is absent from the final plan/report coverage closure. Final proof says strong/verified from a Go source-string probe plus a Python source-string project check; neither executes Java behavior, and no behavior-contract obligations survive into the final proof ledger. Delivery authority is overstated despite the useful patch. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decisions

- `EVAL-B47-SEMCAL1/P1`: operational rows need explicit value semantics (`stage_ordinal`, one-based stage position, total configured stages) and explicit exclusions for retry/attempt/failure/budget/exhaustion meanings. Producer rows must be reserved in the compact principal ledger ahead of conflicting model aggregates; the fix may requalify evidence authority but must not scan or rewrite final prose.
- `EVAL-B47-REPLANPROOF1/P1`: post-replan verification must close over the cumulative still-applied path and behavior-contract set, not only the latest repair plan. A successful exact declared roster can cover cumulative members, but a failed prior report cannot disappear merely because the repair plan edits one path.
- `EVAL-B47-CAPCAL1/P1`: source-token/static checks prove structural/source conditions, not target-language runtime behavior. Preserve their changed-path/check authority, but proof profile/final wording must expose capability caliber and must not call behavior strong/verified when no behavior-contract execution exists.
- No Trace input or Trace code path was touched. Explicit-window causal projection, rank/wakeup/eliminable surfaces and system supplement remain outside this replay and unchanged.
