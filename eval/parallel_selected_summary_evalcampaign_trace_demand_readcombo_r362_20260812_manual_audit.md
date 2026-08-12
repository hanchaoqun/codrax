# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T04:50:58Z
- sweep_start_ts: 20260811-215057
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-215059 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B605 production positive: final keeps 10.331ms compute-delivery deficit as a secondary positive seat, makes no 17.609/44.082ms cross-seat sums, and leaves demand-vs-supply judgment to the model. Explicit window, typed wakeup chain, on-chain rank, occupancy/eliminable axes, VerifyClass clue, background demotion, causal boundary and auto-supplement all remain. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-215059 | trace_attachment,answer_regex | perf_triage+trace_query | 288s | 37 | read=2,repo_map=1,list=0,trace=2,source_lens=1 | midloop=5,inv=3/1,fin_reject=0,unavail=1,prune=0 | fail | B604 residual: Explorer again cites defaults.go:1203 with anchor_kind=text_reference as a parser mechanism; the ClaimForm boundary never reaches same-turn emit feedback, accepted-evidence handoff or Relation Role Handoff. It finds ExactTraceMark conversion but misses findSpanWindowsCompacted pairing and overstates adjacent B/E pairing. Perf triage also imports an untyped 60fps/16.7ms baseline and 5.16x ratio although no refresh/deadline carrier exists; preserve as B608 soft-context/model-fluctuation watch, never prose-scan harden. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
