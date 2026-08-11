# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T12:54:27Z
- sweep_start_ts: 20260811-055426
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260811-055427 | answer_regex,answer_contains | none | 155s | 22 | read=6,repo_map=4,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B530 confirmed. The model improved the typed roster to `run -> fetchUser -> send`, plus `send -> dispatchOnce` and `send -> nextDelay`, but the requested “complete call chain” still omitted the already-read `dispatchOnce -> fetch` invocation and the separate `send -> sleep` nested call. The answer calls a fan-out graph a complete linear chain. No validator reject occurred because the published edges themselves were honest; the missing-interior coverage contract is the gap. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-055427 | answer_regex,answer_contains,mermaid_edge_count | none | 401s | 39 | read=22,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=2,unavail=0,prune=1 | fail | B529b production-positive: Analyzer emitted separate incident-required `Mutable` and `BusContext` participants on its first accepted IR. Explorer did read and cite the exact initializer and reader area, but the assignment endpoint repair lived only in ToolRepair and was absent from the same-turn Summary; later unrelated successful evidence cleared the “latest emit” latch. Completion therefore converged with both participants explicitly unproven, and Finalizer honestly left them disconnected. This is B529c durable-repair-debt, not model-only fluctuation. The 401-second active stream returned a model-authored answer and did not enter the retired four-minute system fallback (B517 production-positive). |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
