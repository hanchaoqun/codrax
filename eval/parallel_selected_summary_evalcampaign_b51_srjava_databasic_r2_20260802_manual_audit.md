# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T03:04:34Z
- sweep_start_ts: 20260802-200432
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260802-200434 | log_regex,answer_regex | none | 102s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final product is exactly `17`, matching the supplied row-filter/sum rules. Data workflow took 7 rounds and 2 planning repairs before converging; this is latency/churn evidence, not an answer-correctness failure in B51. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-200434 | primary_answer | none | 356s | 23 | read=18,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=8,unavail=0,prune=0 | fail | B51-A correctly preserved the model's six-step principal path and the new primary oracle was satisfied by that body rather than citations/raw recovery. Residual system failure: analyzer R1 amplified a typed narrative call-chain into category enumeration, forcing 3-row and 14-row duplicate operation rosters plus 8 finalizer rejects. Independent model variance remains in hop 6 prose (`CREATE_VISIT`, `visitId`) despite the cited source showing `visit.insert`, `petId`; the system citation repair did not rewrite the conclusion. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
