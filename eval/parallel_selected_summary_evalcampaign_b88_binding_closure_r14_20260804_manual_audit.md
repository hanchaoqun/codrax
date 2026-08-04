# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T16:46:24Z
- sweep_start_ts: 20260804-094623
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260804-094624 | answer_regex,answer_contains | none | 285s | 38 | read=3,repo_map=8,list=0,trace=0,source_lens=8 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=1 | fail | The 3 type / 5 function / 30 Kind-constant sets and block boundary are complete, but the answer calls `type Kind string` an `int` alias. The old count regex also falsely matched `grammar.go:26`, so runner PASS did not prove the Kind count binding; this is replaced by checkout-derived 3/5/30 scalar bindings. The system then appended three function anchors plus a generic weak-enumeration caveat even though the typed principal row sets were complete; record as `EVAL-B88-SUPPCAVEAT1`. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260804-094624 | answer_regex,answer_contains | none | 518s | 27 | read=14,repo_map=1,list=0,trace=0,source_lens=0 | midloop=14,inv=8/2,fin_reject=4,unavail=0,prune=0 | fail | The diagram's visible call edges finally preserve typed direction, but prose/item output still says `RunWith` is a wrapper/equivalent form of `Run`; source proves the reverse wrapper `Run -> RunWith`. A `gate.Run` item borrowed the `buildAnalysisIR -> gate.RunWith` citation because code-surface citation matching admitted identifier substrings. Explorer also took four dispatches/36 iterations and finalizer four rejects; the first closure retry was actionable, while later churn includes broad irrelevant DAG windows and participant identity edits. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
