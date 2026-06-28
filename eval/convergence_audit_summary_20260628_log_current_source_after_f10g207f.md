# Eval audit summary — log/current-source after D1-F10g.207f

Run: `eval/results/read_combo_log_current_source_explanation-20260628-145313`

Case: `read_combo_log_current_source_explanation`

Verdict: PASS

Key metrics:
- `tool_read_file=6`, `tool_repo_map=3`, `source_inventory_lens=0`
- `wall_seconds=231`, `max_context_tokens_est=61219`, `max_context_window_pct=31`
- `analyzer_iters=4`, `explorer_iters=11`, `finalizer_iters=1`
- `midloop_inject=7`
- `completion_lane_fired=0`
- `investigation_complete_calls=2`, `investigation_complete_rejects=0`
- `finalizer_rejects=0`, `answer_contract_advisories=4`

Manual audit:
- The final answer is functionally correct and grounded: it explains the distinction between LLM stream timeout and answer-document validation failure, cites current source, and preserves the runtime-log boundary as a caveat.
- Projection fixes improved prompt shape and avoided finalizer retries, but the run still spent extra time before closure.
- Analyzer emitted `exact_targets` copied from runtime/log-derived context rather than the current request. This optional form debt caused a rejected `emit_analysis` and an extra model round.
- The first completion attempt had enough substantive evidence but used decorated `member_set` rows with positional `support_refs` in the wrong member order. This was completion-form debt, not evidence insufficiency, and should be fixed deterministically from accepted evidence/read-file anchors instead of asking the model to retry.
- `completion_lane_fired=0` remains open: the mixed runtime/current-source soft carrier did not surface before later support reads and form repair. This needs typed observability on why the carrier missed, not prompt-only tuning.

Follow-up gaps:
- D1-F10g.207h: drop invalid optional analyzer `exact_targets` instead of hard-retrying the classifier.
- D1-F10g.207i: deterministic aggregate `support_refs` repair for decorated members when unique current-source anchors are already available.
- D1-F10g.207j: instrument/fix mixed runtime/current-source completion-ready non-fire without turning it into a hard gate.
- D1-F10g.207k: de-noise answer-contract advisory churn when strict violations are zero.
