# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T17:29:49Z
- sweep_start_ts: 20260816-102947
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260816-102949 | write_apply,answer_regex | none | 156s | 25 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Production guard change is correct, and the verifier honestly downgraded source-static evidence to unverified. The test edit is nevertheless structurally wrong: three sibling `test(...)` blocks were inserted immediately after an existing test's opening line, nesting them inside that test. Global delimiters remain balanced, so the current placement guidance did not prevent an invalid test topology. General fix: teach all brace-language structured edits that inserting after an opening-scope anchor enters that body; sibling declarations/cases must anchor before the header or after its matching close. Keep this soft/model-owned; do not hard-gate on test names or request prose. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260816-102949 | answer_regex,answer_contains,mermaid_edge_count | none | 287s | 32 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=2,unavail=1,prune=0 | partial | Text explains all four stages plus BusContext/Mutable, but the required architecture diagram was reduced to the two grounded generic dispatch calls and lost the requested stage/carrier topology. The literal `finalizer` oracle is also narrower than the semantically correct `finalize`/`answerDocumentEvaluator`, but that does not erase the human gap. Root cause: analyzer emitted a typed relation_scope_quote containing six actors while leaving participants empty; the existing consistency check only inspected the shorter diagram-dimension quote, so checkout-verified stage binding/precedence/carrier authority never activated. B915 is production-positive here: flow completion fell from the historical ~90s to 1.106s first use and ~12ms warm uses. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
