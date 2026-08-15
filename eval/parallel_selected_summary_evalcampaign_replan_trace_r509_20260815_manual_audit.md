# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T13:57:09Z
- sweep_start_ts: 20260815-065707
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-065709 | write_apply,write_patch_oracle | none | 135s | 24 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B832 production-positive. The implementation and regression tests are correct, all eight executed checks passed, changed-symbol coverage is verified, and the terminal verdict is `verified/all_batches_verified`. Soft PatchEffect warnings no longer become unclosable verification targets. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-065709 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 207s | 32 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail-system | B831's exact semantic relation-only boundary reached the final prompt and the deterministic projection remained correct, but the prompt carried only generic CPU-topology semantics, not the concrete typed tuple `worker-200 CPU2 -> app-100 target CPU1`. The model therefore invented same-CPU occupancy and split the 0.800ms runnable delay into 0.400ms worker work + 0.400ms scheduler response. The system appendix published the correct cross-CPU tuple only after generation. B833 moves the shared exact tuple authority before synthesis; it does not rewrite the answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B832 is closed by production replay. The write controller reached `verified`; eight checks passed and no advisory was promoted into an impact target or repair loop.
2. B831 is data- and teaching-positive but not sufficient for the whole answer. The final prompt explicitly said a relation-only semantic span has zero effective attribution and cannot prove target wait/completion/direct blocking. The model still invented a different mechanism: worker-200 allegedly occupied CPU1 after wakeup and caused half of the target's runnable delay.
3. The raw deterministic edge already contains `waker_cpu=2`, `wakee_target_cpu=1`, `cpu_relation=cross_cpu`; the final system appendix renders the same fact correctly. The gap is handoff timing and prioritization: concrete topology was not in the pre-generation final decision tail, so a generic rule had no exact tuple to bind to.
4. B833 uses one typed compiler for both pre-generation guidance and the post-answer fact appendix. Hard deterministic edge rows are deduplicated, capped and consistency-checked. Cross-CPU forbids same-CPU occupancy/preemption/direct-competition attribution; same-CPU proves placement only and still requires compatible running/runnable overlap. Missing/unknown/inconsistent relations remain unknown.
5. This remains a model-owned answer architecture. No request/model/final prose scanning, verdict gate, entity inference, answer replacement, system-authored diagnosis, diagram or optimization conclusion is introduced. Explicit windows, Trace causal projection, automatic supplementation, typed on-chain-only root causes, actual-occupancy/business-clue versus rule-priced-eliminable axes, and support-only background all remain intact.
6. Both cases produced complete answers. No active stream was degraded because a complete answer was absent at 4ms, 4s, 4m, or another fixed cumulative age.
