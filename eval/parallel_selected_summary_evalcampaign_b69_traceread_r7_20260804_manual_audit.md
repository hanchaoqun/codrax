# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T08:13:37Z
- sweep_start_ts: 20260804-011336
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260804-011337 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 139s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The selected-window value is now consistently 20.000ms in model summary, caveat, typed state partition, projection, and system cross-check. The 20.020ms attachment extent remains only as explicitly bounded unit provenance. The 11.000ms fscache IO root, wakeup chain, occupancy-vs-eliminability axes, exact-window projection, and automatic supplement all remain intact. Runtime model scalars/member sets are retained as supporting context, not rewritten. |
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260804-011337 | answer_regex | none | 208s | 25 | read=13,repo_map=1,list=2,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | The file-owned direction is now correct and the false EmitInvestigationComplete/HasEnoughFacts bridge is gone. Remaining mechanism drift: the answer draws a direct `ObservationRecord -> AnswerAggregateFact` edge even though the observation ledger and model-emitted completion aggregate are parallel carriers; deterministic count reconciliation is a separate completion path. It also conflates the history branch (VCSMetadata, History=true) with the ordinary command-measurement branch. This is a context boundary gap; no answer-text gate is warranted. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B69-RUNTIMEBIND1`: closed by production replay. Removing runtime text-overlap authority fixed the selected-window contradiction without rewriting model prose or weakening Trace capabilities.
- `EVAL-B69-CMDPATH1`: partial. Source ownership and data-flow direction are fixed; the finalizer still needs an explicit parallel-carrier boundary between compiled observations and model-authored aggregate facts, plus exact producer branch semantics.
- Efficiency observation: Read used 13 reads and 12 mid-loop injections but no rejects. This is slower than necessary, yet evidence acquisition was source-grounded; optimize only after correctness closes, and do not add a skip/keyword hard gate.
