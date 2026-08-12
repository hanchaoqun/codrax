# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T06:49:51Z
- sweep_start_ts: 20260811-234950
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260811-234951 | write_apply,answer_regex | none | 186s | 23 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Product behavior is correct and fail-loud despite the runner FAIL. The single production patch changes raw environment-string truthiness to exact `=== 'true' || === 'error'`, leaves the existing regression expectations untouched, and `make check` passes. The final workflow does not sign an empty or weak verification domain: it reports `production_verification_source_static_only` because the fixture's Make target executes only the Python source/static oracle and never runs the TypeScript behavioral test under Node/ts-node. No replan occurred, so cumulative replan verification was not exercised. This is an eval-fixture/runtime-capability limitation, not a reason to weaken product verification or falsely mark `all_verified`. |
| 1 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-234951 | trace_attachment,answer_regex | perf_triage+trace_query | 414s | 49 | read=16,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | B612b is production-positive for source depth but not yet end-to-end correct: Explorer followed the wrapper into the actual B push/E LIFO-pop and duration body instead of stopping at the entry line. The flat evidence handoff still lost branch topology, so Finalizer linearized mutually exclusive exact-marker and generic `EventTraceMark` decode paths. It also rewrote the raw unnamed end `E|2000` as `E|2000|H:RenderService:DoFrame`, omitted the physical-source+emitter-TID sync key/lifecycle fail-closed rules, invented a 60fps/16.67ms budget, and inferred continuous On-CPU execution merely because this two-row artifact lacks scheduler events. The last inference is a deterministic authority gap: absence of scheduler rows means residency is unavailable, not running. B619 succeeds only at the weaker level that 86.111ms remains a performance-investigation clue while jank is unproven. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
