# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T09:06:52Z
- sweep_start_ts: 20260803-020650
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-020652 | primary_answer | none | 253s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=10,unavail=0,prune=0 | fail | B52f evidence handoff worked: all four load-bearing direct calls reached the finalizer as grounded typed edges and a method-qualified sequence shipped. The model needed 10 rejects/5 patches after first drawing a guard as an invocation self-edge. Its final message labels still substitute caller/illustrative operations (`create(...)`, `record("CREATE_VISIT", id)`) for the cited callees (`schedule(...)`, `record("visit.insert", petId)`), and prose overstates stdout as durable storage. These are model evidence-consumption errors; do not add an answer-text hard gate or system conclusion rewrite. Add typed-family soft guidance for invocation labels and guard presentation across every executable language. |
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-020652 | answer_contains | none | 322s | 23 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=4/2,fin_reject=0,unavail=0,prune=0 | fail | The model-authored principal markdown table correctly lists exactly two direct production callers. The deterministic aggregate carrier then appends the same two rows as an ordered list because a complete markdown table cannot be selected as the primary typed relation label carrier. This is B52g, a system-owned duplicate-answer gap. Fix in the typed presentation compiler: exact accepted-row coverage plus principal/enumeration annotations may annotate the existing table in place; incomplete tables must retain supplement behavior. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52f-CALL-EDGE-EVIDENCE-HANDOFF`: covered by replay. The finalizer received all four direct Java call edges; the remaining wrong edge labels and stdout/storage wording are model mistakes, not missing typed evidence.
- `B52g-PRINCIPAL-MARKDOWN-CARRIER-DUPLICATION`: confirmed P1. The system duplicated an already-complete two-row model table. This changes answer structure deterministically and is therefore code-fixable without interpreting the model's conclusion.
- Cross-language follow-up: the soft diagram semantics guide must apply to Go, Java, Kotlin, JavaScript/TypeScript/ArkTS, C/C++, Rust, Python, Ruby, Swift, Lua, Cangjie, and other executable languages. Proto/import/inheritance/annotation relations remain declarative unless separate call evidence proves an invocation.
