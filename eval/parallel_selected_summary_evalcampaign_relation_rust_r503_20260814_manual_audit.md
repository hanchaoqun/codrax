# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T18:30:26Z
- sweep_start_ts: 20260814-113024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260814-113026 | write_apply,answer_regex | none | 226s | 24 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-safe / implementation unverified | The product correctly refused `all_verified`: `make check` ran only the Python source-shape oracle, so all three changed Rust paths remained `source_static`. The patch has the requested checked arithmetic, but the new `#[test]` items were inserted inside the preceding test function; the static oracle still passed and no Rust test executed. This is a repeated B774 fixture/runtime-proof gap, not a reason to relax the production verification caliber. |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260814-113026 | answer_regex,answer_contains | none | 228s | 30 | read=12,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Investigation found all 12 production implementers and the first draft authored 12 correct implementer-to-interface edges with matching `type_relation` anchors. The Mermaid normalizer/parser split compact inline link labels (`-.implements.->`, then `--implements-->`) into synthetic `codraxNode1` endpoints, causing two contradictory relation-gate retries. The third patch deleted every edge to pass, leaving a node inventory rather than the requested relationship diagram. This is deterministic `B819-MERMAIDINLINELABELTOPOLOGY1`, not model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
