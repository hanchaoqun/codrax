# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T10:05:11Z
- sweep_start_ts: 20260814-030510
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-030512 | log_attachment,answer_contains | log_triage | 104s | 23 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Both requested frame sets were preserved correctly: Cangjie `demo.bridge.ohSum:18` / caller `checkout:42`, ArkTS `NativeBridge.invokeOhSum:33:11` / caller `HomePage.computeTotal:54:7`. The first log-triage emit incorrectly tried to create a cause edge and was correctly rejected because there was no explicit chain marker; the accepted bundle contained two peer errors. Final prose nevertheless called Cangjie the established root and claimed propagation. The finalizer prompt exposed the contradiction: prose boundary `cross_error_relation=unproven`, but both ledger rows were typed `lane=observed_direct_cause`. B784 separates observed error occurrences from marker-proven nested causes and emits a typed peer-relation boundary; it does not rewrite the model answer. |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260814-030512 | log_regex,answer_regex | none | 201s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B782 received production positive evidence: compact repair retained the complete-reference path/key tuple, the workflow reconciled successfully, and the exact answer was `17,0,5`. Correctness is restored, but the tiny four-file job still needed 10 data rounds, 2 repair rounds, 7 failed actions and 195s; this remains an efficiency/churn observation, not authority to merge plans, skip typed ranks, or synthesize business values. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner: 2/2 PASS. Human: one pass, one partial.
- B782 is production-positive for correctness. The remaining Data wall time is tracked as typed-DAG/model planning churn; no unsafe shortcut was introduced.
- The mixed-language result exposed a generalized context-authority conflict, not a language-specific parser defect: peer errors were represented as direct causes in the shared Observation Ledger. B784 introduces `observed_error_occurrence`, reserves `observed_direct_cause` for a nested error whose incoming `cause_relation` has a validated verbatim artifact marker, and publishes `log:cross_error_relation value=unproven` for multiple top-level occurrences.
- The change is language/runtime agnostic, reads only structured log-error topology, and never scans user/model/final prose. It preserves both ArkTS and Cangjie frames while leaving the bridge/root conclusion to the model under an internally coherent evidence ceiling.
- This batch does not touch Trace query, explicit windows, automatic supplementation, causal projection, on-chain root election, background support lanes, or active-stream timeout/recovery behavior.
