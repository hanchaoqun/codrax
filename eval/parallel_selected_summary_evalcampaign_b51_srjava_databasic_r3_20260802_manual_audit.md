# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T03:25:28Z
- sweep_start_ts: 20260802-202526
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-202528 | primary_answer | none | 93s | 19 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer has the correct four-layer ordered path, correct `visit.insert`/`petId` call, and correctly locates the only capacity guard in `VisitService.schedule`. No system-added 3/14-row rosters and no reject loop. Cold-reading the prompt still found R1 category amplification because this analyzer run used relational=false; B51-C2 broadens the typed guard across that legal classifier variation. |
| 2 | data_basic_sum_with_rules | FAIL | eval/results/data_basic_sum_with_rules-20260802-202528 | log_regex,answer_regex | none | 350s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Computation reached reconcile expected=17/actual=17/pass, but terminal validation correctly failed closed because source-backed rules were not linked to any decision/contribution/entity record by rule_id. Recovery then spent 10 data rounds/6 repairs and attempted a restricted custom_transform using unavailable Python builtin `type`, ending with no answer. Logged separately as EVAL-B51-DATAREF1; do not weaken provenance validation to pass it. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
