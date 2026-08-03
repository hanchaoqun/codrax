# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T09:45:44Z
- sweep_start_ts: 20260803-024542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-024544 | answer_contains | none | 81s | 20 | read=1,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Closure prose correctly says two caller functions, and exact typed relation projection publishes two unique principal candidates. The emitted aggregate fact nevertheless uses three callsite observations as members. Because analyzer `question_kind=call_chain` drifted with `predicate_axis=unknown` and `is_relational_lookup=false`, `HasTypedRelationMemberSetShape` did not grant exact relation canonicalization; finalizer therefore listed the first function twice. B52i: accept typed `ReqCallChain` as relation member-set shape, then collapse repeated exact observations onto candidate identity. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-024544 | primary_answer | none | 147s | 20 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | B52h's `callable:line` trigger did not occur because the model used bare method labels; the deterministic positive/repair pins remain the authority for that fix. The graph keeps resolveMaxVisits/insert/record but drops the real countOpenVisits invocation after an earlier draft substituted an abstract CapacityCheck node and made it call insert. Extend the existing soft guide: a compound guard preserves its grounded call edge and renders the comparison separately; an abstract guard never replaces the callee or becomes the post-guard caller. The answer also continues to describe stdout AuditLog as durable audit logging; this remains a model wording error, not system rewrite authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52h-CALLABLE-LINE-CITATION-OWNERSHIP`: implementation remains covered by deterministic cross-language pins; r9 is a no-trigger replay and cannot supersede them.
- `B52i-REQCALLCHAIN-RELATION-IDENTITY`: confirmed P1. `ReqCallChain` already selects exact `called-by` candidates, but member-set ownership additionally required an axis/predicate that the same analyzer omitted. The two typed decisions drifted and allowed observation-axis rows to escape canonicalization.
- Cross-language graph residual: compound guard expressions that call a function need two visual semantics—a real caller-to-callee invocation plus a separate condition/branch annotation. This is soft authoring guidance only; do not add a label-text rejection or system-authored graph rewrite.
