# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T05:32:34Z
- sweep_start_ts: 20260811-223232
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-223234 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Explicit window, state partition, typed wakeup chain, on-chain/adjacent/background partition, positive 10.331ms compute-supply secondary seat, occupancy/eliminable axes, VerifyClass business clue, causal projection and deterministic supplement were all preserved. The model nevertheless added overlapping on-chain seats 23.994+19.041 into 43.035ms despite the same context saying `成员区间重叠,合计不可直加`, and upgraded priority-inversion candidates into proven downstream blocking/waiting. r363 passed under the same precise contract, so this remains model variance: no answer-prose hard gate, system rewrite, or conclusion takeover is authorized. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-223234 | trace_attachment,answer_regex | perf_triage+trace_query | 260s | 40 | read=5,repo_map=1,list=0,trace=3,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B610 worked: Analyzer emitted the active current-source profile, Explorer read current source, and the B609 ladder drove a bounded follow-up to `findSpanWindowsCompacted`. B608 also worked: the answer did not invent refresh rate, frame budget, deadline, or ratio, and kept 86.111ms as measured while jank stayed unproven. Mechanism correctness is still incomplete: mutually exclusive buffer-carrier and structured-field decode profiles were narrated as consecutive steps; the cited `query.go:10445` gutter did not contain the claimed source+emitter key, LIFO pop, lifecycle/malformed reset, provenance ambiguity, or duration-order fail-closed operations; and a raw recovery witness was blended into the normal query path without a selection/call anchor. Total pipeline time was 260s and the model carrier remained intact; there was no fixed-age degradation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized findings

- `B611-CURRENTSOURCEBRANCHTOPOLOGY1/P1-high`: source explanation needs a branch-aware mechanism shape. Decode profiles guarded by different predicates are alternatives, not an ordered pipeline; recovery/audit/fallback lanes cannot be promoted into the normal consumer without a typed call or selection anchor.
- `B612-CURRENTSOURCEOPERATIONGUTTER1/P1-high`: a function-entry citation does not prove operations later in the body. Correlation keys, stacks/queues, reset boundaries, malformed/order policies, and fail policy need focused source gutters containing those operations, or the layer must remain explicitly unvisited.
- These are soft evidence-navigation fixes only. They are language/path/event-family neutral, do not scan model prose, do not hard-reject a structurally valid answer, and do not authorize system-authored conclusions.
- Active-stream invariant: cumulative wall-clock age is not a precise failure signal. The 260s production run only proves total-pipeline preservation; adapter tests separately pin that real reasoning/content/tool progress may outlive the configured request timeout/old total cap. Degradation remains limited to typed first-byte absence, real byte stall, transport break, caller cancel, safety, or precise decode degeneration.
