# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T12:07:15Z
- sweep_start_ts: 20260814-050713
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-050715 | answer_regex,answer_contains | none | 316s | 31 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=1,unavail=0,prune=0 | partial | B790 production-positive: First-Pass carries the exact two-edge Mermaid body plus matching `edge_anchors_json`, the model copied both, and the prior `agent.buildAnalysisIR -> gate.RunWith call_edge_unproven` rejection disappeared. The sole reject was B789 correctly catching a definition item and two uncited items inside `principal_path_edge`; one patch converged. Final diagram and no-directed-path conclusion are correct, but the prompt itself simultaneously emitted requested-sink path `gate.Run -> gate.RunWith` and `requested_sink_existence_proof=definition_only / incident_call_evidence=not_emitted`; the final copied that false state. B792 confirmed system context conflict. |
| 2 | s8a | PASS | eval/results/s8a-20260814-050715 | answer_regex,answer_contains | none | 324s | 33 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=6/1,fin_reject=1,unavail=0,prune=0 | partial | B789 again correctly rejected 15 local calls in the principal endpoint facet plus missing exact endpoint labels; one patch split them into exact endpoint and support blocks. No diagram was required by this case, so its absence is not degradation. Final preserves useful local calls but repeats the system's contradictory definition-only state and says the two callers are “并行指向” one callee despite explicit typed non-concurrency guidance; the latter remains model adherence, not a prose hard-gate candidate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS. Human: 0 pass, 2 partial.
- B790 is production-positive: initial diagram teaching and strict relation validation now use the same typed body/anchor carrier; neither case repeated the former identity rejection.
- Both prompts exposed B792: the call graph consumed parser-owned `OwnerSymbol=gate.Run`, while endpoint-existence consumed only short `Subject=Run`. One typed capsule therefore both proved and denied the requested sink's incident call edge. This is a precise system context contradiction and directly contaminated both finals.
- B792 now canonicalizes existence identities through the same fail-closed call-graph projection. A unique qualified owner coalesces short definition/call spellings; multiple owners with the same operation tail remain ambiguous.
- No malformed JSON, empty answer, stale-draft fallback, timeout, or fixed-age active-stream degradation occurred.
