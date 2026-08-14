# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T02:23:57Z
- sweep_start_ts: 20260813-192356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-192357 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 146s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B758 production-positive: no root board/rank/full causal projection. Product failure remains: model-authored advisory member sets were replayed beside stricter typed frequency authority; the model treated min=558000 as the upper cap, overclaimed target binding, overinterpreted 16.358ms blocked census as 70.338ms sleep cause, and printed 70.3% instead of 30.2%. Runner D-state miss is secondary. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-192357 | answer_regex,answer_contains,mermaid_edge_count | none | 590s | 37 | read=34,repo_map=4,list=0,trace=0,source_lens=0 | midloop=12,inv=6/1,fin_reject=1,unavail=0,prune=0 | partial | Final answer retained the four stage precedence edges and three exact data-flow recipes. BusContext/Mutable remained disconnected with explicit unproven boundaries because no accepted request-scoped incident operation proved them. Correctly avoids fabrication, but does not fully answer the requested carrier flow. 590s/34 reads is excessive; analyzer spent four schema attempts and exploration repeatedly reopened already-oriented files. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B758 did what it was designed to do. H4 used three trace queries rather than the previous broad root-cause sweep; the finalizer prompt had no Runtime Trace Root-Cause Board, rank arithmetic, root seat, or fix-direction conclusion, and the visible answer did not receive a system-authored Trace causal projection.
2. The remaining H4 error is an authority collision, not a missing engine value. The prompt's direct typed lane stated `policy_limit_status=present` and `target_binding_status=unproven...`, with CPU4 `min=558000,max=2100000`. Earlier model-authored `system_inference` member sets were nevertheless replayed verbatim and claimed that 558000 was the cap / 26.6% ceiling. High-salience stale synthesis overrode the more precise later lane.
3. The same answer also exceeded evidence caliber: a 50-row blocked-reason census totaling 16.358ms cannot account for 70.338ms of S-sleep, and those caller labels do not by themselves prove network/filesystem causation. The summary's 70.3% is an arithmetic transcription error; the exact partition says 30.2%.
4. The automated H4 failure was triggered by its D-state principal regex even though the table visibly contains `D-State | 0.000 ms`; that oracle is secondary and must not be widened before product correctness is repaired.
5. QF's single finalizer rejection was legitimate: the first diagram drew unproved participant/assignment edges. The patch retained only typed precedence/data-flow recipes and left BusContext/Mutable disconnected. This is evidence-limited partial correctness, not permission for the system to manufacture the requested bridge.
6. Neither run produced malformed-JSON recovery, empty-answer recovery, stale-draft degradation, or an active-byte-stream 4ms age downgrade.
