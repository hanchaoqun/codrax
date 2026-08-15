# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T21:08:40Z
- sweep_start_ts: 20260815-140838
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260815-140840 | primary_answer | none | 146s | 28 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | Typed evidence proves all five requested direct calls and the terminal `AuditLog.record -> System.out.println` body call. The answer preserves the main chain, but calls it five hops while omitting `ClinicConfig.resolveMaxVisits` as its own list row and does not state the required negative durability boundary for stdout. The first diagram invented a guard edge and lacked exact edge authority; the retry used the system copy-ready graph, whose true edges were ordered terminal-first because evidence slice order leaked into sequence display order. This is a system presentation-order gap, not permission to invent or delete relations. |
| 2 | github_issue_chrono_duration_min | FAIL | eval/results/github_issue_chrono_duration_min-20260815-140840 | write_apply,write_patch_oracle | none | 388s | 24 | read=10,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | The final Rust change is correct: it adds `try_milliseconds`, delegates the panicking constructor, preserves MIN/MAX direct construction, and adds tests. The first insertion put top-level tests inside another test; `make check` caught it and replan repaired the braces. The fixture has no Cargo project/native Rust runner, so `verification_proof_incomplete` is honest and must not be weakened. Nineteen controller dispatches and repeated identical source-static checks after the available verification surface was exhausted expose a separate scheduling-efficiency gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
