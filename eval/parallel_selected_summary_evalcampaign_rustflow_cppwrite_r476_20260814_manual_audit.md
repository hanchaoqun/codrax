# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T08:32:30Z
- sweep_start_ts: 20260814-013229
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260814-013230 | answer_regex | none | 220s | 25 | read=3,repo_map=4,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | The answer retained the principal calls and explained walker's file-discovery role, but it called sibling `run` call sites “parallel” without concurrency evidence. Its sequence diagram then placed `collect_files -> walk` after `index_file -> Matcher.is_match`, although the system carrier proved only call topology, not that temporal order. It also described RegexLikeMatcher as ordinary regex and cited only LiteralMatcher's declaration for the combined implementation row. Precise prompt boundaries already prohibited concurrency/order inference; B776 removes the system's optional non-linear copy-ready sequence skeleton so typed topology no longer supplies a temporal-looking repair artifact. |
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260814-013231 | write_apply,answer_regex | none | 406s | 25 | read=7,repo_map=4,list=0,trace=0,source_lens=2 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Native C++ `make check` correctly rejected the first plan: widening only the addition still truncated at `render_year(int)`. The controller preserved the failed verification, replanned, widened both the intermediate and `render_year` parameter to `long long`, applied a second checkpoint, and reran `make check`; compilation with `-Wall -Wextra` plus both ordinary and `INT_MAX` behavior passed. Final changed-path authority is `covered/project_runner/target_behavior`; tests were not edited. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner: 2/2 PASS. Human: one pass, one partial.
- C++ write is a healthy replan witness: the first plausible but incomplete fix failed real runtime behavior, the failure was not papered over, and the cumulative two-plan patch is correct. Reporting two recovery refs is expected because both checkpoint commits are needed.
- Rust's call identities/directions were mostly preserved, but call topology was overread as concurrency and temporal order. The initial prompt already said sibling calls are not concurrent and that topology proves no temporal order, so no answer-prose scanner or rejection gate is justified.
- Registered and implemented `B776-OPTIONALSEQUENCETOPOLOGY1`: when a generic call-chain default has no explicit diagram contract and its auto-selected optional sequence sits over a non-linear exact call graph, system-authored copy-ready sequence/component skeletons are withheld. Exact per-edge recipes remain; soft all-language guidance tells the model to use call-DAG/flow/prose unless separate precedence/control-flow authority exists. Explicit required/preferred diagram contracts remain unchanged.
- The Rust explorer made four completion calls; three returned typed `DOWNGRADED` repair outcomes, but runner metrics show `investigation_complete_rejects=0` because downgraded is not a transport reject. Registered `B777-COMPLETIONDOWNGRADEMETRIC1/P2-observability`: expose downgraded completion count/reason separately so retry-cost audits do not misread it as zero churn. This is not required for B776 correctness.
- No system-authored answer, relation, edge, node, label, or conclusion was added. No raw request/model/final prose hard gate, Trace causal change, fixed-age answer fallback, or active-stream-at-4ms degradation occurred.
