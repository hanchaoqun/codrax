# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T04:42:52Z
- sweep_start_ts: 20260810-214250
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-214252 | primary_answer | none | 118s | 22 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The model emitted the correct four typed calls and correctly cited `VisitService.java:18` for the capacity check. Deterministic principal-aggregate normalization then rebound that item to the label-only caller row `VisitController.java:18`, pruned the correct citation, and degraded a valid answer. Separately, the answer says `AuditLog.record` persists the audit event, while its body only calls `System.out.println`; terminal implementation evidence was absent, so the requested “audit persistence” endpoint was overclaimed. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260810-214252 | answer_regex,answer_contains | none | 358s | 40 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=5,unavail=0,prune=0 | fail | B505 closed the exact-display-identity contradiction: `Analyzer Agent`/`Finalizer Agent` are no longer simultaneously unknown and missing. The run still spent five finalizer rejects and produced only one proven diagram edge. Analyzer had promoted seven inferred architecture components to `incident_required`; the hard coverage contract then forced them back as disconnected nodes even though the user asked for the complete stage flow. The prose/table also state several unproved transitions. Runner PASS therefore remains a human false green. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2; human: 0/2.
- B505 has a direct production witness and closes: exact typed participant identities containing spaces now match their own boundary rows, and the former same-response `unknown + missing` contradiction did not recur.
- `B506-CITMONO1/P0-redline` is deterministic system evidence degradation. A label-only aggregate support row must not overwrite an already valid citation that grounds the exact item identity plus a second visible typed attribute. The fix is a monotone keep rule over structured item fields and grounded evidence; it never edits answer prose or creates a claim.
- `B507-PARTOBLIGAUTH1/P1-high` is now confirmed across repeated architecture replays: analyzer-authored participant roles are planning guidance but are consumed as hard presence authority without an independent relation witness. This forces context/inferred components into diagrams as disconnected nodes, burns retries, and still cannot recover the requested relations. The next batch must separate upstream evidence-coverage guidance from final relation authority; no request/final prose scan and no system-authored edge is allowed.
- `B508-TERMINALBEHAVIOR1/P1` records the Java endpoint overclaim. A callsite proves `VisitRepository.insert -> AuditLog.record`, not that `AuditLog.record` persists anything. Terminal behavior needs definition/body evidence or an explicit unproven disclosure; the system must not infer persistence from the question or symbol name.
