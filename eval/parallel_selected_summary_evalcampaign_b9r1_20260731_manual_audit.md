# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T17:40:36Z
- sweep_start_ts: 20260731-104034
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260731-104036 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 110s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Four independent B/E spans were ordered correctly, but the answer promoted temporal adjacency into a confirmed UI→RS→GPU cross-thread flow. The engine's FrameFlowEdge had no relation/causal authority. Filed EVAL-B9-T1 and fixed in Batch U. |
| 2 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-104036 | trace_attachment,answer_regex | perf_triage+trace_query | 302s | 49 | read=5,repo_map=2,list=0,trace=3,source_lens=2 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=2 | partial | Runtime duration 86.111ms and current-source mechanism were both correct and artifact/source line namespaces stayed visibly distinct. However the scalar block attached the runtime value to query.go:20636, which proves implementation mechanism but not that trace measurement. Filed EVAL-B9-C1. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `EVAL-B9-T1/P1`: `buildFrameTimelineFromPipeline` connected every adjacent
  time-sorted complete span and named it `frame_flow`, but the edge carried no
  typed connector kind or causal conclusion. The accepted answer therefore
  called B/E-only adjacency a confirmed cross-thread flow with no missing hop.
  This is engine authority leakage, not model variance.
- `EVAL-B9-C1/P1`: the analyzer correctly emitted an active
  `artifact_value_profile` for the 86.111ms runtime scalar. The model's scalar
  block declared `external_observation` but still cited a current-source line.
  Existing cleanup intentionally preserved all current-source citations for a
  mixed artifact+source request, with no item-level origin-alignment pass.
- The combo answer's source mechanism citations are otherwise useful and must
  remain: the correction must detach only the source citation from the typed
  artifact scalar, not remove the current-source explanation lane.
