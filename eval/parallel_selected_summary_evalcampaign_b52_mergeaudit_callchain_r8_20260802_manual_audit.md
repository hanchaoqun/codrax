# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T09:29:53Z
- sweep_start_ts: 20260803-022951
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-022953 | primary_answer | none | 119s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Graph semantics improved materially: five exact callee-operation edges shipped, the capacity guard was not forged as a self-call, and churn fell from 10 rejects/5 patches to 2/1. However, the model submitted correct citation refs 0..4 and the deterministic label/citation normalizer rewrote four by endpoint-name affinity, then detached one. Final rows therefore cite prior hops (`VisitService.schedule:21` -> Controller:18, `VisitRepository.insert:23` -> Service:21). This is B52h, a cross-language typed callsite-ownership gap; fix structurally from `qualified_callable:line` + typed evidence subject + exact citation location, not from prose keywords. |
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-022953 | answer_contains | none | 155s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Exactly two direct production caller rows; no indirect-upstream promotion and no second system checklist. This model chose a structured ordered list rather than a markdown table, so r8 proves no regression while the r7 markdown trigger remains pinned by the production normalizer test. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52g-PRINCIPAL-MARKDOWN-CARRIER-DUPLICATION`: implemented and no-regression replay passed. The exact r7 markdown trigger is retained as a deterministic positive/negative production pin because r8's model selected a list carrier.
- Cross-language call-diagram guidance: replay passed. Exact callee operations appear on all five edges; a pure guard is expressed in prose/list context rather than as a fabricated invocation edge.
- `B52h-CALLABLE-LINE-CITATION-OWNERSHIP`: confirmed P1. A structurally correct `Owner.method:line` hop label must bind to the citation at that exact line whose typed evidence `subject` owns the callsite. Endpoint-name affinity is insufficient because the same method is both the previous edge's object and the next edge's subject.
