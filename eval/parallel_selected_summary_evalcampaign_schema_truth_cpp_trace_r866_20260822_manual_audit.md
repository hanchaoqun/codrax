# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T15:40:30Z
- sweep_start_ts: 20260822-084029
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260822-084031 | answer_regex,answer_contains | none | 173s | 29 | read=6,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | Core call/selection answer is grounded and the final Mermaid renders. B1351 first batch prevents the old fused section+diagram/hollow-title symptom, but the model stringified the whole blocks array; executor recovery then admitted a diagram that the live schema had omitted. Final prose also overstates the base no-op flush as ensuring console output. |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-084031 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000..2.020s window, four-node wake chain, 11ms on-chain IO first root, three independent 1ms runnable/inversion candidates, actual-time/removable accounts, business clues and full Trace causal projection all remain. Neighbor/background rows stay support-only; no fixed-duration downgrade. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `r866=runner-pass-2/2,human-cpp-partial+trace-pass`.
- The first B1351 batch is production-positive for dispatch-local kind teaching and fused-carrier prevention, but not yet closed.
- The residual is deterministic, not model fluctuation: the finalizer context simultaneously said `requested outputs: summary, trace, diagram` and `Allowed block kinds: summary, ordered_list, diagram`, while the live schema had `has_diagram=false`; then lossless string-wrapped `blocks[]` recovery bypassed the item-level projected schema.
- Generalized fix boundary: optional `DiagramHint` stays advisory; intent/support/tool schema share the same typed live-kind projection; any recovered native `blocks[]` is revalidated against that same dispatch schema. JSON repair stays enabled and no answer relation, label, layout, or conclusion is system-authored.
