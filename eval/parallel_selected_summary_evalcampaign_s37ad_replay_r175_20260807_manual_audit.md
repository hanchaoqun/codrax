# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T16:37:26Z
- sweep_start_ts: 20260807-093724
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260807-093726 | typed_inventory_rowset,answer_contains | none | 88s | 21 | read=0,repo_map=2,list=1,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Typed source inventory returns exactly four `@Entry` rows and two `@Builder` rows with correct ArkTS symbols, files, line citations, and category separation. Analyzer pre-scan prose briefly inferred that no ETS files existed after production-only navigation missed third-party corpus files; this did not enter typed absence state or the final answer, and Explorer's source-inventory authority corrected it. Record as soft context precision debt, not a hard-gate failure. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-093726 | answer_regex,answer_contains | none | 474s | 33 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=0 | partial | Directional conclusion is correct and consistent: `buildAnalysisIR -> gate.RunWith` and `gate.Run -> gate.RunWith` converge in parallel, with no invented source-to-requested-sink path. S37ad reduces two Explorer attempts to one and completion attempts from 24 to 4. One remaining identity branch required an extra emit: `subject=gate + anchor_symbol=Run` was ignored by scoped bare-definition qualification until the model re-emitted `anchor_symbol=Run` without Subject. Finalizer itself spent about 232s waiting for one provider response. The answer also cites `gate.go:134` (the definition line) for the `gate.Run -> RunWith` call edge whose actual grounded callsite is line 135; block-level call-edge authority did not enforce per-item citation alignment. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
