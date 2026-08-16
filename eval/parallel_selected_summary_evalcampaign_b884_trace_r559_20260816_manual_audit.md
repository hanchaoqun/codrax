# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T09:29:09Z
- sweep_start_ts: 20260816-022908
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-022909 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 100s | 36 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Bounded state/effect query correctly stayed out of full causal projection and preserved the explicit window. The four-state account is exact and the answer correctly says policy ceilings exist but target binding is unproven. However it compares CPU12's 2075MHz observation with CPU4's 2100MHz policy ceiling and calls CPU12 a main core, despite the typed context explicitly forbidding cross-CPU transfer. Runner failure is partly an oracle-order false negative, but the cross-CPU factual claim independently makes the human verdict fail. |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-022909 | answer_regex,answer_contains | none | 501s | 49 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=13,inv=4/0,fin_reject=9,unavail=0,prune=0 | fail | B884b production-positive on the first repair: completion published an exact cmd/root.go range and the very next turn called read_file without broad grep. The read was not settled mid-loop, so the same internal/agent/agent.go range was read three times; target selection also preferred an early weak preview occurrence over the exact tool-schema type usage. Finalizer then retried nine times because every participant mismatch misleadingly began with a JSON-placement warning although participant_boundaries was already block-level. The final table is useful, but the diagram still omits the requested full-vs-patch finalizer dispatch relation and instead shows registration/name-return/preview side relations. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
