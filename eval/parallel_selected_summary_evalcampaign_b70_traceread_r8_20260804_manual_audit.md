# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T08:21:17Z
- sweep_start_ts: 20260804-012115
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260804-012117 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | Second production replay remains correct: selected-window sleep is 20.000ms, 20.020ms is attachment extent only, and the 11.000ms IO root/wakeup chain/occupancy-vs-eliminability/projection remain intact. One exact relation-claim reject corrected a voluntarily submitted wrong member roster; the next call copied the typed five-state partition and completed, so the former format-only retry storm did not recur. |
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260804-012117 | answer_regex | none | 179s | 31 | read=3,repo_map=0,list=2,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The route carried `current_source=required + source=mixed + needs_repo=true`, but Analyzer omitted the optional CurrentSourceExplanationProfile, so the command-measurement path authority never activated. The model read only EmitAnalysis/EmitEvidence/EmitInvestigationComplete and produced a false architecture in which those tools transport/aggregate command measurement. This is a typed obligation propagation gap, not evidence that the new parallel-carrier guidance is wrong. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B69-RUNTIMEBIND1`: remains closed across a second replay.
- `EVAL-B70-CMDPROFILE1`: new P0 soft-context activation gap. A precise route-level current-source requirement can be lost when Analyzer omits its optional explanation profile; downstream prompt guidance must be allowed to consume typed route agreement for mechanism-shaped mixed-source command measurements.
- Do not repair this with request keywords or a final-answer hard gate. The route hint is schema-validated and present in AgentContext; use it only to activate soft source-owner guidance.
