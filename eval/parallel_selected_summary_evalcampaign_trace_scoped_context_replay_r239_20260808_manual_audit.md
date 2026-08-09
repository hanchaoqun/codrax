# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T04:14:11Z
- sweep_start_ts: 20260808-211410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-211411 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Ranked root seats remain typed/on-chain; unbound caller census is absent from Finalizer/root prose; no cross-seat or repair-lane sum is emitted. Background IO remains explicitly non-root support. |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-211411 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Compact authority arrives, but first emit copies three typed temporal arrows without edge_anchors and needs one patch. Final prose still upgrades item-stage/span labels to owning-thread roles and unproved UI submission/GPU work. Earlier perf/analyze/explore summaries already promote the requested proposition into facts, so this is a cross-stage authority leak rather than a Finalizer-only wording fluctuation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
