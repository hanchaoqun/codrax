# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T23:41:17Z
- sweep_start_ts: 20260801-164115
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-164117 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 114s | 31 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact D-state=0, io_wait=3, total=0.635ms and all three intervals are correct. No finalizer rejection. The model explicitly says sched_blocked_reason caller is a kernel wait call-site and not a lock/resource holder, so CALLER1 is covered on the bounded-fact lane. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-164117 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 230s | 45 | read=7,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | REL1 improved: the model states 7.386ms io_wait is already included in the 10.433ms D/IO parent and does not sum D/io_wait/io_latency. PHASE1 remains wrong: it calls pre-wakeup dependencies holders/CPU competitors and interprets sleep as post-wakeup churn despite the exact phase handoff. New system gap B28-SHARD1: observation coverage summed six heavily overlapping resource-pressure query windows into 14.204ms; the model repeated this as block_rq cumulative. The arithmetic advisory later flagged it, but the bad aggregate originated in system context. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
