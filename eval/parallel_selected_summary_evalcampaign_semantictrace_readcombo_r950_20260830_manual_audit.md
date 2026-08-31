# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T03:41:29Z
- sweep_start_ts: 20260830-204128
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-204129 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 192s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 10ms window, five typed queries, one final projection, ranked on-chain causes, raw/priced dual account and automatic supplement are intact. The 0.285ms class-verification row retains exact direct-wakeup evidence but publishes effective=0, `仅关系凭证`, and `优化项,非根因`; no neighboring/background row was promoted. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260830-204129 | answer_regex,answer_contains | none | 493s | 50 | read=14,repo_map=1,list=0,trace=0,source_lens=0 | midloop=15,inv=7/1,fin_reject=3,unavail=0,prune=0 | partial | B1463 is production-positive: no completion reject demands `o.busCtx.Mode` or `.Language`; reads fall 26→14, repo maps 3→1, explorer iterations 53→30, wall 851→493s. The stage table is complete, but the final sequence declares `Analyze` and emits `analyze->>Explore`; Mermaid treats those as distinct IDs, so the first relation is attached to an implicit duplicate actor despite runner PASS. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `B1463-CARRIERMEMBERARGUMENTOVERMATCH1` is production-positive. The remaining single investigation reject is an exact call-result assignment at `internal/analysis/dataflow/engine.go:108`, not a carrier-member overmatch. It is now counted accurately by B1464.
- `B1465-H8STALESEMANTICORACLE1` is closed: the updated oracle passes while the stricter relation-versus-causality boundary remains visible and the Trace root-ranking contract is unchanged.
- New `B1466-SEQUENCEPARTICIPANTCASEIDENTITY1/P1` is confirmed. The atomic sequence addition normalizer used case-insensitive declaration matching even though Mermaid IDs are case-sensitive. That made a lower-case endpoint appear declared to the validator while the renderer created a duplicate actor. The generalized exact-ID fix and red/green regression were pushed as `a11327ced`; production replay remains pending.
- The system still does not choose a relation, label, participant, wording, layout, or conclusion. The B1466 normalization only reuses the unique exact declared ID already authored by the model for the same typed endpoint.
