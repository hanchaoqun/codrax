# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T20:28:52Z
- sweep_start_ts: 20260807-132851
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260807-132852 | answer_regex,answer_contains | none | 120s | 23 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Required generic flow rendered on the first finalizer emit with the four main stages, responsibilities, citations, and no reject. Extra Request/FinalAnswer/preStages nodes are grounded contextual boundaries. This is the negative control: source-call body ownership did not incorrectly hard-gate an ordinary mechanism flow. The system supplement repeats some stage detail but is directly responsive and explicitly marked as a non-replacement audit table. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-132852 | answer_regex,answer_contains | none | 136s | 21 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | Parser/type/call/return facts were present and the validator correctly rejected the invented Sink -> ConsoleSink call plus class-only aliases; patch removed only the optional diagram. The final prose preserves the missing Logger-construction bridge caveat, but still calls the overall path “two polymorphic dispatches” and overstates stdio as direct disk output. One member_set support-ref repair also collapsed qualified C++ members to repeated short names. Root system gap: discover_path did not receive the existing typed relation/dynamic-dispatch composition capsule because publication was gated on DiscoverSinkActive only. No unlabeled flowchart was emitted, so the production witness remains not-exercised. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
