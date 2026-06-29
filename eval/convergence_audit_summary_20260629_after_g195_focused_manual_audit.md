# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T14:12:57Z
- sweep_start_ts: 20260629-221256
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-221257 | trace_attachment,answer_regex | perf_triage | 149s | 32 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Functionally answers the user request: combines attached trace observation with current-source mechanism evidence, states the 86.111ms / 5.16x signal, explains span parsing and evidence boundary, and final citation pool includes current-source anchors. Residual P1/P2: context slightly high, one answer-contract advisory, and several ordered-list sidecar citations are weaker than the final citation pool; track under D1-G194, not a correctness blocker. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-221257 | typed_inventory_rowset,dimension_substring,answer_contains | none | 165s | 22 | read=10,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Functionally correct: main answer surface lists extend=2 (`Cart`, `String`), foreign func=2 (both `native_add` rows with distinct packages), and public class=8 with exact file paths and package fields. No finalizer reject or investigation-complete reject. Residual P1/P2: repeated midloop and high source reads remain, and row text duplicates family labels/attributes (`extend extend`, repeated `package=`); track under D1-G194 and cost/noise follow-up. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
