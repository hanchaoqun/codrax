# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T23:04:24Z
- sweep_start_ts: 20260805-160423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260805-160424 | write_apply,write_patch_oracle,answer_contains | none | 115s | 19 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | analyzer=1,planner_probe_reject=1,run_tests=1 | pass | Full write path is real: one-line structured patch changes only `retrun buf;` to `return buf;` inside an isolated worktree; deterministic verification selects `make test`, executes compilation and two binary runs, and reports 1/1 passed. The first write-analyzer tool call was truncated malformed JSON and recovered on the next iteration; the first planner draft also emitted a forbidden shell verification probe despite the prompt explicitly saying C/C++ must omit it, then removed it after a same-direction typed enum error. Both are contained carrier/model variance, not contradictory teaching or a false verified state. Main repository remains clean. |
| 1 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260805-160424 | log_regex,answer_regex | none | 207s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=9,repair=1,failed_actions=2 | pass | Final bytes are exactly `30`; required audit semantics remain intact after the new shape teaching: rule coverage=1, decisions=8, entity resolutions=6, Alpha contributions=2 (20+10), reconcile=pass. The system did not over-simplify this explicitly audited scalar task into a direct transform. Two typed planning failures (diagnostic-child alias used as a record input, and compute crossing the current DAG stage) were caught before execution and repaired; 9 rounds/207s is an efficiency P2, but no wrong ledger or answer was published. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
