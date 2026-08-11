# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T04:21:10Z
- sweep_start_ts: 20260810-212106
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260810-212110 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 186s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Explicit 114.940ms window and deterministic supplement survived. Model answer separates direct wakeup chain, ranked on-chain priority inversion, D/I/O, scheduling and frequency supply, deterministic `VerifyClass`, business clues, representative windows, and background. System projection preserves actual/effective values, eliminable overview, on-chain/adjacent/background lanes and `frame_causality=unproven`; one missing-summary patch was resolved without changing the conclusion. |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260810-212110 | answer_regex,answer_contains | none | 659s | 44 | read=13,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=5/0,fin_reject=20,unavail=0,prune=0 | fail | One finalizer dispatch churned 20 times and shipped a degraded draft. The same typed identities `Analyzer Agent` and `Finalizer Agent` were simultaneously reported as `missing_unproven_boundary` and `unknown_or_context_only_boundary`. Cause: non-code display identities containing spaces were not retained as exact participant surfaces; a model-authored boundary therefore could not match the obligation that required it. B505 gives exact typed identity precedence, then falls back to code aliases. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 1/2; human: 1/2.
- Trace is a clean non-regression witness for B504's isolation: explicit-window selection, auto-supplement, causal projection, on-chain-only principal causes, all requested cause families, business clues and the independent “major time occupancy / new direction” axis remain intact. No source analysis or system-authored replacement conclusion appeared.
- The read failure is a deterministic hard-contract contradiction, not model variance. `DiagramParticipantIdentitySurfaces` intentionally yields code identities only; participant coverage then discarded the original typed display identity whenever resolution failed. A boundary named exactly `Analyzer Agent` could not match itself, so it was rejected as unknown and the untouched obligation was emitted again as missing in the same response.
- B505 retains each analyzer-provided typed participant identity as its primary exact display surface and gives an exact obligation match priority over short/qualified alias compatibility. Alias resolution remains available only as fallback; no edge is created and relation evidence requirements are unchanged.
- B504 was not exercised by `required_diagram_edge_absent` in this replay (there was no post-finalize repair plan), so it remains implemented with package proof and pending a direct production witness rather than being declared closed.
