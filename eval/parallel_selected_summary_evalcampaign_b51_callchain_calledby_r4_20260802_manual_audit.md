# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T03:34:52Z
- sweep_start_ts: 20260802-203451
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260802-203452 | answer_contains | none | 205s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Correct exhaustive production caller set: BuildTypedRelationQueryWithResolvedSources and TypedRelationKindsForRequest, both with exact call-site citations; tests excluded and no reject. This is the true relation-enumeration counter-arm and remained intact. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-203452 | primary_answer | none | 370s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=10,unavail=0,prune=0 | fail | Main five-step path and capacity guard are mostly correct, but analyzer omitted optional exact_targets and R1 again amplified the typed call-chain into category enumeration, yielding duplicate 2-row and 5-row system rosters. The model also wrote VISIT_CREATED although the citation/source says visit.insert. Independently, T3-2 reproduced deterministically: a sequence diagram consumed 10 rejects/5 patches and was ultimately removed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
