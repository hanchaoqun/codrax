# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T00:03:11Z
- sweep_start_ts: 20260815-170309
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260815-170311 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | partial | AK3 production positive closed: projection tree, key-metrics table, lossless detail and legend now distinguish `4组CPU汇总(组和...;单段...)` from the typed 11 D-state occurrences. Explicit window, causal projection, deterministic supplement, on-chain root-cause scope, occupancy/business clues and rule-based eliminable axes all survive. The model-authored prose nevertheless misreads four per-CPU group sums as four waits, treats a kernel callsite as a proved fence object/mechanism, and adds cross-direction eliminable quantities despite precise typed guidance. This is model noncompliance/fluctuation over sufficient context; do not add a prose hard gate or system-authored conclusion. One rejected analyzer `grep` was recovered and did not affect evidence. |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260815-170311 | write_apply,answer_regex | none | 277s | 25 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | Applied patch and ChangePlan agree with the source oracle, and the sole `make check` source-static verifier passed once. This fixture has no Rust compiler/behavior runner on the host, so the deterministic source-static boundary correctly finished as `unverified` instead of fabricating runtime confidence; machine FAIL is expected verification honesty, not a regression. The repeated-verifier B856 failure did not recur and the B864 misleading-plan witness was not reproduced. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
