# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T17:20:29Z
- sweep_start_ts: 20260816-102027
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-102029 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 141s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | The answer stayed bounded and did not materialize the full causal projection. It correctly published running=157.248ms, runnable=5.604ms, sleep=70.338ms, D/IO=0, CPU4 policy min/max 558000/2100000kHz and CPU12 frequency without a same-CPU policy row. It also kept the policy ceiling separate from the target-effect verdict: the same-CPU identity pair exists for CPU4, but target-running-slice overlap is absent, so actual binding remains unproven. The runner FAIL is a wording-order false negative: the principal answer says `policy 记录 ... 缺乏 target-slice 级别的绑定证据，无法证明`, while the regex requires an unproven token before a later impact/binding token. Human cannot sign full pass because the model says CPU1=7.155ms and then calls 25.207ms the “other CPU(2/3/7/8/13)” subtotal; the typed roster shows those five sum to 18.052ms and 25.207ms includes CPU1, so the prose double-counts CPU1. Analyzer settled on bounded_fact_set + observed_value, not the new target_effect_verdict role; B916 therefore has a safe bounded-production witness but not positive production adoption of the new role. No active-stream degradation or answer replacement occurred. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-102029 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 183s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Full causal diagnosis remained intact: Analyzer emitted causal_diagnosis with a required causal_contributor_set; one model-authored document was accepted without retry; the final contains `Trace 因果投影`, a typed on-chain ranking, separate adjacent/background sections, target running/runnable/sleep/D/IO state account, runnable and compute-supply headroom, priority-inversion candidates, deterministic JIT spans and business-span clues. Adjacent/background rows are explicitly excluded from root ranking. The model's principal summary nevertheless leaks internal enum vocabulary (`fix_direction`, `frequency_thermal`, `io_dependency`, `lock_priority`, `scheduling_supply`) and describes 5.106ms as a direction aggregate without an exact typed additive carrier; the deterministic projection remains seat-local and does not make that aggregation. Treat this as model/context presentation partial, not authorization for a system rewrite or prose hard gate. No 4ms/fixed-age degradation occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
