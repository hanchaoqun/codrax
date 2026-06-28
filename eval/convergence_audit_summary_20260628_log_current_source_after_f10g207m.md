# Convergence Audit — mixed log/current-source after D1-F10g.207m

Case: `eval/cases/read_combo_log_current_source_explanation.case`

Run directories:

- `eval/results/read_combo_log_current_source_explanation-20260628-151746`
- `eval/results/read_combo_log_current_source_explanation-20260628-152458`
- `eval/results/read_combo_log_current_source_explanation-20260628-153308`
- `eval/results/read_combo_log_current_source_explanation-20260628-153958`
- `eval/results/read_combo_log_current_source_explanation-20260628-154436`

## Summary

The case passes throughout. D1-F10g.207m reduced form retry and exploration churn after the 152458 regression. D1-F10g.207n then closed the main mixed runtime/current-source close-ready gap: phase 0 now promotes to phase 1 after successful materialized `emit_evidence`, so completion-ready can fire before the model widens again.

## Metrics

| run | status | wall | explorer_iters | read_file | midloop_inject | complete_calls | complete_rejects | completion_lane_fired | max_context |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 151746 | PASS | 203 | 7 | 8 | 4 | 1 | 0 | 0 | 73566 |
| 152458 | PASS | 295 | 18 | 10 | 8 | 3 | 1 | 0 | 83826 |
| 153308 | PASS | 262 | 12 | 9 | 5 | 1 | 0 | 0 | 74224 |
| 153958 | PASS | 157 | 7 | 7 | 3 | 1 | 0 | 0 | 63752 |
| 154436 | PASS | 146 | 5 | 3 | 3 | 1 | 0 | 2 | 42280 |

## Audit Findings

- Positive: completion form retry is back to zero rejects in the latest run.
- Positive: source-inventory stays out of the mixed runtime/source lane.
- Positive: enumeration push and tool-history prune are eliminated in the latest run.
- Root cause closed: completion-ready did not fire because the explorer remained in phase 0 after successful evidence materialization. The 154436 run logs `Phase 0 -> Phase 1 transition: structured evidence materialized` followed by `explorer.mid-loop.completion-ready`.
- Positive: latest run eliminates answer-contract advisory churn, completion rejects, source-inventory, tool-history prune, and high context-window advisories.
- Remaining tracked gap: `mermaid_source_repair_applied=1` in the latest run. This is a repairable finalizer/render polish item, not a read-loop completion blocker.
