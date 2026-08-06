# Selected Eval Manual Audit

- date: 2026-08-06T12:37:20Z
- total cases: 2
- parallel: 2
- code baseline: `main@73f95f869`

| case | runner | human | process/context audit | answer audit |
|---|---|---|---|---|
| `data_json_strict_ids` | PASS, 59s | PASS | One data-plan repair: the first custom script declared `instructions.md` as `script_consumed` but did not read it; the typed material guard correctly required the model to add `read_text`. No malformed JSON, finalizer retry, or answer recovery. | Exact JSON-only `{"ids":["u1","u3"]}`; source order and active filter correct. |
| `real_trace_d4_demand_vs_supply` | PASS, 193s | FAIL | Exact pre-final relation was present: rank #1/#2, 95.156ms envelope overlap, `addition=forbidden`. No finalizer reject. The relation followed eight detailed seat rows and still required the model to combine an ID-only pair carrier with values elsewhere. | Demand-side conclusion, window account, wakeup path, roots, dual axes and causal projection remain useful. But prose still says #1+#2 = 43.035ms and calls #2 independent, contradicting the typed overlap and deterministic “不可直加” section. |

## Disposition

- `EVAL-B177-TRACEADD1` is only partial after B178-S1: the precise signal exists, but its presentation still imposes avoidable cross-row mental work.
- `EVAL-B178-TRACEGROUP1` (P1): compile each displayed exact overlap pair into a pre-seat safe aggregation group carrying member values, `members_independent=false`, `aggregation=max_member_only_no_subtotal`, and the safe comparison value. This remains prompt-only; no prose scan, hard reject, or system answer rewrite.
- The data repair is a truthful, one-step typed-material correction and not a JSON schema contradiction. Keep under observation; do not infer `planner_distilled` or mutate a declared consumption mode from prose.

