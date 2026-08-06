# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T06:25:24Z
- sweep_start_ts: 20260805-232522
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-232524 | log_regex,answer_regex | none | 34s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Read-only plan executed four bounded commands. Final values match typed command payloads: macOS 26.5.2 (25F84), 18/18 CPU cores, 128 GiB memory, Apple M5 Max / 40 GPU cores, Metal 4. No JSON repair or unavailable tool event; native-array operation teaching held. |
| 1 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260805-232524 | write_apply,answer_regex | none | 361s | 20 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Protected-test post-image false positive is closed: plan applied in 361s. Planner first string-wrapped changes[] was losslessly repaired and one invalid line range was corrected; second emit omitted the invalid cross-language probe. Only `make check` ran a Python source-static oracle. The Rust patch still returns Some for i64::MIN under its lexicographic candidate guard and was never compiled because rustc/cargo are absent. Deterministic finalizer correctly kept the applied worktree but downgraded to unverified (`production_verification_source_static_only`); runner FAIL is honest, not a missing answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
