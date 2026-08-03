# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T03:44:53Z
- sweep_start_ts: 20260802-204450
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260802-204453 | answer_contains | none | 145s | 20 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Correct two-member production caller set with exact call-site citations, no tests and no reject. The true relation-enumeration lane remains covered after C3. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-204453 | primary_answer | none | 365s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=7,inv=1/0,fin_reject=26,unavail=1,prune=0 | fail | C3 authority target is covered: explorer prompt has no Structured Aggregate/Source Operation Site/Attribute-bearing Enumeration sections, and final product has no system materialized member roster. Remaining failure is independent T3-2: diagram validation/recovery consumed 26 rejects/11 patches and shipped a degraded answer with most item citations removed. Model also used VISIT_CREATED/CREATE_VISIT while source says visit.insert. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
