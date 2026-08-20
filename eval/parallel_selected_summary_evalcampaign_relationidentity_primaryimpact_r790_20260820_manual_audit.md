# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T23:46:04Z
- sweep_start_ts: 20260820-164603
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-164604 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B1264 production positive: explicit 2.000..2.020s window, auto-supplement, four-hop chain and Trace causal projection survive. The 17/14ms sleep observations stay state-only, while the three independent 1ms scheduling candidates remain separate ranked rows; 11ms threadpool IO remains #1. One retry was only a model-omitted block id, not a contradictory contract or degraded fallback. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-164604 | answer_regex,answer_contains | none | 473s | 44 | read=7,repo_map=3,list=0,trace=0,source_lens=1 | midloop=13,inv=4/0,fin_reject=10,unavail=0,prune=0 | fail | B1265 production positive but incomplete overall. The final three model-authored precedence edges retained their visible nodes/labels and uniquely received canonical typed identities before lease validation; the old `unlisted_relation_added` loop did not recur. However the first draft invented dispatcher/state-write arrows, and ten rejected patches followed. Rejected merged patches silently advanced the hidden patch base inside the same dispatch, so later exact-occurrence edits targeted a state the model could not inspect (new B1266/P0). Final prose also incorrectly says only analyze uses the model and all later stages are deterministic despite typed AgentExplorer/AgentExtractor/AgentFinalizer context (new B1267/P1 context-authority boundary). Final diagram and table are usable, but the summary is factually wrong. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
