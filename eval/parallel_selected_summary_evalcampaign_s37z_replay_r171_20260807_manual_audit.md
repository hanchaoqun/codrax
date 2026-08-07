# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T15:23:23Z
- sweep_start_ts: 20260807-082322
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260807-082323 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 165s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit selected window, five typed queries, wakeup path, ranked seats, representative windows, deterministic supplement, and both actual-occupancy/existing-rule-eliminable axes were preserved. The new `absent` semantics reached the final prompt and the model no longer said that no drop occurred. It nevertheless inferred an unproved VSync-deadline miss, summed same-direction seats (#1+#2 and #3+#5+#6+#7) despite explicit `direction_subtotal_authority=not_provided`, and labelled uncalibrated pressure values medium despite `aggregate_absolute_level_authority=not_provided`. These are repeated model-calibration violations after precise tail guidance, not justification for prose scanning or system-authored replacement. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-082323 | answer_regex,answer_contains | none | 282s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=5/0,fin_reject=1,unavail=0,prune=0 | partial | Line-coordinate frontier sampling reached Normalize/Compile/Plan/Bind and the final answer correctly rendered `buildAnalysisIR -> gate.RunWith <- gate.Run`, with no invented source-to-requested-sink path. One avoidable finalizer retry remained: `buildAnalysisIR` was already a Mermaid participant and endpoint of a typed, evidence-validated edge, but the mechanism-anchor checker saw only the alias `IR` and forced a redundant ordered-list item. General fix: resolve only typed edge endpoint aliases through parsed node declarations; never scan sequence message text. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
