# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T12:46:31Z
- sweep_start_ts: 20260807-054630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-054632 | answer_regex,answer_contains | none | 153s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Analyzer emitted the contradictory typed shape `source=buildAnalysisIR, sink_mode=discover, sink=""` while `exact_targets=[buildAnalysisIR, gate.Run]` named one unique other endpoint. Exact reachability never activated. Explorer therefore stopped at sibling `gate.RunWith`, omitted `analyzerGraphForNormalize`, and finalizer was forced to preserve the requested `gate.Run` label without path authority. It invented the false statement “RunWith routes to Run”; source proves the reverse (`gate.Run -> RunWith`). The exact diagram capsule repaired the second diagram reject, but cannot repair the model-owned false conclusion. New `EVAL-B269-NAMEDENDPOINTDISCOVER1` P0/P1; B262 remains open. |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-054632 | answer_regex | none | 160s | 21 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Explorer emitted `collect_files -> walk @ src/walker.rs:6` in its first evidence batch, so the final answer now preserves the full main→run→collect_files→walk and run→index_file→is_match structure and no longer mislabels the sequential phases as parallel. B265's production outcome is positive, although its new completion downgrade did not need to fire in this sample. One optional diagram was removed after it drew an unproved `walk -> run` edge; prose/list remained correct. Two walk rows lost inline citation refs but the exact cited lines remain in the citation ledger, so this is a minor presentation observation rather than a wrong answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Rust: human-pass; the missing bare parser edge is present and the earlier sequential-as-parallel error did not recur. `EVAL-B265` remains implemented/full-tests-pass with a positive production outcome, but the precise downgrade branch itself is not yet production-witnessed.
- Go sequence: human-fail. `EVAL-B269-NAMEDENDPOINTDISCOVER1` is the highest-ROI root cause: a unique typed named destination was silently demoted to discover-empty, disabling exact path/no-path handling. `EVAL-B262-TRANSITIVEHOPDEPTH1` also remains open because `analyzerGraphForNormalize` was omitted again.
- Diagram repair: the S37r exact capsule worked after the second reject, but evidence repair cannot substitute for a false model conclusion and did not do so.
- Neither case attached a runtime artifact; no claim about Trace causal-projection behavior is inferred from this batch.
