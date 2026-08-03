# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T02:27:51Z
- sweep_start_ts: 20260802-192750
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260802-192751 | log_regex,answer_regex | none | 35s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Data lane consumed both `rules.md` and `orders.csv`, computed the authoritative single-line result `17.0`, and completed in one batch with no repair. The trivial rule does not require a contribution ledger; no repository or answer-prose authority was invented. |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-192751 | answer_regex | none | 139s | 21 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=8,unavail=0,prune=0 | fail | The model's first structured draft contained the five requested rows and grounded citations, but `normalizeCallChainReachabilityAuthority` mistook the visible-anchor pair `VisitController` / `VisitController.create` for source/sink and replaced the model summary and principal list with a false unproven verdict. That deletion then made the five-row member-set check report three rows missing on every retry, causing fallback. The draft also flattened side calls into a five-hop chain and contained the contradictory phrase `超限则放行`; typed edge context was sufficient, so those two are model-variance observations rather than reasons for a prose hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
