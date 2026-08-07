# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T10:46:40Z
- sweep_start_ts: 20260807-034638
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-034640 | answer_regex,answer_contains | none | 105s | 24 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Mermaid is valid and all seven visible arrows match direct calls from `buildAnalysisIR`, with zero finalizer reject. But the requested sink is the distinct symbol `gate.Run`; source proves `buildAnalysisIR -> gate.RunWith` and the reverse wrapper edge `gate.Run -> RunWith`, so no source-to-requested-sink path exists. Analyzer emitted the contradictory typed shape `sink_mode=discover` plus non-empty `sink=gate.Run`; normalization silently discarded the sink, endpoint reachability stood down, and the answer omitted this boundary. The runner also carried stale formatting/line pins (`analyzer.go:1865`), but that is not the only failure. |
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260807-034640 | write_apply,answer_regex | none | 148s | 20 | read=7,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | S37m production-closed the retry gap: one write-analyzer dispatch retained all 12 contracts and deterministically softened only seven exact rows without grounding; no constraint/outcome/phase deletion occurred. The patch is correct and the Python oracle passed. Node remains unavailable, so the final `production_verification_source_static_only` unverified verdict is honest and human correctness remains uncertain rather than code-fail. Runtime fell from r158 196s to 148s. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B257-WRITEEXACTRETRY1` is production-closed: one dispatch, all 12 contracts preserved, item-local authority calibration only.
- `EVAL-B258-DISCOVERSINKWIRE1` is a deterministic typed-contract gap: `sink_mode=discover` and non-empty `sink` cannot both be accepted. Silent sink deletion disables exact endpoint reachability and can make a wrong boundary answer look complete.
- `EVAL-B259-EVALSOURCELOC1` is an eval-infrastructure issue: mutable current-source line numbers and same-line answer formatting must not own semantic correctness. The case should assert symbol/edge/citation presence with dynamic numeric coordinates over folded text.
- Sequence display-message parameter identity and all-language labelled/unlabelled flowchart relation anchors were not exercised by this diagram and remain open.
