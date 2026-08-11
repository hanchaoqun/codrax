# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T11:34:54Z
- sweep_start_ts: 20260811-043453
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260811-043455 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | B525 numeric role is production-positive: the lead now keeps the selected query window and target sleep at exactly 20.000ms and leaves the 20.020ms attachment extent in provenance. The typed wakeup path, #1 threadpool io_wait=11ms, three 1ms runnable seats, and background/root-cause separation survive. The answer still overclaims untyped S-state semantics (`它只等待下游线程完成`) and says the 2.020020 sched-in switched the target back to running even though it is outside the selected window; the final typed boundary explicitly forbade both upgrades, so this residual is model noncompliance/context competition rather than missing final facts. The target 20ms sleep is also rendered twice in the system occupancy table, keeping B522 open. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-043455 | answer_regex,answer_contains | none | 286s | 38 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=2,unavail=0,prune=1 | fail | B524 identity carrier works: all final anchors carry exact from_identity/to_identity and concise display aliases are no longer rejected by label-to-evidence reverse resolution. The required diagram remains, but it contains two disconnected components, omits the claimed full Run→task graph→finalizer path, uses sequence reply arrows and literal `calls`/`precedence` plus source locations as visible copy, and exposes internal implementation terms. The first draft invented unsupported bridge/call edges; the second misused participant boundaries; only the third passed by deleting those relations. Typed recipes contained just five relations although prose evidence covered more workflow nodes, confirming a generalized relation-evidence planning/carrier completeness gap rather than B524 regression. At 286s the active stream crossed four minutes and still returned the model-authored answer; no system degraded answer was published. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- `B524-DIAGRAMIDENTITYCARRIER1`: production-positive. Endpoint identity and visible business copy are structurally separate; no old false label-resolution reject recurred.
- `B525-TRACETIMEROLE1`: numeric/window role production-positive; S-state mechanism wording remains production-negative despite an exact final typed ceiling. Do not add a prose keyword hard gate; replay on heterogeneous Trace cases before deciding whether this is model variance or competing system guidance.
- `B526-DIAGRAMRELPLAN1/P1-high`: confirmed. A required end-to-end sequence can have a complete grounded prose/member roster but an incomplete typed relation recipe set. The validator correctly rejects invented bridges, then the only passing repair deletes the missing portions. The repair belongs in relationship evidence planning/completion and typed carrier coverage, not in answer rewriting or permissive validation.
- `B517-STREAMLIVE1`: second production-positive long-task witness. A 286s active pipeline run completed with a model answer and no four-minute degraded response.
