# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T01:39:15Z
- sweep_start_ts: 20260810-183914
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260810-183915 | log_regex,answer_regex | none | 34s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Strict JSON result is exactly `{"ids":["u1","u3"]}`; active filter and source order are correct. One data-workflow batch, no malformed-JSON repair, retry, prose wrapper, or missing answer. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-183915 | answer_regex,answer_contains,mermaid_edge_count | none | 271s | 37 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | Runner false positive. The final diagram preserves only the three stage-precedence edges and leaves `Orchestrator`, `BusContext`, and `Mutable` disconnected, while prose still claims shared no-copy data flow. Analyzer correctly made the named carriers incident-required, but Explorer repeatedly emitted declaration-only `Mutable *MutableState` as `initializer`; grounder rejected it. The rejected row nevertheless re-entered finalizer handoff twice as `lane=value_fact ... authority=illustrative`, exposing an ungrounded-to-value-lane authority leak. B490/B493 were not disproved: no semantic-call row reached their new repair path in this replay. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r285 judgement

- Runner: 2/2; human: 1/2.
- JSON lane is healthy and low-churn. Its strict-output teaching did not conflict with the workflow contract and did not require answer recovery.
- QF failure is not a reason to weaken the relation validator or author a system graph. The validator correctly removed every unproved carrier edge and preserved the checkout-verified stage ordering.
- `B495-UNGROUNDEDLANE1/P1-high`: typed enrichment currently admits explicit `GroundingUngrounded` evidence. Because lane selection looks at `AnchorInitializer` before authority, a rejected declaration can be replayed as a `value_fact`, contradicting both the grounder and completion caveat. Fix at the typed handoff boundary: ungrounded rows cannot enter factual value/flow/chain lanes; they may remain only in an explicit boundary/unverified surface when that surface is useful.
- `B496-DECLREPAIR1/P1`: the grounder has a precise source-shape signal (identifier visible, assignment/initializer shape absent), but its repair remains generic. Publish a typed, source-local correction explaining that this line cannot prove value transfer and that a declaration may only be re-emitted as a definition; real flow still requires a writer/reader operation. Do not auto-create an edge or silently reinterpret a semantic relation.
- `B497-PARTINVENT1/P2-watch`: Analyzer added `Orchestrator` beyond the six identities explicitly named by the request. This increased the required relation slate and finalizer churn, but one run is insufficient for a hard policy. Raw-request keyword gating is prohibited; retain as a heterogeneous replay watch item.
