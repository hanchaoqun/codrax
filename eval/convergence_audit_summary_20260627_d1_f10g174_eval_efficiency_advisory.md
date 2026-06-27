# D1-F10g.174 Eval Efficiency Advisory Audit

- Date: 2026-06-27
- Commit under test: local working tree after D1-F10g.174
- Case: `eval/cases/read_combo_log_current_source_explanation.case`
- Result dir: `eval/results/read_combo_log_current_source_explanation-20260627-230605`

## Result

- Verdict: PASS
- Wall: 217s
- Source reads: 4
- Repo map calls: 2
- Explorer iterations: 9
- Midloop injections: 3
- Max context tokens: 45557 / 200000 (23%)
- Tool history prunes: 0
- Unavailable tool attempts: 0
- Finalizer rejects: 0
- Answer contract strict violations: 0
- Answer contract advisories: 1
- Transient retry checkpoints: 1
- Mermaid source repairs: 6

## New Advisory Surface

The summary now reports typed efficiency advisory rows from numeric metrics. This run remained a correctness PASS, while the advisory table surfaced:

```text
high_wall_seconds: wall_seconds=217 limit=180
```

This is the intended boundary for D1-G46: routine correctness verdicts remain stable, and commercially rough convergence is visible for audit and prioritization. Focused eval cases can opt into hard budgets with explicit `MAX_*` knobs; defaults remain advisory-only.

Follow-up audit of the same run showed two additional typed churn signals that should also be visible by default: `transient_retry_checkpoints=1` and `mermaid_source_repair_applied=6`. The harness now includes those metrics, plus contract-advisory and pipeline-dispatch churn, in the same typed advisory/budget surface.

## Follow-Up

- Product-side soft-guidance compaction for non-core proof debt remains tracked under D1-G46.
- Next representative 6-case batch should inspect `Efficiency advisories` alongside correctness verdicts, tool selection, context pressure, and final answer quality.
