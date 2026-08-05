# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T20:45:05Z
- sweep_start_ts: 20260805-134504
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_mro_order | PASS | eval/results/sr_py_mro_order-20260805-134505 | answer_regex,answer_contains | none | 87s | 19 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Core MRO and the three-base order are correct, proving the relation-preservation fixes reached the answer. The model nevertheless calls the timestamp overwrite “idempotent”, which is not value-idempotent, and the system emits two complementary same-title 2-row completeness tables instead of one 4-row typed roster. The incidental semantic overstatement is model variance; the duplicate supplement is a deterministic presentation gap. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260805-134505 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 5.000–5.007 window, auto supplement, target-state partition, wakeup chain, two root-cause axes, ranked eliminable board and Trace causal projection all survive with zero finalizer rejects. The prior “fix #1 makes #2 disappear” claim is gone, but the model still upgrades causal_conclusion=unproven/frame_evidence_status=absent into a definite frame-delay root cause and describes directions as physically independent. Typed relation/causal ceilings were present; generic Trace opening/root-cause teaching still competed with them, so the soft teaching was made authority-conditional without adding prose gates or system rewriting. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
