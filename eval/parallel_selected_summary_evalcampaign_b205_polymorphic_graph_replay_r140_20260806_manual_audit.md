# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T01:01:18Z
- sweep_start_ts: 20260806-180117
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-180118 | answer_regex,answer_contains | none | 107s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | S36c typed recipes removed completion rejects and preserved the diagram, but the answer incorrectly says only `level >= kError` enters the write path (the source only gates `flush`) and says the unknown-sink arm may throw (source returns `nullptr`). The emitted flowchart also failed the bundled Mermaid.js 11.12.0 parser because JSON-style `\"` survived inside already-quoted labels; several untyped logical arrows additionally remain outside the strict relation-anchor contract. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-180118 | answer_regex,answer_contains | none | 114s | 20 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | The first structured draft copied the supplied typed call/return/register recipes, but the validator misparsed the sequence message `resolve("json")` as a document node alias `resolve -> json`, rejected its own correct recipe, and drove the repair to delete relation metadata and the diagram. Final prose identifies `JsonPlugin` and import-time registration, but omits the requested cooperative `TimestampMixin -> ValidationMixin -> BasePlugin` execution path. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
