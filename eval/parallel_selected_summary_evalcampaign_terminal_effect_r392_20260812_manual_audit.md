# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T14:54:16Z
- sweep_start_ts: 20260812-075414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260812-075416 | primary_answer | none | 110s | 24 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Typed `AuditLog.record -> System.out.println` reached the final prompt, but the answer still called console output “完成审计落库”. Runner was falsely green because it checked names/capacity only. The first diagram draft also represented owner-local operations as self-call edges; the typed skeleton repair correctly removed those unsupported arrows. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-075416 | answer_regex,answer_contains | none | 216s | 30 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly reports parallel convergence `buildAnalysisIR -> gate.RunWith <- gate.Run`, retains grounded intermediate calls and emits valid Mermaid. Removing the fixed five-function oracle eliminated the prior false failure. Completion churn remains observable (five complete attempts) but did not corrupt the answer; keep for later cross-case ROI analysis rather than fitting this case. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
