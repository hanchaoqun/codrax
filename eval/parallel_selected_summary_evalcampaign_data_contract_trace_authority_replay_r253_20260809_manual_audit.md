# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T11:19:09Z
- sweep_start_ts: 20260809-041907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-041909 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000..2.020 window and three typed trace calls preserved. Root #1 is the on-chain threadpool-400 iowait 11ms; logger-900 remains background/off-chain and is excluded from projection rank/effective attribution. Context wording still calls the background row's 7ms “有效归因”, conflicting with the projection's typed `—`; filed as EVAL-B434. |
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260809-041909 | log_regex,answer_regex | none | 204s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-answer/fail-process | Final answer is exactly 17. B430 is production-effective (first compute has action-local input_paths); B431 is production-effective (resolutions=0). Eight rounds, three repair rounds and three action failures remain: candidate coverage flags retroactively narrow their own rank (B432), then assemble_answer uses order_by=reference without typed reference authority (B433). |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `EVAL-B430-ACTIONINPUTCARRIER1` is production-closed: the first rejected compute plan already carries
   `input_paths=["orders_records"]`; no `missing_action_inputs` repair loop remains.
2. `EVAL-B431-RELATIONLINEAGEOVERLAP1` is production-closed: all data results report `resolutions=0`; no same-lineage
   normalize/entities detour is emitted.
3. `EVAL-B432-CANDIDATERANKDRIFT1=P0/REDLINE`: the pre-plan reducer publishes
   `decision_next_actions=compute_contributions`, and the narrowed tool schema admits that kind. The candidate then adds
   `rule_coverage_required=true` and `decision_records_required=true`; staging recomputes the action rank from this uncommitted
   candidate and rejects the same compute as `action_outside_allowed_next_stage`. This is a typed transaction-order conflict,
   not model prose or JSON syntax noise.
4. `EVAL-B433-ASSEMBLEREFERENCEDEPENDENCY1=P1`: after reconcile passes, the candidate emits
   `assemble_answer(order_by=reference)` while both `output_contract.complete_reference=false` and action-local
   `reference_path/reference_key_field` are absent. The runner correctly fails, but the JSON action contract permits the exact
   impossible combination to leave the model. The schema should express the executor-owned dependency; the system must not
   silently change `reference` to another ordering.
5. Trace answer authority is correct: the on-chain wakeup path is
   `threadpool-400 -> network-300 -> cookie-200 -> app-100`; only its typed seats are ranked. The final projection renders both
   logger-900 rows under background and shows effective attribution `—`.
6. `EVAL-B434-BACKGROUNDEFFECTIVEWORDING1=P1`: finalizer prose nevertheless says logger-900 has “有效归因 7.000ms”. The
   observation remains supporting/background, but its generic value plus an `effective_impact_ms` note gives the model a
   contradictory caliber. Fix the typed context at its producer; do not scan or rewrite final prose.
7. No system answer rewriting occurred, and no finalizer rejection occurred. Explicit-window Trace causal projection and auto
   supplementation remain intact. Adjacent/background rows remain support or additional-investigation directions only; they do
   not acquire a root-cause seat.
