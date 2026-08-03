# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T10:15:43Z
- sweep_start_ts: 20260803-031542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-031543 | answer_contains | none | 103s | 21 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | B52i remains stable under a second production replay: exactly two unique caller functions, with the repeated callsite represented as detail rather than a third principal member. No duplicate system carrier. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-031543 | primary_answer | none | 139s | 21 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | B52j fires: the guard-only representation is rejected and the final graph keeps a real `VisitService.schedule -> VisitRepository.countOpenVisits` call. A new generic completeness gap remains: the model-selected principal list and typed handoff both contain `VisitService.schedule -> VisitRepository.insert`, but the final diagram replaces it with `capacity Check -> VisitRepository.insert`; the validator proves every visible call edge but never requires every typed call between model-selected principal path nodes to stay visible. B52k: close that set relation using only principal_path_edge items plus citable typed call evidence. The sentence “标准输出流完成落库” remains a model semantic contradiction; no system prose rewrite is authorized. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52i`: covered by two consecutive production replays.
- `B52j`: production trigger covered; the guard-only disguise was rejected and the real `countOpenVisits` invocation survived.
- `B52k-PRINCIPAL-CALL-DIAGRAM-COMPLETENESS`: confirmed P1. In QFCallChain only, when both endpoints are already structured items in the model-owned principal_path_edge carrier and citable typed evidence proves their direction, a strict sequence/call_dag must contain the same call edge. Supporting calls with an endpoint outside that principal carrier must not expand the model's diagram. This is typed structural consistency, not system conclusion takeover.
