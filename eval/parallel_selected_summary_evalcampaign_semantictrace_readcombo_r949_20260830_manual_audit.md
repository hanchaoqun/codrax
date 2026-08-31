# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T03:09:16Z
- sweep_start_ts: 20260830-200911
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-200916 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 232s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Runner false-red on stale `边锚定` token. Current typed contract correctly preserves the 0.285ms class-verification span as raw occupancy/business evidence, shows the exact 34579.496810s direct wakeup relation, and keeps effective impact at zero with `仅关系凭证` / `优化项,非根因`. The model introduction nevertheless called an ordinary runnable delay “确定性优化工作”; projection facts remain correct and no product prose hard gate is warranted from one sample. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260830-200916 | answer_regex,answer_contains | none | 851s | 48 | read=26,repo_map=3,list=0,trace=0,source_lens=0 | midloop=21,inv=7/2,fin_reject=3,unavail=0,prune=1 | pass (process partial) | Final answer contains all four stages, complete input/output/state-carrier rows, and a valid three-edge analyze→explore→extract→finalize sequence; BusContext is honestly disclosed as isolated rather than connected by an invented edge. Process churn is excessive: two typed completion rejects were hidden by the old metric, and one reject demanded `o.busCtx.Mode` / `.Language` solely because their owner binding was `BusContext`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `B1463-CARRIERMEMBERARGUMENTOVERMATCH1/P1` is confirmed. Ordinary call-argument sibling repair treated every descendant scalar of a typed carrier binding as the whole incident participant. The generalized fix requires the argument itself to terminate at the declared carrier binding; assignment-call member data flow keeps its existing broader typed path. Implemented and pushed as `7e72e91e6`.
- `B1464-SEMANTICREJECTMETRIC1/P2` is confirmed. Emit tools return transport `ok=true` for a typed semantic rejection, so the runner reported zero investigation rejects despite two anchored `emit_investigation_complete rejected:` results. The metric now counts anchored control-plane semantic rejects without counting assistant/prompt prose; implemented in `a0c035f0f`.
- `B1465-H8STALESEMANTICORACLE1/eval` is confirmed. The case still required the retired `边锚定`/ranked-seat interpretation although the current product contract deliberately distinguishes wakeup relation from semantic-completion causality. The oracle now requires `唤醒锚定`, the direct-edge credential, the relation-only caveat, and the non-root optimization position. No Trace product logic was changed.
- No fixed elapsed-time, iteration, context-percent, request-text, answer-text, or Mermaid-message gate was added. Trace explicit-window selection, causal projection, automatic supplementation, typed on-chain root ranking, raw occupancy, and priced eliminable accounts remain unchanged.
