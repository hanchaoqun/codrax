# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T23:41:16Z
- sweep_start_ts: 20260810-164115
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260810-164116 | typed_inventory_rowset,dimension_substring,answer_contains | none | 113s | 23 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B483 direct positive: all 12 items copied exact prompt row_id and every citation/file/package is correct; no reject. One model-owned sentence says the 8 public classes exclude sealed/abstract while the same list includes Animal and Service. Typed rows were complete, so this residual is a visible self-contradiction/model fluctuation, not inventory extraction or row binding loss. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260810-164116 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Trace isolation passed: explicit 5.000..5.007 window, on-chain VerifyClass #1, runnable #2, 4.600ms eliminable, deterministic optimization point, frame-causality caveat, and background demotion all survive. Model nevertheless says the 5.000ms span fully overlaps/directly determines the 5.000ms sleep, while typed authority states overlap/effective=4.600ms and actual span=5.000400..5.005400 (0.400ms after wakeup). Context was precise; treat as model fluctuation unless repeated. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B483-ENUMROWID1`: closed by direct production witness. The finalizer copied all 12 exact row IDs and the system bound all 12 exact citations without changing visible member text.
- `B484-ENUMBUCKETPROSE1/P2-watch`: one model-owned bucket sentence contradicts the typed member roster by saying modifier variants are excluded while listing them. The prompt already marks every Principal Enumeration Row as qualified and authoritative. Do not add a prose-scanning hard gate; replay across other languages/buckets before deciding whether a short typed soft reminder has positive ROI.
- `B485-OVERLAPCALIBER1/P2-watch`: finalizer received exact `overlap=4.600ms`, `actual_span=5.000ms`, both occurrence intervals, and measured-vs-effective guidance, but still wrote “fully overlaps/directly determines 5.000ms”. This is not missing context or a projection error. Do not let the system rewrite the model conclusion; track recurrence and prefer soft caliber teaching/semantic review only if it generalizes.
- Trace structural invariants passed: non-chain supply pressure stayed background, the semantic span stayed on-chain and ranked, both actual occupancy and existing-rule eliminable impact were present, and frame causality remained unproven.
