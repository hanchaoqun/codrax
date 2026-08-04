# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T07:54:28Z
- sweep_start_ts: 20260804-005426
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260804-005428 | answer_regex | none | 122s | 24 | read=4,repo_map=0,list=2,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Routing and the typed count are correct (253), but the mechanism answer reverses the adapter/compiler direction, cites `compileToolResultObservations` in `internal/tool/emit_investigation_complete.go` although it lives in `internal/types/observation_ledger.go`, and incorrectly joins ledger compilation to `MutableState.InvestigationComplete`/`HasEnoughFacts`. The prompt named the right high-level path, but did not provide file-owned adjacent edges or a typed separation from the independent explorer-closure path; the model read the hint renderer and closure type and synthesized a false bridge. |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260804-005428 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 228s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Relation completion closed in one call with zero rejects, and exact-window projection/supplement stayed correct at 20.000ms with the 11.000ms IO root and wakeup chain intact. However Explorer emitted an unbound runtime scalar `20.020ms` as `principal_answer`; because it had no typed window/metric/support identity, the existing role normalizer treated it as a direct runtime observation and finalizer was told to preserve it over the deterministic 20.000ms partition. Summary and caveat therefore contradict the system projection. This is an authority/context gap, not authorization to scan or rewrite answer prose. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B68-RELTAIL1`: closed by replay. Trace exploration completion was accepted once; the former 20-attempt relation-carrier retry storm did not recur.
- `EVAL-B69-CMDPATH1`: open. Current-source routing and measurement transport work, but the finalizer lacks a precise, source-owned edge list and an explicit typed boundary between observation compilation and exploration closure.
- `EVAL-B69-RUNTIMEBIND1`: open. In an explicit-window runtime analysis with deterministic target-state authority, a model-authored scalar without typed artifact/window/metric binding must not remain principal numeric authority. Keep it as model reasoning/supporting context; do not delete it or rewrite the answer.
