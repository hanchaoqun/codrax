# D1-F10g.172 Focused Eval — Read Combo Contract Advisory Telemetry

Date: 2026-06-27
Case: `read_combo_log_current_source_explanation`
Result dir: `eval/results/read_combo_log_current_source_explanation-20260627-174912`

## Verdict

| case | verdict | wall(s) | notes |
|---|---:|---:|---|
| `read_combo_log_current_source_explanation` | PASS | 194 | No finalizer retry/rewrite; no unavailable tool attempt; no tool-history prune |

## Metrics

- `answer_contract_violations=0`
- `answer_contract_strict_violations=0`
- `answer_contract_advisories=2`
- `finalizer_iters=1`
- `finalizer_rejects=0`
- `finalizer_rewrites=0`
- `[CGEC] summary` now reports `strict_findings=0 advisory_events=3` instead of a generic `violations=N` tail.

## Audit

D1-F10g.172 is functionally closed for the targeted gap: non-strict answer-contract observations no longer look like blocking violations in eval metrics or CGEC summary.

Residual non-blocking signals remain for the broader noise backlog: `midloop_inject=4`, `max_context_tokens_est=70597`, and `repo_lens_discovery_hints=3`. These should stay on the read-mode noise/context-governance track and must not be folded back into contract-failure handling.
