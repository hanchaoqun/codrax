# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T00:23:52Z
- sweep_start_ts: 20260817-172351
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-172352 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1030 production closure: model first queried 1.000000..1.010100, while the deterministic supplement queried the exact requested 1.000000..1.010000 window; the surviving ranked seat is exact-window worker-200 #1, on-chain priority-inversion candidate, 9.000ms cumulative / 8.300ms effective. Explicit-window projection, wake chain, on-chain-only root cause and background-only context remain correct. Human usability is partial because raw enum/key surfaces such as state_partition_coverage=complete, priority_inversion_candidate, tier=primary and chain_relevance=on_chain still leak into Chinese prose, and the cross-artifact boundary appendix is disproportionate for one inline trace. Track as presentation/context debt; do not change the typed computation or hard-gate model prose. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-172352 | answer_regex,answer_contains,mermaid_edge_count | none | 232s | 35 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=4/0,fin_reject=2,unavail=0,prune=0 | partial | Final Mermaid is syntactically valid and preserves Analyzer→Explorer→Extractor→Finalizer plus two grounded calls, but BusContext/Mutable remain disconnected/unproven. Explorer read the exact `o.busCtx, types.AgentExtractor` call yet emitted only the call row. Root is deterministic: operation identity binding searched only the current file, package-import aliases did not canonicalize package functions, and exact stage arguments were not joined through checkout-verified stage authority. Two final rejects correctly removed invented edges, but the missing typed bridge forced a weaker diagram and extra retry churn. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized disposition

- `B1030-TRACECHAINROOTCLASSIFICATION1`: production closed by r653; retain the exact-window survivor rule and its wider-window negative pins.
- `B1031-CROSSFILECARRIERARGUMENTFLOW1`: implement same-language/same-package cross-file declaration lookup, exact resolved-import alias call identity, and checkout-verified stage-argument identity. These mechanisms only create model repair debt for exact parser-owned argument rows; they do not emit evidence or author diagram edges.
- `B756-RUNTIMEENUMCUSTOMERLANGUAGE1`: production still partial. Address separately through typed field/value presentation metadata and localized soft teaching, never by scanning or rewriting model output strings.
- Active streaming showed no fixed-4ms fallback. No malformed JSON, draft recovery, or missing answer occurred in either case.
