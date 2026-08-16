# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T17:42:31Z
- sweep_start_ts: 20260816-104230
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260816-104232 | write_apply,answer_regex | none | 159s | 25 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | B918 production-positive. The planner used `insert_at_eof`; all three regression tests are top-level siblings, not nested under the existing test, and the production guard edit is correct. The machine FAIL is the existing honest boundary: the fixture exposes only source-static verification, so the controller refuses to call it behavior-verified. Keep that downgrade; do not make the eval pass by lowering proof caliber. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260816-104232 | answer_regex,answer_contains,mermaid_edge_count | none | 409s | 38 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=7/1,fin_reject=3,unavail=0,prune=0 | partial | B917/stage authority production-positive: the accepted analysis carries all six typed participants; final context contains four checkout-verified stage_binding rows, three precedence recipes, nine carrier fields, and BusContext→Mutable no-arrow ownership. Final diagram preserves all four stages and the three proved order edges, keeps BusContext/Mutable visibly grouped, and discloses their requested directed relation as unproven instead of inventing one. Remaining quality debt: 3 finalizer patch rejects to make unproven participants visibly exact; prose broadly says all agents read/write shared state even though the diagram correctly limits directed incidence; the automatic dimension supplement still exposes internal wording. Treat as soft context/authoring follow-up, not a final-prose keyword hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
