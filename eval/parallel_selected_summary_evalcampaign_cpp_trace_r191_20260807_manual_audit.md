# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T23:09:35Z
- sweep_start_ts: 20260807-160933
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-160935 | answer_regex,answer_contains | none | 90s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | S37bb fired correctly: the collapsed conditional/call row was rejected and the explorer re-emitted line-37 guard plus line-38 call. The finalizer also received the line-18 concrete return, but both remained enrichment-only while the principal lane/citation repair favored call sites. Final text consequently says flush happens immediately after every write, misstates the guard as deciding output, and cites line 32 for the factory return. Runner oracle missed all three. Diagram rejection was valid; removing the optional diagram did not cause these prose errors. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260807-160935 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | Explicit 34579.472865..34579.587805 window survived both trace_query calls. Final answer preserves frame causality=unproven/absent, actual-time and rule-eliminable axes, ranked candidates, four-node wakeup path, representative windows, four-state accounting, compute-supply and IO bounds, plus one system causal projection block. No source evidence or answer rewrite path interfered. Advisories: internal phrase “prompt 状态” leaked; CPU/IO pressure scores still lack a user-comparable calibration; attached_trace/file-name aliases repeat the same physical observations (existing B316). |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner: 2/2 PASS. Human: 1/2 PASS.
- `EVAL-B319` is process-positive but not closed: typed input is now correct, while principal support-lane membership and citation ownership still hide the exact guard/return behind enrichment.
- New high-ROI continuation: promote only parser/grounding-backed guard facts owned by a principal caller and only connected typed factory selection facts into the existing call-chain support lane. Do not trust evidence summary, infer control from adjacency, scan answer text, or rewrite the model conclusion.
- Trace isolation is production-positive. File `EVAL-B322` for internal prompt-state wording and `EVAL-B323` for typed pressure calibration as P2; keep `EVAL-B316` open for exact physical-artifact identity.
