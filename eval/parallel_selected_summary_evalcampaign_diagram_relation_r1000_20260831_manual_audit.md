# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T04:10:43Z
- sweep_start_ts: 20260831-211041
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-211043 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 10ms window, on-chain ranking, actual-occupancy/existing-rule-eliminable dual accounts, business-span clues, deterministic supplement, and final Trace causal projection are complete. The model-authored lead nevertheless calls NetworkService runnable plus CookieMonster sleep “deterministic optimization work”, uses 6.599ms for the peer whose published sleep is 6.406ms, and claims same-CPU competition for T7 although its typed seat publishes no CPU placement/competitor carrier. The final prompt contains the exact B1523/B1525 evidence ceilings, so this run is recorded as model adherence variance; no prose-scanning hard gate or system rewrite is added. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260831-211043 | answer_regex,answer_contains | none | 269s | 42 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=6/3,fin_reject=2,unavail=0,prune=0 | pass | B1528 is partially production-positive: no separator-bearing lookup key leaks and conditional pre-stage guards are no longer attached to main stages. Core four-stage order and table are correct. The first draft still invents dispatcher/state-flow edges, incurs two relation repairs, and the accepted diagram retains disconnected Orchestrator/Dispatcher/BusContext/MutableState participants. B1529 promotes an existing typed minimal-sequence instruction to the front of the authority context; relation gates and model authorship remain unchanged. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B1527-DIAGRAMPARTICIPANTNORMALIZATIONRECEIPT1/P1`: not directly exercised. This run retained exact current-request `analyze` and `finalizer` participants, so normalized slate was non-empty and no receipt was expected.
- `B1528-READMODEPRESENTATIONBOUNDARY1/P1`: internal-key and conditional-pre-stage arms production-positive; disconnected context-participant arm remains advisory-only and did not reliably guide the model.
- `B1529-READMODEMINIMALSEQUENCEFIRST1/P1`: the typed stage authority already prohibited dispatcher fan-out and unsupported state edges, but this instruction appeared late in a large context. Move a concise minimal-first recipe directly after the canonical sequence: selected stages plus adjacent precedence only; sibling table owns artifacts/carriers unless an independent typed directed-operation recipe exists.
- `B1523/B1525`: typed semantic/scheduler and CPU-placement ceilings are present in the production prompt, but this Trace sample violates both in model prose. Keep as model adherence variance pending heterogeneous replay; do not add a request/final-text keyword gate or deterministic answer replacement.
- Trace projection, explicit window, on-chain-only root election, automatic supplement, and active-stream behavior remained intact.
