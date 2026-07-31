# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T10:54:28Z
- sweep_start_ts: 20260731-035428
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-035428 | log_attachment | log_triage | 154s | 18 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | Core runtime conclusion and S6 origin boundary remain correct; reads fell from 11 to 4. However both external-frame hop items are cited as current `internal/agent/analyzer.go:1`, even while prose says those runtime line numbers do not map to the checkout. This is a generic artifact item/citation identity violation, not a runtime-origin regression. |
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260731-035428 | log_attachment,answer_regex | log_triage | 179s | 29 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=3/0,fin_reject=0,unavail=1,prune=0 | fail | S7 works: after repo-map/positive discovery, authority remained `satisfied=false`; it became true only after source-line evidence. S5A also rendered in explorer/finalizer. Yet pre-triage observation 3 had already promoted `4/4 means fourth answer iteration` and an unproved finalizer→render propagation into a principal runtime fact. Final answer repeats both, contradicts the source comment that retry payload also covers gate-driven rejection, and accepts impossible citation `internal/render/status_messages.go:4346` although that file has 984 lines. Automatic PASS is a false positive. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
