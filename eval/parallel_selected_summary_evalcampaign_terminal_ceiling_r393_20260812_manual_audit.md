# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T15:08:12Z
- sweep_start_ts: 20260812-080810
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260812-080812 | primary_answer | none | 231s | 24 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | Not an oracle synonym miss: answer still says `System.out` “完成一次完整的审计落库”. Final prompt unexpectedly carried `selected_terminal_body_calls=unproven`; the concrete-value preview cache was filled before same-dispatch emit_evidence selected the terminal and was not invalidated because read coverage did not change. It also spent repeated completion attempts repairing aggregate member definition/callsite identity. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-080812 | answer_regex,answer_contains | none | 308s | 30 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=5/0,fin_reject=2,unavail=0,prune=0 | pass | Correct convergence conclusion and cited intermediate roster remain intact. Diagram is weaker than prose: it lists several outgoing calls but expresses the `gate.RunWith` convergence only as a note, not the second visible edge. Keep this as a diagram-relation completeness observation for a later heterogeneous batch; do not fit another gate to this case before the terminal-evidence P0 is closed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
