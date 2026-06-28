# Eval audit summary — log/current-source after D1-F10g.207h/i

Run: `eval/results/read_combo_log_current_source_explanation-20260628-150656`

Case: `read_combo_log_current_source_explanation`

Verdict: PASS

Key metrics:
- `tool_read_file=7`, `tool_repo_map=2`, `source_inventory_lens=0`
- `wall_seconds=193`, `max_context_tokens_est=57100`, `max_context_window_pct=29`
- `analyzer_iters=3`, `explorer_iters=8`, `finalizer_iters=1`
- `midloop_inject=2`
- `completion_lane_fired=0`
- `investigation_complete_calls=1`, `investigation_complete_rejects=0`
- `finalizer_rejects=0`, `answer_contract_advisories=0`

Delta versus pre-fix run `read_combo_log_current_source_explanation-20260628-145313`:
- `wall_seconds`: `231 -> 193`
- `max_context_tokens_est`: `61219 -> 57100`
- `midloop_inject`: `7 -> 2`
- `explorer_iters`: `11 -> 8`
- `investigation_complete_calls`: `2 -> 1`
- `answer_contract_advisories`: `4 -> 0`

Manual audit:
- No `emit_analysis rejected` event remains. Invalid optional `exact_targets` no longer force classifier retry.
- No `emit_investigation_complete DOWNGRADED` event remains. Decorated aggregate support refs are repaired/accepted locally when unique current-source anchors are available.
- The final answer remains functionally correct and grounded.
- `completion_lane_fired=0` remains an observability/future-optimization gap, but no longer causes this case to loop or retry completion.
- The only eval advisories are high wall time and mermaid repair churn. Mermaid repair is tracked separately by another owner; this batch does not modify it.
