# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T00:00:33Z
- sweep_start_ts: 20260813-170032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-170033 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 103s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Model answer contains the complete 233.190ms state partition, CPU=4 2100MHz direct limit row, and the correct unproven target-binding verdict; no full causal projection was injected. Auto FAIL is an oracle spelling gap (`CPU=4` was not accepted). Analyzer still emitted bounded_fact_set/observed_value instead of bounded_effect_verdict/causal_attribution, so B749 is not production-closed. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-170033 | answer_regex,answer_contains,mermaid_edge_count | none | 286s | 36 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=3,unavail=0,prune=0 | partial | B751 component fragments reached production and the model copied their n# topology. It omitted optional from_identity/to_identity fields; the validator then resolved business/split labels as Orchestrator and rejected five exact call edges. After three retries the model deleted all supporting call/data-flow components, leaving only the stage precedence spine and disconnected BusContext/Mutable. Confirmed B752 contract self-conflict, not model-only fluctuation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B751 improved evidence delivery but did not close production relation retention. The missing seam is typed recipe receipt consumption: exact endpoint identity was optional in JSON, while the hard gate fell back to visible display labels when the model omitted it.
2. B752 repair direction is metadata-only and fail-closed: retain the finalizer's exact recipe anchors for the dispatch and restore a missing identity pair only for an already visible/model-authored edge with the same node IDs, direction, and relation kind. It must never add an edge, relation, business label, order, or conclusion.
3. The Trace answer itself is materially correct. The case oracle now accepts `CPU4`, `CPU 4`, and `CPU=4` spellings without weakening the CPU/lane, policy-ceiling, or unproven-binding requirements.
4. B753 remains open for another production replay: the analyzer ignored the new finite-effect scope in this run. No raw-request/prose keyword hard gate will be added; any follow-up must simplify or strengthen typed classification without system-authored conclusions.
5. No malformed JSON recovery, stale-draft fallback, empty answer, or active-stream 4ms degradation occurred in either run. The active-byte-stream contract remains: transport activity cannot authorize degradation merely because a complete SSE event/answer has not arrived yet.
