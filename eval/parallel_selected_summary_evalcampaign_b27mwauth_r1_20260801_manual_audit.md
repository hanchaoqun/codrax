# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T23:17:07Z
- sweep_start_ts: 20260801-161706
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-161707 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 137s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B27 missing_wakeup shape was not observed in this replay. ALIAS remains correct (one projection family). The finalizer received impact_phase=pre_wakeup_dependency and explicit no-holder/no-post-wakeup authority, but the model still promoted priority-inversion candidates to the root cause and described a pre-wakeup dependency as limiting post-wakeup scheduling. Typed overlap/non-additivity exists in the system projection but is not salient enough in the decision handoff. |
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-161707 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 174s | 38 | read=2,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass_with_advisory | Final values are correct: D-state=0, three io_wait rows, 0.635ms total. Two earlier drafts were rejected because hierarchy sorting moved a model caveat across a model-owned repaired carrier (`model block 2 changed`). OWN4 correctly caught a real system mutation; EVAL-B27-OWNGUARD1 fixes the sorter rather than weakening the guard. The model also over-interpreted caller `sync_buffer_read_wi` as a physical wait-object/block-device conclusion; tracked by CALLER1. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
