# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T03:59:25Z
- sweep_start_ts: 20260809-205923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260809-205925 | log_regex,answer_regex | none | 115s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Typed ledger graph correctly remained at `prepare_contribution_inputs` with rules=11, contributions=0 and reconcile blocked. The ordinary continuation planner then emitted invalid `emit_data_task_plan` params twice (`input_paths` missing, then unsupported `lookup_path`). Its deterministic fallback rebuilt state without `repoRoot`, and the bounded action-scaffold list had already been filled by variants from early action families, so no typed candidate survived although `compute_contributions` remained legal. Terminal failure is B450, not malformed result JSON and not B445/B448 production proof. |
| 2 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260809-205925 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 143s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Runner oracle is stale: it still pins the July-16 `0.018 + 49.638` split and `0.105` micro-fold, while current typed output consistently publishes `0.033 + 49.623` and an explicit incomplete-enumeration boundary. Human answer preserves the exact window/state account, on-chain self compute-supply 65.912ms, D-state 36.757ms with `dma_fence_default_w` and zero IO-wait, render-service/priority-inversion/small on-chain candidates, JIT semantic work, and keeps 49.623ms logd runnable pressure adjacent-only. Minor model prose says five listed small rows total under 2ms although they sum about 2.118ms; treat as advisory model arithmetic, not a production hard-gate request. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
