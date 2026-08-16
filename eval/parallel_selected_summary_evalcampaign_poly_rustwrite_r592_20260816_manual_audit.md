# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T22:43:07Z
- sweep_start_ts: 20260816-154305
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-154307 | answer_regex | none | 195s | 29 | read=3,repo_map=3,list=0,trace=0,source_lens=2 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | B932 is production-positive: the principal ordered list could not retain a relation claim with zero anchors, and its accepted form carried eight fully identified typed anchors. The optional sequence diagram was rejected twice for unsupported/mismatched topology and the model then removed it honestly. However, the visible answer still calls the list a complete cross-language chain while the exact registered-export bridge `_fastlex.tokenize_bytes` to `py.tokenize_bytes` is prose-only; the structured anchors cover ordinary calls and `m -> wrap_pyfunction!` registration but not every principal cross-component hop. Machine regex PASS therefore does not close relation completeness. Two final rejects plus two later dimension-display patches also remain excess churn. |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260816-154307 | write_apply,answer_regex | none | 672s | 25 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The final `unverified:production_verification_source_static_only` verdict is honest: `make check` runs a Python source-shape oracle, there is no rustc/project runtime, and the system did not promote it to Rust behavior proof. The retained patch is not mergeable without Rust verification: added tests call absent `checked_add` and refer to module constants as `Duration::MIN/MAX`; const compatibility is also unproved. A deeper contract gap caused the long replan: analyzer soft required contracts demanded `MIN = Duration::milliseconds(...)`, while the project verifier rejected that recursive form. A second plan repaired the shape, but its final retry omitted acceptance_tests and the controller resurrected the original contradictory contract snapshot. B943 filed and implemented as typed verify-failure contract-generation retirement; no failure-output or request prose scan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
