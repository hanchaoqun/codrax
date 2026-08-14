# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T18:22:00Z
- sweep_start_ts: 20260814-112159
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260814-112200 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 133s | 32 | read=2,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | All state/runtime values and both policy-limit rows are correct; the conclusion says target binding impact is unproven. Machine FAIL is mainly a wording-oracle miss (`频率限制证据`/`未证实`), but the explanation improperly centers cpu12 and does not explicitly reconcile the target's proven 35.960ms on cpu4 with the separate unproven binding-impact caliber. Analyzer also mislabeled the target-effect dimension as `observed_value`, a model-variance watch item rather than a safe hard-gate trigger. |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260814-112200 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 234s | 45 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B817 production positive: accepted analysis is `causal_diagnosis` with required `causal_contributor_set`; full Trace causal projection, 65.912/36.757/49.623/0.033ms, incomplete enumeration and unpriced occupancy all returned. Model prose still calls four CPU-grouped D members “4 independent segments” despite 11 actual waits and over-interprets the blocked caller as GPU-fence mechanism, so PASS is not a full human pass. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- H7 first emitted an invalid runtime target tuple, then a bounded root-cause
  tuple. The existing root-cause consistency gate rejected that second tuple;
  the new teaching made the third complete emit converge to
  `causal_contributor_set + causal_diagnosis`. Final projection count is one,
  and the model-authored answer plus deterministic projection preserve the
  requested major/minor on-chain contributors. No system-authored conclusion
  or answer replacement occurred.
- H4 remained `bounded_fact_set`, so the new causal-roster role did not widen a
  finite question. The finalizer nevertheless received precise typed frequency
  authority: cpu4 target runtime, direct policy-ceiling witness, and the
  separate `target_binding_status=unproven` caliber. Its direct conclusion is
  therefore directionally correct; the weak cpu12-centric rationale is a model
  synthesis issue, not missing context.
- A single replay is not sufficient to turn H4's analyzer role drift into a
  new hard validator. No independent typed signal distinguishes the incorrect
  `observed_value` role from the intended single target-effect verdict; reading
  its label/source quote to force a retry would violate the precise-signal red
  line. Keep it as a replay watch and prefer clearer soft teaching only if the
  same drift repeats across heterogeneous effect questions.
- Neither case used malformed-JSON salvage, stale-draft fallback, finalizer
  rewrite, or fixed-age active-stream degradation.
