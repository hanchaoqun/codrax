# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T11:54:53Z
- sweep_start_ts: 20260805-045451
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260805-045453 | answer_regex,answer_contains | none | 141s | 20 | read=6,repo_map=4,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Alias definition, retry parameters, and the broad CLI→client→transport narrative are correct, but the answer labels four rows as the “complete call chain” while omitting the exact `ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce/fetch` call-site sequence and cites the CLI call for `client.fetchUser` twice. The final network operation is absent. Analyzer initially selected call_chain, failed its first endpoint-pair emit, and then downgraded to mechanism instead of repairing the ordered pair; the runner's name/substring oracle is therefore a false green for completeness. |
| 1 | mr_poly_binding_chain | FAIL | eval/results/mr_poly_binding_chain-20260805-045453 | answer_regex | none | 481s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=10,inv=6/1,fin_reject=6,unavail=0,prune=0 | fail | B105 admission repair worked: Analyzer retained `question_kind=call_chain` and the cross-language call-edge guide fired. The first grounded relationship rows omitted subject/predicate, however, and the decoder's `predicate=relationship` sentinel bypassed call normalization. Later corrected rows at the same source/line were swallowed as exact duplicates, leaving `grounded_callsite_facts=3` but `explicit_caller_callee_edges=0`. Completion eventually accepted an inaccurate no-path waiver after low-delta convergence; six correct diagram rejects then exhausted Finalizer into degraded output. The answer reverses the Rust core/wrapper roles, calls line 42 recursion although it is wrapper→core, emits unsupported guard/binding arrows, and leaks the final think. Filed as EVAL-B106-CALLEDGEID1. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B105-ENDPOINTDISC1`: implementation obtained a positive admission witness in the polyglot replay; close the admission sub-gap, while answer correctness remains blocked by the downstream directed-carrier gap below.
- `EVAL-B106-CALLEDGEID1`: confirmed product gap. A grounded parser/read-line call row could lose its directed carrier when sparse model fields decoded to compatibility placeholders; same-anchor corrections were then misclassified as no-op duplicates, and the call graph ignored the already system-stamped qualified owner. This is one cross-language identity/transport defect, not a PyO3-only issue.
- `EVAL-B106-TSCHAIN1`: runner false green / replay watch. The endpoint-pair retry was abandoned by the model and the answer omitted exact final hops. Do not infer direction from unordered entities or add a request/answer text gate; replay after the generic call-carrier repair before deciding whether another product change is justified.
- No Trace case ran in B106. No explicit-window, causal-projection, auto-supplement, or two-axis root-cause code changed.
