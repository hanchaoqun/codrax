# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T06:57:40Z
- sweep_start_ts: 20260731-235739
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260731-235740 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 104s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | User-first ordering and narrow-report boundary are correct: no causal projection was emitted, and the deterministic data block contains all 3 complete intervals totaling 0.635ms. The principal prose nevertheless combines the full-artifact total with only the 2-row exploratory-window roster, omits the third interval from the requested answer, and calls typed `io_wait` with non-IO D-state=0 “D 状态”. This is a typed scope-convergence gap, not a reason to restore a front-loaded authority block. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260731-235740 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 174s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit-window capabilities are intact: projection/root rank/wakeup/eliminable/representative windows/coverage/system supplement all published, and the readable frequency/state data follows the decision surfaces with no “系统权威” protocol leak. The model still treats the out-of-window 34579.595130 VSync as the end of the 34579.472865..34579.587805 window despite typed `frame_causality=unproven/frame_evidence_status=absent`. The actual-occupancy axis also remains only an auxiliary “未计价占用 2行/最大44.836ms” summary without subjects or an independent decision surface. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
