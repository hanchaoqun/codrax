# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T23:23:38Z
- sweep_start_ts: 20260807-162336
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260807-162338 | write_apply,write_patch_oracle,answer_contains | none | 116s | 21 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `main.go` only changes `retrun` to `return`; both compile probe and project `go test` pass, changed-path coverage is `project_runner`, and finish follows typed `batch_verified`. No replan/resume/stamp lane was entered. Analyzer spent one correction on an incorrect diagnostic classification, but it did not alter the plan or verification authority. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-162338 | answer_regex,answer_contains | none | 134s | 22 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | S37bc is production-positive: the final answer cites the factory return at `src/registry.cpp:18`, preserves the `kind == "console"` selector, and no longer misstates `flush` as unconditional. The one finalizer reject is legitimate: five unproved diagram arrows were reduced to the two typed call edges. However the answer still omits the entry guard at line 30 and the error-only `flush` branch at lines 37-38, because exploration saw but did not emit those control facts. File as a generalized typed call-control completeness gap, not an output-keyword gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
