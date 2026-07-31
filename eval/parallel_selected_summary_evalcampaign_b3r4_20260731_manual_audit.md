# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T10:22:15Z
- sweep_start_ts: 20260731-032215
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260731-032215 | log_attachment,answer_regex | log_triage | 134s | 21 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Mixed current-source lane is restored (`current_source=required`, three source reads), but the answer materially misidentifies the dock's `4/4` as a fourth finalizer retry / retry upper bound. `renderer_dock.go` defines it as stage ordinal `finalize=4/4`; `finalizerIdenticalErrorStreak=4` is an unrelated loop breaker. It also overstates first-byte timeout as a proved network-layer error instead of an upstream no-SSE-byte observation. |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-032215 | log_attachment | log_triage | 161s | 20 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | Principal runtime conclusion is now correct: stack top `main.crashy`, caller `main.main`, and checkout mismatch is only a mapping caveat. However, reads/grep/evidence over the attached repo-relative log are minted as `current_source`, so attachment lines become repository citations and falsely satisfy current-source authority. This is an origin-laundering architecture gap; segment efficiency also remains P2. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Code-chain verification

- `internal/render/renderer_dock.go::stageProgressForFocus` explicitly defines read mode as `analyze=1/4, explore=2/4, extract=3/4, finalize=4/4`. The ordinal is composed separately from the status-message payload.
- `internal/agent/finalizer.go::finalizerIdenticalErrorStreak=4` configures a same-error-class loop breaker. Numeric equality does not establish a producer relationship with the UI ordinal.
- `internal/render/status_messages.go` supplies the localized retry phrase only; it does not own the `K/N` counter.
- The oversized run's preflight typed artifact identity is `kind=log source=eval/fixtures/oversized_log.txt`, but the observation ledger later emits `origin=current_source` for `read_file`, `grep`, and `explorer.emit_evidence` on that exact path. The finalizer therefore receives a structurally false source lane even though the artifact lane was known before analysis.
