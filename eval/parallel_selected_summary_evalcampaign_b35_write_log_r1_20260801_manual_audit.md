# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T04:13:39Z
- sweep_start_ts: 20260801-211337
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | logtri_goroutine_dump | PASS | eval/results/logtri_goroutine_dump-20260801-211339 | log_attachment,answer_regex | log_triage | 73s | 18 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Triager emitted three synthetic peer errors although only one fatal header exists; final answer upgraded concurrent snapshots 87/120 into independent crashing writers and asserted a shared map/no lock without artifact proof. Runner regex only checks race/map vocabulary and false-greened. |
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260801-211339 | write_apply,write_patch_oracle,answer_contains | none | 85s | 19 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-line `retrun`→`return` patch, isolated apply and `go test -json ./...` all correct. `kind=patch` is an intentional capability-probe oracle, not a product-correctness requirement: an equivalent exact modify operation must not be treated as product failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
