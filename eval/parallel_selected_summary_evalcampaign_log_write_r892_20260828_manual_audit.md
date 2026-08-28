# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T16:05:45Z
- sweep_start_ts: 20260828-090545
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260828-090545 | write_apply,write_patch_oracle,answer_contains | none | 86s | 26 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | The plan and applied tree change only `main.c:19`, from `retrun buf;` to `return buf;`. Auto Pilot preserves the single-file/single-line scope, runs the repository-owned `make test`, records exit 0, and delivers only `main.c`. Verification creates an untracked `main` build product in the retained worktree, but the worktree audit discloses it and the artifact is not included in the delivery ref. No risk, approval, fingerprint, apply, or verification gate is bypassed. |
| 1 | log_path_question_multi_runtime_files | PASS | eval/results/log_path_question_multi_runtime_files-20260828-090545 | answer_regex,answer_contains | none | 104s | 25 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | The two files remain separately headed and the error strings, four stack frames, source paths, and immediate top frames are reproduced correctly without reading repository source. However, the answer widens stack context into unsupported causality: `Get(0x0, …)` proves a nil receiver at that frame, not that `ServeHTTP` supplied it or that initialization/guarding was missing; `context deadline exceeded` at `Fetch` does not prove the incoming context was already expired, that the caller timeout was too short, or that a downstream service was slow. The closing caveat partially retracts those claims but cannot make the earlier assertions authoritative. The explicit “不分析代码” boundary also lost its typed `exclude` mode because the analyzer emitted a non-verbatim exclusion quote and the parser deliberately demoted it to default; no source read occurred in this run only because the remaining artifact path was sufficient. One precise member-set cardinality reject was repaired from 1 to 3; it was not a contradictory contract. Record B1386 for repairable explicit-exclusion carrier loss and B1387 for path-backed log causality calibration. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
