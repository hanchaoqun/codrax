# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T13:07:43Z
- sweep_start_ts: 20260801-060742
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_git_current_source_explanation | PASS | eval/results/read_combo_git_current_source_explanation-20260801-060743 | answer_regex | none | 113s | 22 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B21-A is live: route current_source=required minted route_backed_history_explanation and the old forced accepted-member table did not return. B21-B exposed a P0 merge edge bug: typed changed-path authority says latest merge 2a58a60d has emitted=0/total=0/complete=true although its first-parent diff changes 3 files. The model then mixes current unrelated tests into “four new scenarios” and says a test helper directly affects production candidate construction, contradicting its later “no production path change” boundary. |
| 2 | read_combo_git_diff_hunk_current_code | FAIL | eval/results/read_combo_git_diff_hunk_current_code-20260801-060743 | answer_regex | none | 169s | 36 | read=8,repo_map=4,list=1,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Typed changed-path carrier is correct for the ordinary latest commit: 8/8 exact paths reach the finalizer. The runner failure is a multiline-equivalent oracle false negative: separate “diff hunk” and “当前源码依据” paragraphs satisfy the intended semantics but not one-line regex. Human failure is independent: the principal scalar observationRecordForVCSChangedPaths cites ToolVCSChangedPathSet in context.go, and the boundary incorrectly says both git_log and git_show default patch keep VCSHistory=nil (only git_show does). This is a general scalar-symbol/citation identity gap; do not repair via prose scanning. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
