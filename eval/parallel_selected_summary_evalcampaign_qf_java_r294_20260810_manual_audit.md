# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T05:09:43Z
- sweep_start_ts: 20260810-220941
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-220943 | primary_answer | none | 142s | 22 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B506 positive: capacity-check row keeps `VisitService.java:18`. B508 persists: Explorer read `AuditLog.java:6` but emitted only a definition-shaped row at line 5; final callable authority stayed `definition_status=unproven`, while the answer still called stdout logging “审计落库”. Discover-sink selection also misdirected three rounds toward unrelated config precedence before bounded convergence. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260810-220943 | answer_regex,answer_contains | none | 633s | 45 | read=21,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=7,unavail=0,prune=4 | fail | B507 provider parity is positive: the prompt carried all three checkout-verified `stage_precedence` recipes. The model did not consume them because Analyzer invented hard `incident_required` participants (`Orchestrator`, `Agent`) absent from the request and omitted named carriers (`EvidenceItems`, `AnswerDocument`, `Mutable`); completion then forced 27 explore rounds/33 evidence rows and finalizer spent 7 rejects on disconnected boundaries. Final diagram shows scheduler helper calls, not the requested analyze→explore→extract→finalize stage sequence. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner `PASS` is not human correctness for either case.
- `B506` is production-closed by this replay; `B507` is provider/consumer-wiring closed but its visible value is masked by the independent participant-authority gap.
- New `B509-PARTICIPANT-PROVENANCE/P1-high`: `diagram_hint.participants` is documented as current-request-only planning guidance, yet an Analyzer guess becomes a hard completion/finalizer obligation with no per-participant user-provenance carrier. This is a contradictory authority upgrade, not model-only variance.
- `B508-TERMINAL-BODY/P1` remains open. The next repair must improve typed Explorer→Finalizer body evidence and discover-sink repair targeting; it must not infer behavior from names/comments or scan/rewrite final prose.
