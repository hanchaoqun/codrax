# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T22:40:23Z
- sweep_start_ts: 20260818-154021
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260818-154023 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 222s | 48 | read=4,repo_map=0,list=0,trace=5,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1113 production-positive: explicit 233.190ms window, ranked on-chain root causes, actual-occupancy/rule-eliminable axes, business spans and Trace causal projection all survive; the system-owned footer renders `枚举未完整` and no longer leaks `enumeration_status=incomplete`. However the model-authored kernel-wait table conflates 11 D-state intervals/36.757ms with the distinct 12-row blocked_reason census/39.157ms and invents a 2.400ms remainder despite the typed footer explicitly forbidding that substitution. Model prose also copies `holder_authority=not_provided`. No finalizer reject and no fixed-4ms degradation. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-154023 | answer_regex,answer_contains,mermaid_edge_count | none | 298s | 33 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | Legal Mermaid is present, but all requested stage↔BusContext/Mutable flow edges disappear after the first draft's unsupported edges are correctly rejected. Explorer proves `o.busCtx -> BuildAgentContext` and `BuildAgentContext -> bus.Mutable.Objective`, then low-delta convergence closes while the requested stage component remains disconnected. Prose still states Explorer/Extractor/Finalizer carrier flow, so prose and graph authority diverge. Root cause is upstream recovery: the typed blocker ignored advancing parser frontiers and reverse caller navigation skipped the un-emitted request-owned sibling argument (`types.AgentExtractor`) at the same call site. Validator behavior is correct. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

1. `B1113-TRACECOVERAGEAUTHORITYREADER1` is production-closed by r707: machine wire values remain typed internally, while the reader-facing coverage boundary is localized and natural. Trace projection, explicit-window authority and deterministic supplements were not weakened.
2. The Trace answer's 11/36.757ms versus 12/39.157ms error reproduces the already-filed B933 model-compliance family. The final typed context already presents both observation domains and the non-substitution rule accurately; do not add a raw-answer keyword gate or let the system rewrite the model's conclusion. Retain as an eval/manual-audit failure and continue heterogeneous replay.
3. New `B1114-REQUIREDFLOWFRONTIER1/P1`: required-flow participant recovery treated the stable missing-participant set as no progress even while exact parser navigation advanced. After grounding a callee-body operation it searched only arguments belonging to still-missing participants, so it skipped a different requested participant carried as a sibling enum/constant argument at the same call.
4. B1114's repair must remain navigation-only: include the exact parser frontier in convergence identity, revisit the unique caller for un-emitted request-owned sibling arguments, and require a model-authored grounded operation before any coverage/diagram edge closes. No user/model prose scan and no system-authored relation are permitted.
