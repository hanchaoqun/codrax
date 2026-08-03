# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T10:00:59Z
- sweep_start_ts: 20260803-030058
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-030059 | answer_contains | none | 107s | 22 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B52i production trigger passes: the final principal table contains exactly the two unique caller functions, while the two callsites in `BuildTypedRelationQueryWithResolvedSources` remain same-row detail. The typed aggregate published `value=2`, two members, and support refs at 294/321; no duplicate system carrier or misattached short member note appears. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-030059 | primary_answer | none | 220s | 21 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | The visible graph now preserves the real `VisitService.schedule -> VisitRepository.countOpenVisits` invocation instead of replacing it with an abstract guard, so the soft guide improved the requested graph. However the model attached only `relation_kind=guard` to that same call-DAG edge; the exact call evidence existed, yet the validator treated any typed non-call relation as an escape from call authority. B52j: in QFCallChain call_dag, a same-endpoint exact typed call requires a call anchor even when an additional guard relation exists. The answer also repeats the unsupported phrase “只读日志”; the fixture body only proves `System.out.println`. This is model/explorer evidence wording drift, not authority for a deterministic system rewrite. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52i-REQCALLCHAIN-RELATION-IDENTITY`: production replay covered; two unique callers are now the sole principal member set and repeated callsites remain details/evidence.
- Compound-condition guide: directionally effective—the real `countOpenVisits` invocation survived—but r10 exposed a typed contract bypass.
- `B52j-COLON-CALLDAG-RELATION-AUTHORITY`: confirmed P1. First, Mermaid edge parsing split the whole line at the first colon, so Rust/C++/Ruby/Cangjie `::` callable labels could erase a flowchart edge before validation. Second, any typed non-call anchor could hide a same-endpoint exact call. Fix both in the language-neutral parser/evidence gate; keep genuine guard-to-outcome edges legal and keep Trace causal-projection families outside this source-call contract.
